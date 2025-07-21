package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

func listTagsHandler(config *AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		folder := chi.URLParam(r, "folder")
		repoTagDir := config.GetTagsPathForRepo(folder)
		tags, err := listDirContents(repoTagDir, func(e os.DirEntry) bool { return e.IsDir() })
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "Could not list tags"})
			return
		}
		jsonResponse(w, http.StatusOK, map[string][]string{"tags": tags})
	}
}

func listPackagesHandler(config *AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		folder := chi.URLParam(r, "folder")
		tag := chi.URLParam(r, "tag")
		repoTagDir := config.GetTagsPathForRepo(folder)
		path := filepath.Join(repoTagDir, tag)
		pkgs, err := listDirContents(path, func(e os.DirEntry) bool { return strings.HasSuffix(e.Name(), ".rpm") })
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "Could not list packages"})
			return
		}
		jsonResponse(w, http.StatusOK, map[string][]string{"packages": pkgs})
	}
}

func createTagHandler(config *AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		folder := r.URL.Query().Get("folder")
		tagType := r.URL.Query().Get("type")
		if folder == "" || (tagType != "monthly" && tagType != "half-monthly") {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": "Invalid folder or tag type"})
			return
		}

		sourcePaths, err := config.GetRepoPaths(folder)
		if err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		repoTagDir := config.GetTagsPathForRepo(folder)
		today := time.Now().UTC().Format("2006-01-02")
		liveDir := filepath.Join(repoTagDir, tagType)
		backupBase := filepath.Join(repoTagDir, fmt.Sprintf("%s_%s", tagType, today))
		backupDir := nextAvailableBackup(backupBase)

		var prevRpms []string
		backupCreated := false
		if _, err := os.Stat(liveDir); err == nil {
			log.Printf("Backing up existing live directory '%s' to '%s'", liveDir, backupDir)
			if err := os.Rename(liveDir, backupDir); err != nil {
				jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to backup live directory"})
				return
			}
			prevRpms, _ = listDirContents(backupDir, func(e os.DirEntry) bool { return strings.HasSuffix(e.Name(), ".rpm") })
			backupCreated = true
		}

		if err := os.MkdirAll(liveDir, 0755); err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create new live directory"})
			return
		}

		var newRpms []string
		for _, sourcePath := range sourcePaths {
			rpmsInPath, _ := listDirContents(sourcePath, func(e os.DirEntry) bool { return strings.HasSuffix(e.Name(), ".rpm") })
			for _, rpmName := range rpmsInPath {
				src, _ := filepath.Abs(filepath.Join(sourcePath, rpmName))
				dst := filepath.Join(liveDir, rpmName)
				os.Remove(dst) // Remove existing symlink if it exists
				if err := os.Symlink(src, dst); err != nil {
					log.Printf("Warning: Could not link %s to %s: %v", src, dst, err)
					continue
				}
				newRpms = append(newRpms, rpmName)
			}
		}

		cmd := exec.Command("createrepo_c", ".")
		cmd.Dir = liveDir
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("createrepo_c failed: %v. Output: %s", err, string(output))
			jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "createrepo_c execution failed"})
			return
		}

		diffInfo := createDiff(prevRpms, newRpms, backupDir, tagType)
		diffCount := 0
		if diffInfo != nil {
			diffCount = len(diffInfo["added"].([]string)) + len(diffInfo["removed"].([]string))
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"folder": folder, "tag": tagType, "date": today, "file_count": len(newRpms),
			"diff_count": diffCount, "backup_created": backupCreated,
		})
	}
}

func nextAvailableBackup(base string) string {
	candidate := base
	count := 0
	for {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		count++
		candidate = fmt.Sprintf("%s_%d", base, count)
	}
}

func createDiff(oldRPMs, newRPMs []string, backupDir, newTagType string) map[string]interface{} {
	oldSet := make(map[string]struct{})
	for _, s := range oldRPMs {
		oldSet[s] = struct{}{}
	}
	newSet := make(map[string]struct{})
	for _, s := range newRPMs {
		newSet[s] = struct{}{}
	}

	var added, removed []string
	for item := range newSet {
		if _, found := oldSet[item]; !found {
			added = append(added, item)
		}
	}
	for item := range oldSet {
		if _, found := newSet[item]; !found {
			removed = append(removed, item)
		}
	}

	if len(added) == 0 && len(removed) == 0 {
		return nil
	}
	sort.Strings(added)
	sort.Strings(removed)

	diff := map[string]interface{}{
		"from": filepath.Base(backupDir), "to": newTagType,
		"added": added, "removed": removed,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	diffPath := filepath.Join(backupDir, "diff.json")
	data, err := json.MarshalIndent(diff, "", "  ")
	if err != nil {
		return nil
	}
	if err := os.WriteFile(diffPath, data, 0644); err != nil {
		return nil
	}
	return diff
}
