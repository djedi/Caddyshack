# Caddyshack Task List

Check off tasks as completed. Each task should result in working, testable code.

---

## Phase 0: Project Setup

### Task 0.1: Initialize Go Module and Project Structure

- [x] Create `go.mod` with module `github.com/djedi/caddyshack`
- [x] Create directory structure: `cmd/caddyshack/`, `internal/`, `templates/`, `static/`
- [x] Create minimal `cmd/caddyshack/main.go` that starts an HTTP server on port 8080
- [x] Verify with `go run ./cmd/caddyshack` - should respond to requests

### Task 0.2: Open Source Files

- [x] Create `LICENSE` file (MIT)
- [x] Create `README.md` with project description, features, and installation instructions
- [x] Create `.gitignore` for Go projects (binaries, .env, \*.db, node_modules)

### Task 0.3: Docker Setup

- [x] Create `Dockerfile` (multi-stage build: Go build + minimal runtime)
- [x] Create `docker-compose.dev.yml` for local development (caddyshack + caddy container)
- [x] Add `Makefile` with common commands (build, run, docker-build, docker-up)

### Task 0.4: Tailwind CSS Setup

- [x] Create `package.json` with tailwindcss as dev dependency
- [x] Create `tailwind.config.js` configured for Go templates
- [x] Create `static/css/input.css` with Tailwind directives
- [x] Generate `static/css/output.css`
- [x] Add npm scripts for watch and build

---

## Phase 1: Basic UI Shell

### Task 1.1: Base Template Layout

- [x] Create `templates/layouts/base.html` with:
  - HTML5 boilerplate
  - Tailwind CSS link
  - HTMX script (CDN)
  - Alpine.js script (CDN)
  - Sidebar navigation placeholder
  - Main content area with `{{ block "content" . }}{{ end }}`
- [x] Create `internal/templates/templates.go` to load and render templates
- [x] Update main.go to serve a test page using the base layout

### Task 1.2: Static File Serving

- [x] Add route to serve `/static/` files from `static/` directory
- [x] Embed static files in binary using `embed.FS` for production

### Task 1.3: Dashboard Page

- [x] Create `templates/pages/dashboard.html` extending base layout
- [x] Create `internal/handlers/dashboard.go` with handler
- [x] Display placeholder cards for: "Sites", "Snippets", "Status"
- [x] Style with Tailwind (clean, minimal admin UI)

---

## Phase 2: Configuration Management

### Task 2.1: Environment Configuration

- [x] Create `internal/config/config.go`
- [x] Load config from environment variables (see CLAUDE.md for list)
- [x] Provide sensible defaults for local development
- [x] Pass config to handlers via dependency injection

### Task 2.2: SQLite Database Setup

- [x] Add `modernc.org/sqlite` dependency (pure Go SQLite)
- [x] Create `internal/store/store.go` with database initialization
- [x] Create schema for `config_history` table (id, timestamp, content, comment)
- [x] Add migration system (simple version table + SQL files or embedded strings)

---

## Phase 3: Caddyfile Parsing

### Task 3.1: Basic Caddyfile Reader

- [x] Create `internal/caddy/reader.go`
- [x] Function to read Caddyfile from configured path
- [x] Return raw content as string
- [x] Handle file not found gracefully

### Task 3.2: Site Block Parser

- [x] Create `internal/caddy/parser.go`
- [x] Define `Site` struct: Domain, Directives []Directive, Imports []string
- [x] Parse site blocks (domain { ... }) from Caddyfile
- [x] Extract domain name and raw content of each site
- [x] Write unit tests with example Caddyfile from prompt.md

### Task 3.3: Snippet Parser

- [x] Define `Snippet` struct: Name, Content
- [x] Parse snippet definitions `(name) { ... }`
- [x] Store snippets separately from sites
- [x] Write unit tests

### Task 3.4: Global Options Parser

- [x] Define `GlobalOptions` struct: Email, LogConfig, etc.
- [x] Parse the global options block `{ ... }` at start of file
- [x] Write unit tests

---

## Phase 4: Sites List UI

### Task 4.1: Sites Index Page

- [x] Create `templates/pages/sites.html`
- [x] Create `internal/handlers/sites.go` with List handler
- [x] Display all parsed sites as cards showing domain and basic info
- [x] Add navigation link to sidebar

### Task 4.2: Site Card Partial

