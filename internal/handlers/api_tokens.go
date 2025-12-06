package handlers

import (
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/djedi/caddyshack/internal/auth"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/middleware"
	"github.com/djedi/caddyshack/internal/templates"
)

// APITokenView represents an API token for display in templates.
type APITokenView struct {
	ID            int64
	Name          string
	Scopes        []string
	ScopesDisplay string
	CreatedAt     string
	ExpiresAt     string
	ExpiresAtText string
	LastUsedAt    string
	LastUsedText  string
	IsExpired     bool
	IsRevoked     bool
	IsActive      bool
	CanDelete     bool
}

// APITokensData holds data displayed on the API tokens list page.
type APITokensData struct {
	Tokens         []APITokenView
	ActiveCount    int
	RevokedCount   int
	ExpiredCount   int
	Error          string
	HasError       bool
	SuccessMessage string
	NewToken       string // Shown only once when a token is created
}

// APITokenFormData holds data for the token creation form.
type APITokenFormData struct {
	Name          string
	Scopes        []ScopeOption
	ExpiresIn     string
	Error         string
	HasError      bool
}

// ScopeOption represents a scope option for checkboxes.
type ScopeOption struct {
	Value       string
	Label       string
	Description string
	Checked     bool
}

// APITokensHandler handles requests for the API tokens pages.
type APITokensHandler struct {
	templates    *templates.Templates
	config       *config.Config
	tokenStore   *auth.TokenStore
	errorHandler *ErrorHandler
}

// NewAPITokensHandler creates a new APITokensHandler.
func NewAPITokensHandler(tmpl *templates.Templates, cfg *config.Config, tokenStore *auth.TokenStore) *APITokensHandler {
	return &APITokensHandler{
		templates:    tmpl,
		config:       cfg,
		tokenStore:   tokenStore,
		errorHandler: NewErrorHandler(tmpl),
	}
}

