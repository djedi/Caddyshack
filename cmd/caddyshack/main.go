package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	caddyshack "github.com/djedi/caddyshack"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/handlers"
	"github.com/djedi/caddyshack/internal/static"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

func main() {
	cfg := config.Load()

	// Initialize database
	db, err := store.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize templates
	tmpl, err := templates.New(cfg.TemplatesDir)
	if err != nil {
		log.Fatalf("Failed to load templates: %v", err)
	}

	// Serve static files
	// In development mode, serve from filesystem for hot reloading
	// In production, serve from embedded files
	if cfg.DevMode {
		log.Println("Development mode: serving static files from filesystem")
		http.Handle("/static/", static.Handler(nil, cfg.StaticDir))
	} else {
		log.Println("Production mode: serving static files from embedded filesystem")
		http.Handle("/static/", static.Handler(caddyshack.StaticFS(), ""))
	}

	// Initialize handlers
	dashboardHandler := handlers.NewDashboardHandler(tmpl, cfg)
	sitesHandler := handlers.NewSitesHandler(tmpl, cfg, db)

	http.Handle("/", dashboardHandler)
	http.HandleFunc("/status", dashboardHandler.Status)
	http.HandleFunc("/sites/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Route based on path and method
		switch {
		case path == "/sites/" || path == "/sites":
			if r.Method == http.MethodPost {
				sitesHandler.Create(w, r)
			} else {
				sitesHandler.List(w, r)
			}
		case path == "/sites/new":
			sitesHandler.New(w, r)
		case strings.HasSuffix(path, "/edit"):
			sitesHandler.Edit(w, r)
		default:
			// Handle PUT for updates, DELETE for removal, GET for detail view
			switch r.Method {
			case http.MethodPut:
				sitesHandler.Update(w, r)
			case http.MethodDelete:
				sitesHandler.Delete(w, r)
			default:
				sitesHandler.Detail(w, r)
			}
		}
	})
	http.HandleFunc("/sites", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			sitesHandler.Create(w, r)
		} else {
			sitesHandler.List(w, r)
		}
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	absTemplatesDir, _ := filepath.Abs(cfg.TemplatesDir)
	log.Printf("Templates directory: %s", absTemplatesDir)
	log.Printf("Starting Caddyshack on port %s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
