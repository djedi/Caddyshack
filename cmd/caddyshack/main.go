package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/dustinredseam/caddyshack/internal/templates"
)

func main() {
	port := os.Getenv("CADDYSHACK_PORT")
	if port == "" {
		port = "8080"
	}

	// Determine templates directory relative to working directory
	templatesDir := "templates"
	if dir := os.Getenv("CADDYSHACK_TEMPLATES_DIR"); dir != "" {
		templatesDir = dir
	}

	// Initialize templates
	tmpl, err := templates.New(templatesDir)
	if err != nil {
		log.Fatalf("Failed to load templates: %v", err)
	}

	// Serve static files
	staticDir := "static"
	if dir := os.Getenv("CADDYSHACK_STATIC_DIR"); dir != "" {
		staticDir = dir
	}
	fs := http.FileServer(http.Dir(staticDir))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only handle exact "/" path, return 404 for others
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		data := templates.PageData{
			Title:     "Dashboard",
			ActiveNav: "dashboard",
		}
		if err := tmpl.Render(w, "home.html", data); err != nil {
			log.Printf("Error rendering template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	absTemplatesDir, _ := filepath.Abs(templatesDir)
	log.Printf("Templates directory: %s", absTemplatesDir)
	log.Printf("Starting Caddyshack on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