- [x] Create `templates/partials/site-card.html`
- [x] Display: domain, reverse proxy target (if applicable), imported snippets
- [x] Add Edit and Delete buttons (non-functional for now)
- [x] Style with Tailwind

### Task 4.3: Site Detail View

- [x] Create `templates/pages/site-detail.html`
- [x] Show full configuration for a single site
- [x] Display raw Caddyfile block with syntax highlighting (optional)
- [x] Link from site card to detail view

---

## Phase 5: Caddyfile Generation

### Task 5.1: Site Block Writer

- [x] Create `internal/caddy/writer.go`
- [x] Function to generate Caddyfile site block from Site struct
- [x] Maintain proper indentation and formatting
- [x] Write unit tests (parse -> write -> compare)

### Task 5.2: Full Caddyfile Writer

- [x] Function to generate complete Caddyfile from all components
- [x] Order: global options, snippets, sites
- [x] Preserve comments where possible

### Task 5.3: Caddyfile Validator

- [x] Create `internal/caddy/validator.go`
- [x] Shell out to `caddy validate --config /path` or use Admin API
- [x] Return validation errors in structured format
- [x] Write integration test

---

## Phase 6: Site CRUD Operations

### Task 6.1: Add Site Form

- [x] Create `templates/partials/site-form.html`
- [x] Form fields: domain, type (reverse_proxy/static/redirect), target
- [x] Use Alpine.js to show/hide fields based on type selection
- [x] HTMX form submission to POST /sites

### Task 6.2: Create Site Handler

- [x] Add POST /sites handler
- [x] Validate input
- [x] Add site to parsed config
- [x] Regenerate and validate Caddyfile
- [x] Save to file
- [x] Return updated site list (HTMX swap)

### Task 6.3: Edit Site

- [x] Create edit form (reuse site-form partial)
- [x] GET /sites/{domain}/edit returns form with current values
- [x] PUT /sites/{domain} updates the site
- [x] Validate and save

### Task 6.4: Delete Site

- [x] Add DELETE /sites/{domain} handler
- [x] Confirmation modal using Alpine.js
- [x] Remove site from config
- [x] Regenerate Caddyfile
- [x] Return updated site list

---

## Phase 7: Caddy Integration

### Task 7.1: Caddy Admin API Client

- [x] Create `internal/caddy/admin.go`
- [x] Function to reload config: POST to /load endpoint
- [x] Function to get current config: GET /config/
- [x] Function to check Caddy status
- [x] Handle connection errors gracefully

### Task 7.2: Reload After Changes

- [x] After successful Caddyfile save, trigger reload
- [x] Display reload status in UI (success/failure)
- [x] Show error details if reload fails

### Task 7.3: Status Dashboard Widget

- [x] Add Caddy status to dashboard
- [x] Show: running/stopped, uptime, version
- [x] Auto-refresh with HTMX polling (every 30s)

---

## Phase 8: Config History

### Task 8.1: Save Config History

- [x] Before each Caddyfile change, save current version to SQLite
- [x] Store: timestamp, full content, change description
- [x] Limit history to last 50 versions (configurable)

### Task 8.2: History View

- [x] Create `templates/pages/history.html`
- [x] List recent config changes with timestamps
- [x] Show diff between versions (simple text diff)

### Task 8.3: Rollback Feature

- [x] Add "Restore" button to history entries
- [x] Restore selected version as current Caddyfile
- [x] Validate before applying
- [x] Reload Caddy

---

## Phase 9: Authentication

### Task 9.1: Basic Auth Middleware

- [x] Create `internal/middleware/auth.go`
- [x] Implement HTTP Basic Auth
- [x] Read credentials from config
- [x] Apply to all routes except /health

### Task 9.2: Login Page

- [x] Create login form as alternative to browser basic auth prompt
- [x] Session-based auth with secure cookie
- [x] Logout functionality

---

## Phase 10: Polish and Production

### Task 10.1: Error Handling

- [x] Create error page template
- [x] Consistent error responses for HTMX (swap error message)
- [x] Log errors appropriately

### Task 10.2: Loading States

- [x] Add HTMX loading indicators
- [x] Disable buttons during form submission
- [x] Skeleton loaders for async content

### Task 10.3: Production Build

- [x] Embed templates and static files in binary
- [x] Optimize Tailwind for production (purge unused)
- [x] Add health check endpoint
- [x] Document deployment process in README

### Task 10.4: Testing

