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
	"github.com/djedi/caddyshack/internal/middleware"
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
	var tmpl *templates.Templates
	if cfg.DevMode {
		log.Println("Development mode: loading templates from filesystem")
		tmpl, err = templates.New(cfg.TemplatesDir)
	} else {
		log.Println("Production mode: loading templates from embedded filesystem")
		tmpl, err = templates.NewFromFS(caddyshack.TemplatesFS())
	}
	if err != nil {
		log.Fatalf("Failed to load templates: %v", err)
	}

	// Initialize auth
	auth := middleware.NewAuth(cfg.AuthUser, cfg.AuthPass)

	// Create a new mux for protected routes
	mux := http.NewServeMux()

	// Serve static files
	// In development mode, serve from filesystem for hot reloading
	// In production, serve from embedded files
	if cfg.DevMode {
		log.Println("Development mode: serving static files from filesystem")
		mux.Handle("/static/", static.Handler(nil, cfg.StaticDir))
	} else {
		log.Println("Production mode: serving static files from embedded filesystem")
		mux.Handle("/static/", static.Handler(caddyshack.StaticFS(), ""))
	}

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(tmpl, auth)
	dashboardHandler := handlers.NewDashboardHandler(tmpl, cfg)
	sitesHandler := handlers.NewSitesHandler(tmpl, cfg, db)
	snippetsHandler := handlers.NewSnippetsHandler(tmpl, cfg, db)
	historyHandler := handlers.NewHistoryHandler(tmpl, cfg, db)
	exportHandler := handlers.NewExportHandler(tmpl, cfg, db)
	importHandler := handlers.NewImportHandler(tmpl, cfg, db)

	mux.Handle("/", dashboardHandler)
	mux.HandleFunc("/status", dashboardHandler.Status)
	mux.HandleFunc("/sites/", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/sites", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			sitesHandler.Create(w, r)
		} else {
			sitesHandler.List(w, r)
		}
	})

	mux.HandleFunc("/snippets/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Route based on path and method
		switch {
		case path == "/snippets/" || path == "/snippets":
			if r.Method == http.MethodPost {
				snippetsHandler.Create(w, r)
			} else {
				snippetsHandler.List(w, r)
			}
		case path == "/snippets/new":
			snippetsHandler.New(w, r)
		case strings.HasSuffix(path, "/edit"):
			snippetsHandler.Edit(w, r)
		default:
			// Handle PUT for updates, DELETE for removal, GET for detail view
			switch r.Method {
			case http.MethodPut:
				snippetsHandler.Update(w, r)
			case http.MethodDelete:
				snippetsHandler.Delete(w, r)
			default:
				snippetsHandler.Detail(w, r)
			}
		}
	})
	mux.HandleFunc("/snippets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			snippetsHandler.Create(w, r)
		} else {
			snippetsHandler.List(w, r)
		}
	})

	mux.HandleFunc("/history/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/view"):
			historyHandler.View(w, r)
		case strings.HasSuffix(path, "/diff"):
			historyHandler.Diff(w, r)
		case strings.HasSuffix(path, "/restore"):
			if r.Method == http.MethodPost {
				historyHandler.Restore(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		default:
			historyHandler.List(w, r)
		}
	})
	mux.HandleFunc("/history", func(w http.ResponseWriter, r *http.Request) {
		historyHandler.List(w, r)
	})

	mux.HandleFunc("/export", exportHandler.ExportCaddyfile)
	mux.HandleFunc("/export/json", exportHandler.ExportJSON)
	mux.HandleFunc("/export/backup", exportHandler.ExportBackup)

	mux.HandleFunc("/import/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/import/preview":
			if r.Method == http.MethodPost {
				importHandler.Preview(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case path == "/import/apply":
			if r.Method == http.MethodPost {
				importHandler.Apply(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		default:
			importHandler.ImportPage(w, r)
		}
	})
	mux.HandleFunc("/import", importHandler.ImportPage)

	// Apply auth middleware to protected routes
	authMiddleware := auth.Middleware()
	protectedHandler := authMiddleware(mux)

	// Health check endpoint is NOT protected by auth
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Login and logout routes are NOT protected by auth
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authHandler.Login(w, r)
		} else {
			authHandler.LoginPage(w, r)
		}
	})
	http.HandleFunc("/logout", authHandler.Logout)

	// Static files should be accessible without auth for login page styling
	if cfg.DevMode {
		http.Handle("/static/", static.Handler(nil, cfg.StaticDir))
	} else {
		http.Handle("/static/", static.Handler(caddyshack.StaticFS(), ""))
	}

	// All other routes go through auth middleware
	http.Handle("/", protectedHandler)

	absTemplatesDir, _ := filepath.Abs(cfg.TemplatesDir)
	log.Printf("Templates directory: %s", absTemplatesDir)
	if cfg.AuthEnabled() {
		log.Println("Session-based auth enabled")
	} else {
		log.Println("Auth disabled (set CADDYSHACK_AUTH_USER and CADDYSHACK_AUTH_PASS to enable)")
	}
	log.Printf("Starting Caddyshack on port %s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
