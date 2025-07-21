package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
)

// --- Structs for Template Data ---
type Breadcrumb struct{ Name, URL string }
type DirEntry struct {
	Name, URL, Size, Modified, RPMCount string
	IsDir, HasDiff                      bool
	DiffURL                             string
	Icon                                template.HTML
	IsActive                            bool
}
type BrowserData struct {
	PathDisplay, CurrentDir, ParentURL string
	Breadcrumbs                        []Breadcrumb
	Entries                            []DirEntry
	IsRootListing, IsDashboard         bool
}

// NOTE: We only need browser.html now. The diff template is no longer used.
var templates = template.Must(template.ParseFiles("templates/browser.html"))

// --- Main Handler ---
func browserHandler(config *AppConfig) http.HandlerFunc {
	repoPathMap := make(map[string]string)
	for _, repo := range config.Repos {
		if repo.TagDir != "" {
			repoPathMap[repo.ID] = repo.TagDir
		} else {
			repoPathMap[repo.ID] = config.TagsBase
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		pathParam := chi.URLParam(r, "*")
		if pathParam == "" || pathParam == "/" {
			serveVirtualRootListing(w, r, repoPathMap)
			return
		}

		segments := strings.Split(strings.Trim(pathParam, "/"), "/")
		repoID := segments[0]
		baseFSPath, ok := repoPathMap[repoID]
		if !ok {
			http.NotFound(w, r)
			return
		}

		remainingPath := path.Join(segments[1:]...)
		fullFSPath := filepath.Join(baseFSPath, remainingPath)
		log.Printf("[Browser] URL: '%s' -> Mapped repo '%s' to FS path: '%s'", r.URL.Path, repoID, fullFSPath)

		info, err := os.Stat(fullFSPath)
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}

		if info.IsDir() {
			if !strings.HasSuffix(r.URL.Path, "/") {
				http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
				return
			}
			serveDirectoryListing(w, r, fullFSPath, r.URL.Path)
		} else {
			// This is the simplified, correct logic
			if info.Name() == "diff.json" {
				serveDiffJSON(w, r, fullFSPath)
			} else {
				servePhysicalFile(w, r, fullFSPath)
			}
		}
	}
}

// --- NEW, SIMPLER FUNCTION TO SERVE RAW JSON ---
func serveDiffJSON(w http.ResponseWriter, r *http.Request, fullFSPath string) {
	log.Printf("[Browser] Serving raw JSON diff: '%s'", fullFSPath)
	content, err := os.ReadFile(fullFSPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// Set the correct header to tell the browser this is a JSON file.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

// (The rest of the file is the correct, stable version)
func servePhysicalFile(w http.ResponseWriter, r *http.Request, fsPath string) {
	file, err := os.Open(fsPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(fsPath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, stat.Name()))
	io.Copy(w, file)
}
func serveVirtualRootListing(w http.ResponseWriter, _ *http.Request, repoPathMap map[string]string) {
	var entries []DirEntry
	for repoID, pathValue := range repoPathMap {
		entry := DirEntry{
			Name: repoID, IsDir: true, URL: path.Join("/browse/", repoID) + "/", Icon: iconMap["repo"],
		}
		if info, err := os.Stat(pathValue); err == nil && info.IsDir() {
			entry.IsActive = true
			entry.Modified = info.ModTime().UTC().Format("2006-01-02 15:04")
			totalRPMs := 0
			tags, _ := os.ReadDir(pathValue)
			for _, tag := range tags {
				if tag.IsDir() {
					rpms, _ := listDirContents(filepath.Join(pathValue, tag.Name()), func(e os.DirEntry) bool { return strings.HasSuffix(e.Name(), ".rpm") })
					totalRPMs += len(rpms)
				}
			}
			entry.RPMCount = fmt.Sprintf("%d RPMs", totalRPMs)
		} else {
			entry.IsActive = false
			entry.Modified = "(tags not yet initialized)"
			entry.RPMCount = "0 RPMs"
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	data := BrowserData{
		PathDisplay: "/browse/", CurrentDir: "Available Repositories", Entries: entries,
		IsRootListing: true, IsDashboard: true,
	}
	templates.ExecuteTemplate(w, "browser.html", data)
}
func serveDirectoryListing(w http.ResponseWriter, _ *http.Request, fullFSPath, urlPath string) {
	dirEntries, err := os.ReadDir(fullFSPath)
	if err != nil {
		http.Error(w, "Could not read directory", http.StatusInternalServerError)
		return
	}
	var breadcrumbs []Breadcrumb
	segments := strings.Split(strings.Trim(urlPath, "/"), "/")
	for i := 1; i < len(segments)-1; i++ {
		breadcrumbs = append(breadcrumbs, Breadcrumb{Name: segments[i], URL: path.Join("/", segments[0], strings.Join(segments[1:i+1], "/")) + "/"})
	}
	data := BrowserData{
		PathDisplay: urlPath, CurrentDir: segments[len(segments)-1], Breadcrumbs: breadcrumbs,
		ParentURL: path.Dir(strings.TrimSuffix(urlPath, "/")) + "/", IsDashboard: false,
	}
	for _, entry := range dirEntries {
		info, _ := os.Stat(filepath.Join(fullFSPath, entry.Name()))
		isDir := info.IsDir()
		de := DirEntry{
			Name:     entry.Name(),
			URL:      path.Join(urlPath, entry.Name()),
			IsDir:    isDir,
			Modified: info.ModTime().UTC().Format("2006-01-02 15:04"),
			Size:     fmt.Sprintf("%.1f KB", float64(info.Size())/1024),
			IsActive: true,
		}
		if isDir {
			de.URL += "/"
			de.Icon = iconMap["folder"]
			rpms, _ := listDirContents(filepath.Join(fullFSPath, entry.Name()), func(e os.DirEntry) bool { return strings.HasSuffix(e.Name(), ".rpm") })
			de.RPMCount = fmt.Sprintf("%d", len(rpms))
			if _, err := os.Stat(filepath.Join(fullFSPath, entry.Name(), "diff.json")); err == nil {
				de.HasDiff = true
				de.DiffURL = path.Join(de.URL, "diff.json")
			}
		} else {
			de.Icon = iconMap["rpm"]
		}
		data.Entries = append(data.Entries, de)
	}
	sort.Slice(data.Entries, func(i, j int) bool {
		if data.Entries[i].IsDir != data.Entries[j].IsDir {
			return data.Entries[i].IsDir
		}
		return data.Entries[i].Name < data.Entries[j].Name
	})
	templates.ExecuteTemplate(w, "browser.html", data)
}
