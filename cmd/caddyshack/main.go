package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	caddyshack "github.com/djedi/caddyshack"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/handlers"
	"github.com/djedi/caddyshack/internal/static"
	"github.com/djedi/caddyshack/internal/templates"
)

func main() {
	cfg := config.Load()

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
	dashboardHandler := handlers.NewDashboardHandler(tmpl)
	sitesHandler := handlers.NewSitesHandler(tmpl, cfg)

	http.Handle("/", dashboardHandler)
	http.HandleFunc("/sites", sitesHandler.List)

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