- [x] Unit tests for parser and writer (80%+ coverage)
- [x] Integration tests for handlers
- [x] End-to-end test with real Caddy container

---

## Phase 11: Snippet Management UI (V2)

### Task 11.1: Snippets Index Page

- [x] Create `templates/pages/snippets.html`
- [x] Create `internal/handlers/snippets.go` with List handler
- [x] Display all parsed snippets as cards showing name and preview
- [x] Add navigation link to sidebar
- [x] Wire up routes in main.go

### Task 11.2: Snippet Card Partial

- [x] Create `templates/partials/snippet-card.html`
- [x] Display: name, content preview (first few lines), usage count
- [x] Add Edit and Delete buttons
- [x] Style with Tailwind consistent with site cards

### Task 11.3: Snippet Detail View

- [x] Create `templates/pages/snippet-detail.html`
- [x] Show full snippet content with syntax highlighting
- [x] List sites that use this snippet (via import)
- [x] Link from snippet card to detail view

### Task 11.4: Add Snippet Form

- [x] Create `templates/partials/snippet-form.html`
- [x] Form fields: name (identifier), content (textarea)
- [x] Syntax validation on submit
- [x] HTMX form submission to POST /snippets

### Task 11.5: Snippet CRUD Handlers

- [x] Add POST /snippets handler (create)
- [x] Add GET /snippets/{name}/edit handler (edit form)
- [x] Add PUT /snippets/{name} handler (update)
- [x] Add DELETE /snippets/{name} handler
- [x] Validate snippet syntax before saving
- [x] Regenerate Caddyfile and reload Caddy

### Task 11.6: Snippet Tests

- [x] Unit tests for snippet CRUD handlers
- [x] Integration tests for snippet routes
- [x] Test snippet creation, editing, deletion flow

---

## Phase 12: Import/Export Configuration

### Task 12.1: Export Caddyfile

- [x] Create `internal/handlers/export.go` with export handler
- [x] Add `GET /export` route that downloads current Caddyfile as file
- [x] Add `GET /export/json` route that returns config as JSON (using Caddy Admin API)
- [x] Create export button in dashboard or settings area
- [x] Include timestamp in filename (e.g., `caddyfile-2024-01-15.txt`)

### Task 12.2: Import Caddyfile UI

- [x] Create `templates/pages/import.html` with file upload form
- [x] Create `internal/handlers/import.go` with import handler
- [x] Add navigation link to sidebar (under settings or tools section)
- [x] Support file upload or paste text content
- [x] Preview parsed config before applying

### Task 12.3: Import Validation and Apply

- [x] Parse uploaded/pasted Caddyfile using existing parser
- [x] Validate syntax using `caddy validate` or Admin API
- [x] Show validation errors with line numbers
- [x] Show preview of sites and snippets that will be imported
- [x] Create "Apply Import" action that saves and reloads

### Task 12.4: Backup All Configuration

- [x] Create handler to export full backup (Caddyfile + history)
- [x] Package as downloadable JSON or ZIP file
- [x] Include: current Caddyfile, config history, timestamps
- [x] Add backup button to history page or dashboard

---

## Phase 13: Certificate Status Display

### Task 13.1: Caddy PKI API Client

- [x] Extend `internal/caddy/admin.go` with certificate methods
- [x] GET `/pki/ca/local` for CA info (if using internal CA)
- [x] Parse certificate info from Caddy's config JSON
- [x] Handle cases where ACME/certificates aren't configured

### Task 13.2: Certificate Status Page

- [x] Create `templates/pages/certificates.html`
- [x] Create `internal/handlers/certificates.go` with handler
- [x] Display: domain, issuer, expiry date, status (valid/expiring/expired)
- [x] Add navigation link to sidebar
- [x] Color code by status (green=valid, yellow=expiring soon, red=expired)

### Task 13.3: Certificate Status Widget

- [x] Add certificate summary to dashboard
- [x] Show count of valid, expiring, and expired certificates
- [x] Link to full certificates page
- [x] Auto-refresh with HTMX polling

### Task 13.4: Certificate Expiry Warnings

- [x] Highlight certificates expiring within 30 days
- [x] Add warning banner when certificates need attention
- [x] Show in site detail view if that site's cert is expiring

---

## Phase 14: Global Options Editor

### Task 14.1: Global Options Page

- [x] Create `templates/pages/global-options.html`
- [x] Create `internal/handlers/global.go` with handler
- [x] Display current global options (email, logging, admin, debug, etc.)
- [x] Add navigation link to sidebar

