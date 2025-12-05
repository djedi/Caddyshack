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

- [ ] Create login form as alternative to browser basic auth prompt
- [ ] Session-based auth with secure cookie
- [ ] Logout functionality

---

## Phase 10: Polish and Production

### Task 10.1: Error Handling

- [ ] Create error page template
- [ ] Consistent error responses for HTMX (swap error message)
- [ ] Log errors appropriately

### Task 10.2: Loading States

- [ ] Add HTMX loading indicators
- [ ] Disable buttons during form submission
- [ ] Skeleton loaders for async content

### Task 10.3: Production Build

- [ ] Embed templates and static files in binary
- [ ] Optimize Tailwind for production (purge unused)
- [ ] Add health check endpoint
- [ ] Document deployment process in README

### Task 10.4: Testing

- [ ] Unit tests for parser and writer (80%+ coverage)
- [ ] Integration tests for handlers
- [ ] End-to-end test with real Caddy container

---

## Future Phases (V2+)

These are documented in prompt.md under Feature Ideas V2/V3:

- Snippet management UI
- Certificate status display
- Import existing Caddyfile wizard
- Log viewer
- Docker container status
- Multi-user support
