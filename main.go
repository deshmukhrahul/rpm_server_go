package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	log.Println("Starting RPM Server...")

	configPath := os.Getenv("REPO_CONFIG_PATH")
	if configPath == "" {
		configPath = "repo_config.yaml"
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration from %s: %v", configPath, err)
	}

	log.Printf("✅ Configuration loaded successfully from %s", configPath)

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Handle("/assets/*", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))

	r.Route("/api", func(r chi.Router) {
		r.Get("/tags/{folder}", listTagsHandler(config))
		r.Get("/tags/{folder}/{tag}/packages", listPackagesHandler(config))
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware)
			r.Post("/create-tag", createTagHandler(config))
		})
	})

	r.Get("/browse/*", browserHandler(config))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<h1>RPM Server</h1><p>To browse repositories, start at <a href="/browse/">/browse/</a>.</p>`))
	})

	port := "8080"
	log.Printf("🚀 Go RPM server running at http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("❌ Server failed to start: %v", err)
	}
}