### Task 14.2: Global Options Edit Form

- [x] Create form to edit common global options
- [x] Fields: email, admin address, debug mode, log format
- [x] Advanced section for raw block editing
- [x] Validate and save changes

### Task 14.3: Log Configuration Editor

- [x] UI to configure global logging settings
- [x] Log output path, format (json/console), roll settings
- [x] Preview generated Caddyfile block
- [x] Save and reload

---

## Phase 15: Log Viewer (Basic)

### Task 15.1: Log File Reader

- [x] Create `internal/handlers/logs.go` with log handler
- [x] Read last N lines from configured Caddy log file
- [x] Support configurable log path (from config or auto-detect from Caddyfile)
- [x] Handle log file not found gracefully

### Task 15.2: Logs Page

- [x] Create `templates/pages/logs.html`
- [x] Display recent log entries in scrollable view
- [x] Parse JSON log format for readable display
- [x] Show: timestamp, level, message, domain (if applicable)
- [x] Add navigation link to sidebar

### Task 15.3: Log Filtering

- [x] Filter logs by level (error, warn, info)
- [x] Filter by domain/site
- [x] Search within log entries
- [x] HTMX partial refresh for filters

### Task 15.4: Log Auto-Refresh

- [x] Add auto-refresh toggle (poll every 5 seconds)
- [x] Scroll to bottom on new entries (optional)
- [x] Pause auto-refresh when user scrolls up
- [x] Show "new entries" indicator

---

## Phase 16: Docker Container Status

### Task 16.1: Docker API Client

- [x] Create `internal/docker/client.go` with Docker client
- [x] Connect to Docker socket (configurable path)
- [x] Function to list running containers
- [x] Function to get container status by name or ID
- [x] Handle Docker not available gracefully (optional feature)

### Task 16.2: Container Status Page

- [x] Create `templates/pages/containers.html`
- [x] Create `internal/handlers/containers.go` with handler
- [x] Display: container name, status (running/stopped), image, ports
- [x] Add navigation link to sidebar
- [x] Color code by status (green=running, red=stopped)

### Task 16.3: Link Containers to Sites

- [x] Parse reverse_proxy targets to identify potential container hosts
- [x] Match container ports to proxy targets
- [x] Show container status in site detail view
- [ ] Add indicator on site cards for container health (optional, deferred)

### Task 16.4: Container Status Widget

- [x] Add container summary to dashboard
- [x] Show count of running, stopped, and unhealthy containers
- [x] Link to full containers page
- [x] Auto-refresh with HTMX polling

### Task 16.5: Container Actions (Optional)

- [ ] Add start/stop/restart buttons for containers
- [ ] Confirmation modal for container actions
- [ ] Log output for container actions
- [ ] Require admin permissions for container control

---

## Phase 17: Notification System

### Task 17.1: Notification Infrastructure

- [x] Create `internal/notifications/notification.go` with notification types and interfaces
- [x] Define Notification struct: Type, Severity, Title, Message, Timestamp, Acknowledged
- [x] Create notification storage in SQLite (notifications table with type, severity, data, created_at, ack_at)
- [x] Add notification service for creating, listing, and acknowledging notifications

### Task 17.2: Notification UI

- [x] Create `templates/pages/notifications.html` for notification center
- [x] Create `internal/handlers/notifications.go` with handlers
- [x] Add notification bell icon to header with unread count badge
- [x] Create notification dropdown/panel showing recent notifications
- [x] Add "Mark as read" and "Mark all as read" functionality
- [x] Add navigation link to full notification history

### Task 17.3: Certificate Expiry Notifications

- [x] Create background job to check certificate expiry daily
- [x] Generate notification when certificate expires within 30 days (warning)
- [x] Generate notification when certificate expires within 7 days (critical)
- [x] Generate notification when certificate has expired (error)
- [x] Link notification to certificate details page
- [x] Avoid duplicate notifications for same certificate/threshold

### Task 17.4: Email Notification Support

- [x] Create `internal/notifications/email.go` with SMTP client
- [x] Add email configuration to config.go (SMTP host, port, user, password, from address)
- [x] Create email templates for notifications
- [x] Add email preferences per notification type (UI setting)
- [x] Send email for critical notifications (configurable)

### Task 17.5: Webhook Notifications