// List handles GET requests for the API tokens list page.
func (h *APITokensHandler) List(w http.ResponseWriter, r *http.Request) {
	data := APITokensData{}

	// Check for success message from query params
	if successMsg := r.URL.Query().Get("success"); successMsg != "" {
		data.SuccessMessage = successMsg
	}

	// Check for new token from query params (shown only once)
	if newToken := r.URL.Query().Get("new_token"); newToken != "" {
		data.NewToken = newToken
	}

	// Get current user from context
	currentUser := middleware.GetUserFromContext(r.Context())
	if currentUser == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	// Get all tokens for the user
	tokens, err := h.tokenStore.ListByUser(currentUser.ID)
	if err != nil {
		data.Error = "Failed to list tokens: " + err.Error()
		data.HasError = true
	} else {
		data.Tokens = make([]APITokenView, len(tokens))
		for i, t := range tokens {
			data.Tokens[i] = toAPITokenView(t)
			if t.IsRevoked() {
				data.RevokedCount++
			} else if t.IsExpired() {
				data.ExpiredCount++
			} else {
				data.ActiveCount++
			}
		}
	}

	// Check if this is an HTMX request for partial update
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "api-tokens-list.html", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	pageData := templates.PageData{
		Title:     "API Tokens",
		ActiveNav: "profile",
		Data:      data,
	}

	if err := h.templates.Render(w, "api-tokens.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// New handles GET requests for the new token form page.
func (h *APITokensHandler) New(w http.ResponseWriter, r *http.Request) {
	data := APITokenFormData{
		Scopes: getScopeOptions(nil),
	}

	pageData := templates.PageData{
		Title:     "Create API Token",
		ActiveNav: "profile",
		Data:      data,
	}

	if err := h.templates.Render(w, "api-token-new.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Create handles POST requests to create a new token.
func (h *APITokensHandler) Create(w http.ResponseWriter, r *http.Request) {
	currentUser := middleware.GetUserFromContext(r.Context())
	if currentUser == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderFormError(w, r, "Failed to parse form data", "", nil, "")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	scopeValues := r.Form["scopes"]
	expiresIn := r.FormValue("expires_in")

	// Validate name
	if name == "" {
		h.renderFormError(w, r, "Token name is required", name, scopeValues, expiresIn)
		return
	}

	if len(name) > 100 {
		h.renderFormError(w, r, "Token name must be 100 characters or less", name, scopeValues, expiresIn)
		return
	}

	// Validate scopes
	if len(scopeValues) == 0 {
		h.renderFormError(w, r, "At least one scope must be selected", name, scopeValues, expiresIn)
		return
	}

	scopes := make([]auth.TokenScope, len(scopeValues))
	for i, s := range scopeValues {
		scope := auth.TokenScope(s)
		if !scope.IsValid() {
			h.renderFormError(w, r, "Invalid scope: "+s, name, scopeValues, expiresIn)
			return
		}
		scopes[i] = scope
	}

	// Calculate expiration time
	var expiresAt *time.Time
	switch expiresIn {
	case "7d":
		t := time.Now().Add(7 * 24 * time.Hour)
		expiresAt = &t
	case "30d":
		t := time.Now().Add(30 * 24 * time.Hour)
		expiresAt = &t
	case "90d":
		t := time.Now().Add(90 * 24 * time.Hour)
		expiresAt = &t
	case "365d":
		t := time.Now().Add(365 * 24 * time.Hour)
		expiresAt = &t
	case "never":
		expiresAt = nil
	default:
		h.renderFormError(w, r, "Invalid expiration option", name, scopeValues, expiresIn)
		return
	}

	// Create the token
	rawToken, _, err := h.tokenStore.Create(currentUser.ID, name, scopes, expiresAt)
	if err != nil {
		if err == auth.ErrTokenNameExists {
			h.renderFormError(w, r, "A token with this name already exists", name, scopeValues, expiresIn)
			return
		}
		h.renderFormError(w, r, "Failed to create token: "+err.Error(), name, scopeValues, expiresIn)
		return
	}

	// Redirect to tokens list with the new token displayed
	redirectURL := "/api-tokens?success=" + url.QueryEscape("Token created successfully") + "&new_token=" + url.QueryEscape(rawToken)
	w.Header().Set("HX-Redirect", redirectURL)
	w.WriteHeader(http.StatusOK)
}

// Revoke handles POST requests to revoke a token.
func (h *APITokensHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	currentUser := middleware.GetUserFromContext(r.Context())
	if currentUser == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	// Extract token ID from URL path (e.g., /api-tokens/123/revoke)
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/api-tokens/")
	path = strings.TrimSuffix(path, "/revoke")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		h.errorHandler.BadRequest(w, r, "Invalid token ID")
		return
	}

	// Verify the token belongs to the current user
	token, err := h.tokenStore.GetByID(id)
	if err != nil {
		if err == auth.ErrTokenNotFound {
			h.errorHandler.NotFound(w, r)
			return
		}
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	if token.UserID != currentUser.ID {
		h.errorHandler.Forbidden(w, r)
		return
	}

	// Revoke the token
	if err := h.tokenStore.Revoke(id); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Redirect to tokens list
	if isHTMXRequest(r) {
		w.Header().Set("HX-Redirect", "/api-tokens?success="+url.QueryEscape("Token '"+token.Name+"' revoked successfully"))
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, "/api-tokens?success="+url.QueryEscape("Token '"+token.Name+"' revoked successfully"), http.StatusFound)
}

// Delete handles DELETE requests to permanently delete a token.
func (h *APITokensHandler) Delete(w http.ResponseWriter, r *http.Request) {
	currentUser := middleware.GetUserFromContext(r.Context())
	if currentUser == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	// Extract token ID from URL path (e.g., /api-tokens/123)
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/api-tokens/")
	path = strings.TrimSuffix(path, "/")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		h.errorHandler.BadRequest(w, r, "Invalid token ID")
		return
	}

	// Verify the token belongs to the current user
	token, err := h.tokenStore.GetByID(id)
	if err != nil {
		if err == auth.ErrTokenNotFound {
			h.errorHandler.NotFound(w, r)
			return
		}
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	if token.UserID != currentUser.ID {
		h.errorHandler.Forbidden(w, r)
		return
	}

	// Delete the token
	if err := h.tokenStore.Delete(id); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Redirect to tokens list
	if isHTMXRequest(r) {
		w.Header().Set("HX-Redirect", "/api-tokens?success="+url.QueryEscape("Token '"+token.Name+"' deleted successfully"))
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, "/api-tokens?success="+url.QueryEscape("Token '"+token.Name+"' deleted successfully"), http.StatusFound)
}

// toAPITokenView converts an APIToken to an APITokenView.
func toAPITokenView(t *auth.APIToken) APITokenView {
	view := APITokenView{
		ID:        t.ID,
		Name:      t.Name,
		CreatedAt: t.CreatedAt.Format("Jan 2, 2006 3:04 PM"),
		IsExpired: t.IsExpired(),
		IsRevoked: t.IsRevoked(),
		IsActive:  t.IsValid(),
		CanDelete: true,
	}

	// Scopes
	view.Scopes = make([]string, len(t.Scopes))
	for i, s := range t.Scopes {
		view.Scopes[i] = string(s)
	}
	view.ScopesDisplay = strings.Join(view.Scopes, ", ")

	// Expiration
	if t.ExpiresAt != nil {
		view.ExpiresAt = t.ExpiresAt.Format("Jan 2, 2006")
		if t.IsExpired() {
			view.ExpiresAtText = "Expired on " + view.ExpiresAt
		} else {
			view.ExpiresAtText = "Expires " + view.ExpiresAt
		}
	} else {
		view.ExpiresAtText = "Never expires"
	}

	// Last used
	if t.LastUsedAt != nil {
		view.LastUsedAt = t.LastUsedAt.Format("Jan 2, 2006 3:04 PM")
		view.LastUsedText = view.LastUsedAt
	} else {
		view.LastUsedText = "Never used"
	}

	return view
}

// getScopeOptions returns scope options for checkboxes.
func getScopeOptions(selected []string) []ScopeOption {
	options := []ScopeOption{
		{
			Value:       string(auth.ScopeRead),
			Label:       "Read",
			Description: "Read access to sites, snippets, and configuration",
			Checked:     sliceContainsString(selected, string(auth.ScopeRead)),
		},
		{
			Value:       string(auth.ScopeWrite),
			Label:       "Write",
			Description: "Create, update, and delete sites and snippets",
			Checked:     sliceContainsString(selected, string(auth.ScopeWrite)),
		},
		{
			Value:       string(auth.ScopeAdmin),
			Label:       "Admin",
			Description: "Full administrative access including user management",
			Checked:     sliceContainsString(selected, string(auth.ScopeAdmin)),
		},
	}
	return options
}

// sliceContainsString checks if a slice contains a string.
func sliceContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// renderFormError renders the form with an error message.
func (h *APITokensHandler) renderFormError(w http.ResponseWriter, r *http.Request, errMsg, name string, scopes []string, expiresIn string) {
	log.Printf("API token form error: %s", errMsg)

	data := APITokenFormData{
		Name:      name,
		Scopes:    getScopeOptions(scopes),
		ExpiresIn: expiresIn,
		Error:     errMsg,
		HasError:  true,
	}

	// For HTMX requests, return just the form partial
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "api-token-form.html", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	pageData := templates.PageData{
		Title:     "Create API Token",
		ActiveNav: "profile",
		Data:      data,
	}

	if err := h.templates.Render(w, "api-token-new.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}
