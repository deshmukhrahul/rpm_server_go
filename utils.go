package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

// jsonResponse is a shared helper to write JSON data back to the client.
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error writing JSON response: %v", err)
	}
}

// listDirContents is a shared helper to read and filter directory contents.
func listDirContents(path string, filter func(os.DirEntry) bool) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var result []string
	for _, entry := range entries {
		if filter(entry) {
			result = append(result, entry.Name())
		}
	}
	return result, nil
}