- [x] Create `internal/notifications/webhook.go` for webhook delivery
- [x] Add webhook URL configuration (supports multiple endpoints)
- [x] POST notification data as JSON to configured webhooks
- [x] Support webhook headers for authentication
- [x] Retry failed webhook deliveries with exponential backoff

---

## Phase 18: Domain Management

### Task 18.1: Domain Tracking

- [x] Create domains table in SQLite (domain, registrar, expiry_date, notes, created_at)
- [x] Create `internal/handlers/domains.go` with CRUD handlers
- [x] Create `templates/pages/domains.html` for domain list
- [x] Auto-detect domains from Caddyfile sites
- [x] Allow manual domain entry with registrar and expiry info

### Task 18.2: Domain Expiry Notifications

- [x] Add expiry tracking for registered domains
- [x] Background job to check domain expiry dates
- [x] Generate notification when domain expires within 60 days (warning)
- [x] Generate notification when domain expires within 14 days (critical)
- [x] Link notification to domain details

### Task 18.3: WHOIS Integration (Optional)

- [ ] Create `internal/domains/whois.go` for WHOIS lookups
- [ ] Lookup domain expiry date automatically
- [ ] Cache WHOIS results to avoid rate limiting
- [ ] Button to refresh WHOIS data manually
- [ ] Parse registrar and nameserver info

---

## Phase 19: Multi-User Support

### Task 19.1: User Model and Storage

- [x] Create users table in SQLite (id, username, email, password_hash, role, created_at, last_login)
- [x] Create `internal/auth/user.go` with User model and password hashing (bcrypt)
- [x] Define roles: admin, editor, viewer
- [x] Role permissions: admin (all), editor (CRUD sites/snippets), viewer (read-only)
- [x] Migrate from basic auth to session-based auth

### Task 19.2: User Management UI

- [x] Create `templates/pages/users.html` for user list (admin only)
- [x] Create `internal/handlers/users.go` with CRUD handlers
- [x] Add user creation form with role selection
- [x] Add user edit form (change password, role)
- [x] Add user deletion with confirmation
- [x] Only admins can manage users

### Task 19.3: Role-Based Access Control

- [x] Create `internal/middleware/rbac.go` for role checking
- [x] Protect routes based on required role
- [x] Hide UI elements based on user role
- [x] Viewer role: read-only access, no edit/delete buttons
- [x] Editor role: can edit sites/snippets, cannot manage users or global settings
- [x] Admin role: full access

### Task 19.4: User Profile and Settings

- [x] Create `templates/pages/profile.html` for current user settings
- [x] Allow users to change their own password
- [x] Notification preferences per user
- [x] Theme preference (if dark mode is implemented) - skipped, dark mode not yet implemented
- [x] Session management (list active sessions, logout other sessions)

### Task 19.5: Audit Log

- [x] Create audit_log table (user_id, action, resource_type, resource_id, details, timestamp)
- [x] Log all configuration changes with user attribution
- [x] Create `templates/pages/audit.html` to view audit log (admin only)
- [x] Filter by user, action type, date range
- [x] Link audit entries to relevant resources

---

## Phase 20: UI Enhancements

### Task 20.1: Dark Mode

- [ ] Add dark mode CSS variants with Tailwind dark: modifier
- [ ] Add theme toggle in header (light/dark/system)
- [ ] Store preference in localStorage or user profile
- [ ] Respect system preference by default

### Task 20.2: Dashboard Customization

- [ ] Allow reordering of dashboard widgets
- [ ] Allow hiding/showing widgets
- [ ] Collapsible widget sections
- [ ] Store layout preference per user

### Task 20.3: Keyboard Shortcuts

- [ ] Add keyboard shortcuts for common actions
- [ ] `?` to show shortcuts help modal
- [ ] `n` for new site, `s` for sites, `d` for dashboard
- [ ] `Escape` to close modals
- [ ] Use Alpine.js for shortcut handling

### Task 20.4: Search

- [ ] Add global search in header
- [ ] Search across sites, snippets, logs
- [ ] Quick navigation results (cmd+k style)
- [ ] Recent searches history

---

## Future Phases (V4+)

Ideas for future development:

- API tokens for programmatic access
- Two-factor authentication (TOTP)
- Rate limiting for login attempts
- Caddy metrics integration (Prometheus)
- Performance monitoring dashboard
- Mobile-responsive improvements
- Import from other proxy configurations (nginx, traefik)
- Scheduled Caddyfile backups
- Git integration for config versioning
