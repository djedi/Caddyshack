package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	caddyshack "github.com/djedi/caddyshack"
	"github.com/djedi/caddyshack/internal/auth"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/handlers"
	"github.com/djedi/caddyshack/internal/metrics"
	"github.com/djedi/caddyshack/internal/middleware"
	"github.com/djedi/caddyshack/internal/notifications"
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
	var authMiddleware *middleware.Auth
	var userStore *auth.UserStore

	if cfg.MultiUserMode {
		// Multi-user mode: use database-backed authentication
		userStore = auth.NewUserStore(db.DB())
		authMiddleware = middleware.NewMultiUserAuth(userStore)

		// Check if any users exist; if not, create initial admin user
		count, err := userStore.Count()
		if err != nil {
			log.Fatalf("Failed to count users: %v", err)
		}
		if count == 0 {
			// Create initial admin user from config
			if cfg.AuthUser != "" && cfg.AuthPass != "" {
				_, err := userStore.Create(cfg.AuthUser, "", cfg.AuthPass, auth.RoleAdmin)
				if err != nil {
					log.Fatalf("Failed to create initial admin user: %v", err)
				}
				log.Printf("Created initial admin user: %s", cfg.AuthUser)
			} else {
				log.Println("WARNING: Multi-user mode enabled but no users exist.")
				log.Println("Set CADDYSHACK_AUTH_USER and CADDYSHACK_AUTH_PASS to create an initial admin user.")
			}
		}
		log.Println("Multi-user mode enabled with database-backed authentication")
	} else {
		// Legacy single-user mode
		authMiddleware = middleware.NewAuth(cfg.AuthUser, cfg.AuthPass)
	}

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
	authHandler := handlers.NewAuthHandler(tmpl, authMiddleware)
	dashboardHandler := handlers.NewDashboardHandler(tmpl, cfg, userStore)
	sitesHandler := handlers.NewSitesHandler(tmpl, cfg, db)
	snippetsHandler := handlers.NewSnippetsHandler(tmpl, cfg, db)
	historyHandler := handlers.NewHistoryHandler(tmpl, cfg, db)
	exportHandler := handlers.NewExportHandler(tmpl, cfg, db)
	importHandler := handlers.NewImportHandler(tmpl, cfg, db)
	certificatesHandler := handlers.NewCertificatesHandler(tmpl, cfg)
	globalOptionsHandler := handlers.NewGlobalOptionsHandler(tmpl, cfg, db)
	logsHandler := handlers.NewLogsHandler(tmpl, cfg)
	containersHandler := handlers.NewContainersHandler(tmpl, cfg)
	notificationsHandler := handlers.NewNotificationsHandler(tmpl, cfg, db)
	domainsHandler := handlers.NewDomainsHandler(tmpl, cfg, db)
	searchHandler := handlers.NewSearchHandler(tmpl, cfg)

	// Users handler - only created in multi-user mode
	var usersHandler *handlers.UsersHandler
	var profileHandler *handlers.ProfileHandler
	var apiTokensHandler *handlers.APITokensHandler
	var totpHandler *handlers.TOTPHandler
	var tokenStore *auth.TokenStore
	var totpStore *auth.TOTPStore
	if cfg.MultiUserMode && userStore != nil {
		usersHandler = handlers.NewUsersHandler(tmpl, cfg, userStore)
		profileHandler = handlers.NewProfileHandler(tmpl, cfg, userStore, authMiddleware)
		tokenStore = auth.NewTokenStore(db.DB())
		apiTokensHandler = handlers.NewAPITokensHandler(tmpl, cfg, tokenStore)
		totpStore = auth.NewTOTPStore(db.DB())
		totpHandler = handlers.NewTOTPHandler(tmpl, cfg, userStore, totpStore)
		// Set token store on auth middleware for Bearer token authentication
		authMiddleware.SetTokenStore(tokenStore)
		// Set TOTP store on auth handler for 2FA verification
		authHandler.SetTOTPStore(totpStore)
	}

	// Audit handler - admin only
	auditHandler := handlers.NewAuditHandler(tmpl, cfg, db)

	// Metrics handler for Prometheus metrics endpoint
	metricsHandler := handlers.NewMetricsHandler(cfg)

	// Health handler for comprehensive health checks
	healthHandler := handlers.NewHealthHandler(cfg, db.DB())

	// Performance handler for performance monitoring dashboard
	performanceHandler := handlers.NewPerformanceHandler(tmpl, db)

	// Start metrics aggregator for performance monitoring
	metricsAggregator := metrics.NewAggregator(db, cfg)
	metricsAggregator.Start()
	defer metricsAggregator.Stop()
	log.Println("Performance metrics aggregator started")

	// Initialize RBAC settings
	middleware.SetMultiUserMode(cfg.MultiUserMode)

	// Initialize rate limiter
	rateLimitConfig := &middleware.RateLimitConfig{
		LoginMaxAttempts: cfg.RateLimitLoginAttempts,
		LoginWindow:      time.Duration(cfg.RateLimitLoginWindow) * time.Second,
		APIMaxRequests:   cfg.RateLimitAPIRequests,
		APIWindow:        time.Duration(cfg.RateLimitAPIWindow) * time.Second,
		Enabled:          cfg.RateLimitEnabled,
	}
	rateLimiter := middleware.NewRateLimiter(rateLimitConfig)

	// Start certificate expiry checker background job
	notificationService := notifications.NewService(db.DB())

	// Create email sender if configured
	var notificationCreator notifications.NotificationCreator = notificationService
	if cfg.EmailConfigured() {
		emailConfig := notifications.EmailConfig{
			Enabled:            cfg.EmailEnabled,
			SMTPHost:           cfg.SMTPHost,
			SMTPPort:           cfg.SMTPPort,
			SMTPUser:           cfg.SMTPUser,
			SMTPPassword:       cfg.SMTPPassword,
			FromAddress:        cfg.EmailFrom,
			FromName:           cfg.EmailFromName,
			ToAddresses:        cfg.EmailTo,
			UseTLS:             cfg.EmailUseTLS,
			UseSTARTTLS:        cfg.EmailUseSTARTTLS,
			InsecureSkipVerify: cfg.EmailInsecureSkipVerify,
		}
		emailSender := notifications.NewEmailSender(emailConfig)
		notificationCreator = notifications.NewEmailNotifier(notificationService, emailSender, cfg.EmailSendOnWarning)
		log.Printf("Email notifications enabled (sending to: %v)", cfg.EmailTo)
	}

	certChecker := notifications.NewCertificateChecker(notificationCreator, cfg.CaddyAdminAPI)
	certChecker.Start()
	defer certChecker.Stop()
	log.Println("Certificate expiry checker started")

	// Start domain expiry checker background job
	domainChecker := notifications.NewDomainChecker(notificationCreator, db)
	domainChecker.Start()
	defer domainChecker.Stop()
	log.Println("Domain expiry checker started")

	// Set up rate limiter lockout notification callback
	rateLimiter.SetLockoutCallback(func(ip string, duration time.Duration) {
		message := fmt.Sprintf("IP address %s has been locked out due to too many failed login attempts. Lockout expires in %s.", ip, duration.Round(time.Second))
		_, err := notificationService.Create(
			notifications.TypeSystem,
			notifications.SeverityWarning,
			"Login Rate Limit Exceeded",
			message,
			fmt.Sprintf(`{"ip": "%s", "duration_seconds": %d}`, ip, int(duration.Seconds())),
		)
		if err != nil {
			log.Printf("Failed to create rate limit notification: %v", err)
		}
	})

	// Helper to apply RBAC middleware to a handler function
	withRBAC := func(perm auth.Permission, handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			middleware.RequirePermission(perm)(http.HandlerFunc(handler)).ServeHTTP(w, r)
		}
	}

	mux.Handle("/", dashboardHandler)
	mux.HandleFunc("/status", dashboardHandler.Status)
	mux.HandleFunc("/dashboard/preferences", dashboardHandler.SavePreferences)
	mux.HandleFunc("/sites/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Route based on path and method
		switch {
		case path == "/sites/" || path == "/sites":
			if r.Method == http.MethodPost {
				withRBAC(auth.PermEditSites, sitesHandler.Create)(w, r)
			} else {
				sitesHandler.List(w, r)
			}
		case path == "/sites/new":
			withRBAC(auth.PermEditSites, sitesHandler.New)(w, r)
		case strings.HasSuffix(path, "/edit"):
			withRBAC(auth.PermEditSites, sitesHandler.Edit)(w, r)
		default:
			// Handle PUT for updates, DELETE for removal, GET for detail view
			switch r.Method {
			case http.MethodPut:
				withRBAC(auth.PermEditSites, sitesHandler.Update)(w, r)
			case http.MethodDelete:
				withRBAC(auth.PermEditSites, sitesHandler.Delete)(w, r)
			default:
				sitesHandler.Detail(w, r)
			}
		}
	})
	mux.HandleFunc("/sites", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			withRBAC(auth.PermEditSites, sitesHandler.Create)(w, r)
		} else {
			sitesHandler.List(w, r)
		}
	})

	// API endpoint for validating custom directives
	mux.HandleFunc("/api/validate-directives", sitesHandler.ValidateDirectives)

	mux.HandleFunc("/snippets/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Route based on path and method
		switch {
		case path == "/snippets/" || path == "/snippets":
			if r.Method == http.MethodPost {
				withRBAC(auth.PermEditSnippets, snippetsHandler.Create)(w, r)
			} else {
				snippetsHandler.List(w, r)
			}
		case path == "/snippets/new":
			withRBAC(auth.PermEditSnippets, snippetsHandler.New)(w, r)
		case strings.HasSuffix(path, "/edit"):
			withRBAC(auth.PermEditSnippets, snippetsHandler.Edit)(w, r)
		default:
			// Handle PUT for updates, DELETE for removal, GET for detail view
			switch r.Method {
			case http.MethodPut:
				withRBAC(auth.PermEditSnippets, snippetsHandler.Update)(w, r)
			case http.MethodDelete:
				withRBAC(auth.PermEditSnippets, snippetsHandler.Delete)(w, r)
			default:
				snippetsHandler.Detail(w, r)
			}
		}
	})
	mux.HandleFunc("/snippets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			withRBAC(auth.PermEditSnippets, snippetsHandler.Create)(w, r)
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
				withRBAC(auth.PermRestoreHistory, historyHandler.Restore)(w, r)
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

	mux.HandleFunc("/export", withRBAC(auth.PermImportExport, exportHandler.ExportCaddyfile))
	mux.HandleFunc("/export/json", withRBAC(auth.PermImportExport, exportHandler.ExportJSON))
	mux.HandleFunc("/export/backup", withRBAC(auth.PermImportExport, exportHandler.ExportBackup))

	mux.HandleFunc("/import/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/import/preview":
			if r.Method == http.MethodPost {
				withRBAC(auth.PermImportExport, importHandler.Preview)(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case path == "/import/apply":
			if r.Method == http.MethodPost {
				withRBAC(auth.PermImportExport, importHandler.Apply)(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		default:
			withRBAC(auth.PermImportExport, importHandler.ImportPage)(w, r)
		}
	})
	mux.HandleFunc("/import", withRBAC(auth.PermImportExport, importHandler.ImportPage))

	mux.HandleFunc("/certificates", certificatesHandler.List)
	mux.HandleFunc("/certificates/widget", certificatesHandler.Widget)

	mux.HandleFunc("/global-options/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/global-options/" || path == "/global-options":
			if r.Method == http.MethodPut {
				withRBAC(auth.PermEditGlobal, globalOptionsHandler.Update)(w, r)
			} else {
				globalOptionsHandler.List(w, r)
			}
		case path == "/global-options/edit":
			withRBAC(auth.PermEditGlobal, globalOptionsHandler.Edit)(w, r)
		case path == "/global-options/log":
			if r.Method == http.MethodPut {
				withRBAC(auth.PermEditGlobal, globalOptionsHandler.UpdateLogConfig)(w, r)
			} else {
				withRBAC(auth.PermEditGlobal, globalOptionsHandler.LogConfig)(w, r)
			}
		default:
			globalOptionsHandler.List(w, r)
		}
	})
	mux.HandleFunc("/global-options", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			withRBAC(auth.PermEditGlobal, globalOptionsHandler.Update)(w, r)
		} else {
			globalOptionsHandler.List(w, r)
		}
	})

	mux.HandleFunc("/logs", logsHandler.List)

	mux.HandleFunc("/search", searchHandler.Search)

	// Performance monitoring routes
	mux.HandleFunc("/performance/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/performance/" || path == "/performance":
			performanceHandler.Page(w, r)
		case path == "/performance/widget":
			performanceHandler.Widget(w, r)
		case path == "/performance/data":
			performanceHandler.Data(w, r)
		default:
			performanceHandler.Page(w, r)
		}
	})
	mux.HandleFunc("/performance", performanceHandler.Page)

	mux.HandleFunc("/containers/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/containers/" || path == "/containers":
			containersHandler.List(w, r)
		case path == "/containers/widget":
			containersHandler.Widget(w, r)
		case strings.HasSuffix(path, "/start"):
			if r.Method == http.MethodPost {
				withRBAC(auth.PermManageContainers, containersHandler.Start)(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case strings.HasSuffix(path, "/stop"):
			if r.Method == http.MethodPost {
				withRBAC(auth.PermManageContainers, containersHandler.Stop)(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case strings.HasSuffix(path, "/restart"):
			if r.Method == http.MethodPost {
				withRBAC(auth.PermManageContainers, containersHandler.Restart)(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case strings.HasSuffix(path, "/logs"):
			withRBAC(auth.PermManageContainers, containersHandler.Logs)(w, r)
		default:
			containersHandler.List(w, r)
		}
	})
	mux.HandleFunc("/containers", containersHandler.List)

	mux.HandleFunc("/notifications/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/notifications/" || path == "/notifications":
			notificationsHandler.List(w, r)
		case path == "/notifications/badge":
			notificationsHandler.Badge(w, r)
		case path == "/notifications/panel":
			notificationsHandler.Panel(w, r)
		case path == "/notifications/acknowledge-all":
			if r.Method == http.MethodPost {
				withRBAC(auth.PermManageNotifications, notificationsHandler.AcknowledgeAll)(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case strings.HasSuffix(path, "/acknowledge"):
			if r.Method == http.MethodPut {
				withRBAC(auth.PermManageNotifications, notificationsHandler.Acknowledge)(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		default:
			// Handle DELETE for notification removal
			if r.Method == http.MethodDelete {
				withRBAC(auth.PermManageNotifications, notificationsHandler.Delete)(w, r)
			} else {
				notificationsHandler.List(w, r)
			}
		}
	})
	mux.HandleFunc("/notifications", notificationsHandler.List)

	mux.HandleFunc("/domains/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Route based on path and method
		switch {
		case path == "/domains/" || path == "/domains":
			if r.Method == http.MethodPost {
				withRBAC(auth.PermEditDomains, domainsHandler.Create)(w, r)
			} else {
				domainsHandler.List(w, r)
			}
		case path == "/domains/new":
			withRBAC(auth.PermEditDomains, domainsHandler.New)(w, r)
		case path == "/domains/widget":
			domainsHandler.Widget(w, r)
		case strings.HasSuffix(path, "/edit"):
			withRBAC(auth.PermEditDomains, domainsHandler.Edit)(w, r)
		case strings.HasSuffix(path, "/whois"):
			// WHOIS lookup endpoint
			switch r.Method {
			case http.MethodPost:
				domainsHandler.WHOISLookup(w, r)
			default:
				domainsHandler.GetWHOISInfo(w, r)
			}
		default:
			// Handle PUT for updates, DELETE for removal
			switch r.Method {
			case http.MethodPut:
				withRBAC(auth.PermEditDomains, domainsHandler.Update)(w, r)
			case http.MethodDelete:
				withRBAC(auth.PermEditDomains, domainsHandler.Delete)(w, r)
			default:
				domainsHandler.List(w, r)
			}
		}
	})
	mux.HandleFunc("/domains", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			withRBAC(auth.PermEditDomains, domainsHandler.Create)(w, r)
		} else {
			domainsHandler.List(w, r)
		}
	})

	// Users routes - only available in multi-user mode, requires admin permission
	if usersHandler != nil {
		mux.HandleFunc("/users/", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Route based on path and method
			switch {
			case path == "/users/" || path == "/users":
				if r.Method == http.MethodPost {
					withRBAC(auth.PermManageUsers, usersHandler.Create)(w, r)
				} else {
					withRBAC(auth.PermViewUsers, usersHandler.List)(w, r)
				}
			case path == "/users/new":
				withRBAC(auth.PermManageUsers, usersHandler.New)(w, r)
			case strings.HasSuffix(path, "/edit"):
				withRBAC(auth.PermManageUsers, usersHandler.Edit)(w, r)
			case strings.HasSuffix(path, "/2fa"):
				// Disable 2FA for a user (DELETE method)
				if r.Method == http.MethodDelete {
					withRBAC(auth.PermManageUsers, usersHandler.Disable2FA)(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			default:
				// Handle PUT for updates, DELETE for removal
				switch r.Method {
				case http.MethodPut:
					withRBAC(auth.PermManageUsers, usersHandler.Update)(w, r)
				case http.MethodDelete:
					withRBAC(auth.PermManageUsers, usersHandler.Delete)(w, r)
				default:
					withRBAC(auth.PermViewUsers, usersHandler.List)(w, r)
				}
			}
		})
		mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				withRBAC(auth.PermManageUsers, usersHandler.Create)(w, r)
			} else {
				withRBAC(auth.PermViewUsers, usersHandler.List)(w, r)
			}
		})
	}

	// Profile routes - only available in multi-user mode
	if profileHandler != nil {
		mux.HandleFunc("/profile/", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			switch {
			case path == "/profile/" || path == "/profile":
				profileHandler.Show(w, r)
			case path == "/profile/password":
				if r.Method == http.MethodPut {
					profileHandler.UpdatePassword(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			case path == "/profile/notifications":
				if r.Method == http.MethodPut {
					profileHandler.UpdateNotificationPreferences(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			case path == "/profile/sessions/logout-others":
				if r.Method == http.MethodPost {
					profileHandler.LogoutOtherSessions(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			case strings.HasPrefix(path, "/profile/sessions/"):
				if r.Method == http.MethodDelete {
					profileHandler.LogoutSession(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			case path == "/profile/2fa" || path == "/profile/2fa/":
				if totpHandler != nil {
					totpHandler.Setup(w, r)
				} else {
					http.Error(w, "2FA not available", http.StatusNotFound)
				}
			case path == "/profile/2fa/verify":
				if totpHandler != nil && r.Method == http.MethodPost {
					totpHandler.Verify(w, r)
				} else if totpHandler == nil {
					http.Error(w, "2FA not available", http.StatusNotFound)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			case path == "/profile/2fa/disable":
				if totpHandler != nil && r.Method == http.MethodPost {
					totpHandler.Disable(w, r)
				} else if totpHandler == nil {
					http.Error(w, "2FA not available", http.StatusNotFound)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			case path == "/profile/2fa/regenerate-codes":
				if totpHandler != nil && r.Method == http.MethodPost {
					totpHandler.RegenerateBackupCodes(w, r)
				} else if totpHandler == nil {
					http.Error(w, "2FA not available", http.StatusNotFound)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			default:
				profileHandler.Show(w, r)
			}
		})
		mux.HandleFunc("/profile", profileHandler.Show)
	}

	// API Tokens routes - only available in multi-user mode
	if apiTokensHandler != nil {
		mux.HandleFunc("/api-tokens/", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			switch {
			case path == "/api-tokens/" || path == "/api-tokens":
				if r.Method == http.MethodPost {
					apiTokensHandler.Create(w, r)
				} else {
					apiTokensHandler.List(w, r)
				}
			case path == "/api-tokens/new":
				apiTokensHandler.New(w, r)
			case strings.HasSuffix(path, "/revoke"):
				if r.Method == http.MethodPost {
					apiTokensHandler.Revoke(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			default:
				if r.Method == http.MethodDelete {
					apiTokensHandler.Delete(w, r)
				} else {
					apiTokensHandler.List(w, r)
				}
			}
		})
		mux.HandleFunc("/api-tokens", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				apiTokensHandler.Create(w, r)
			} else {
				apiTokensHandler.List(w, r)
			}
		})
	}

	// Audit log route - admin only
	mux.HandleFunc("/audit", withRBAC(auth.PermViewAuditLog, auditHandler.List))

	// Apply auth middleware to protected routes
	authMiddlewareHandler := authMiddleware.Middleware()
	// Apply API rate limiting after auth (so we have user context for per-user limits)
	apiRateLimitHandler := rateLimiter.APIRateLimit()
	protectedHandler := authMiddlewareHandler(apiRateLimitHandler(mux))

	// Health check endpoints are NOT protected by auth
	// Simple health check for load balancers (backwards compatible)
	http.HandleFunc("/health", healthHandler.SimpleHealth)
	// Comprehensive health check with component statuses
	http.HandleFunc("/health/full", healthHandler.Health)

	// Metrics endpoint - optionally protected by auth
	if cfg.MetricsEnabled {
		if cfg.MetricsProtected {
			// Metrics endpoint protected by auth
			mux.HandleFunc("/metrics", metricsHandler.Metrics)
		} else {
			// Metrics endpoint NOT protected by auth (for Prometheus scraping)
			http.HandleFunc("/metrics", metricsHandler.Metrics)
		}
	}

	// Login and logout routes are NOT protected by auth
	// Apply rate limiting to login route
	loginHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authHandler.Login(w, r)
		} else {
			authHandler.LoginPage(w, r)
		}
	})
	http.Handle("/login", rateLimiter.LoginRateLimit()(loginHandler))

	// 2FA verification route (also rate limited)
	login2FAHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authHandler.Verify2FA(w, r)
		} else {
			// Redirect to login if accessed directly via GET
			http.Redirect(w, r, "/login", http.StatusFound)
		}
	})
	http.Handle("/login/2fa", rateLimiter.LoginRateLimit()(login2FAHandler))

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
	if authMiddleware.IsEnabled() {
		if cfg.MultiUserMode {
			log.Println("Multi-user authentication enabled")
		} else {
			log.Println("Session-based auth enabled")
		}
	} else {
		log.Println("Auth disabled (set CADDYSHACK_AUTH_USER and CADDYSHACK_AUTH_PASS to enable)")
	}
	if cfg.DockerEnabled {
		log.Printf("Docker integration enabled (socket: %s)", cfg.DockerSocket)
	} else {
		log.Println("Docker integration disabled (set CADDYSHACK_DOCKER_ENABLED=true to enable)")
	}
	if cfg.RateLimitEnabled {
		log.Printf("Rate limiting enabled (login: %d attempts/%ds, API: %d requests/%ds)",
			cfg.RateLimitLoginAttempts, cfg.RateLimitLoginWindow,
			cfg.RateLimitAPIRequests, cfg.RateLimitAPIWindow)
	} else {
		log.Println("Rate limiting disabled (set CADDYSHACK_RATE_LIMIT_ENABLED=true to enable)")
	}
	if cfg.MetricsEnabled {
		if cfg.MetricsProtected {
			log.Println("Prometheus metrics enabled at /metrics (auth protected)")
		} else {
			log.Println("Prometheus metrics enabled at /metrics (unprotected)")
		}
	} else {
		log.Println("Prometheus metrics disabled (set CADDYSHACK_METRICS_ENABLED=true to enable)")
	}
	log.Printf("Starting Caddyshack on port %s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
