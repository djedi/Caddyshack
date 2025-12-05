package handlers

import (
	"log"
	"net/http"

	"github.com/djedi/caddyshack/internal/templates"
)

// ErrorData holds data for error pages and error message partials.
type ErrorData struct {
	Title   string
	Message string
	Details string
}

// ErrorHandler provides methods for rendering consistent error responses.
type ErrorHandler struct {
	templates *templates.Templates
}

// NewErrorHandler creates a new ErrorHandler.
func NewErrorHandler(tmpl *templates.Templates) *ErrorHandler {
	return &ErrorHandler{
		templates: tmpl,
	}
}

// RenderError renders a full error page for non-HTMX requests.
func (h *ErrorHandler) RenderError(w http.ResponseWriter, r *http.Request, statusCode int, title, message, details string) {
	logError(r, statusCode, title, message, details)

	w.WriteHeader(statusCode)

	// For HTMX requests, return an error message partial
	if isHTMXRequest(r) {
		h.renderHTMXError(w, title, message, details)
		return
	}

	// For regular requests, render the full error page
	pageData := templates.PageData{
		Title:     title,
		ActiveNav: "",
		Data: ErrorData{
			Title:   title,
			Message: message,
			Details: details,
		},
	}

	if err := h.templates.Render(w, "error.html", pageData); err != nil {
		// Fallback to plain text if template rendering fails
		log.Printf("Error rendering error page: %v", err)
		http.Error(w, message, statusCode)
	}
}

// renderHTMXError renders an error message partial for HTMX swapping.
func (h *ErrorHandler) renderHTMXError(w http.ResponseWriter, title, message, details string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := ErrorData{
		Title:   title,
		Message: message,
		Details: details,
	}

	if err := h.templates.RenderPartial(w, "error-message", data); err != nil {
		log.Printf("Error rendering HTMX error partial: %v", err)
		// Fallback to a simple HTML error
		w.Write([]byte(`<div class="bg-red-50 border border-red-200 rounded p-4 text-red-800">` + message + `</div>`))
	}
}

// NotFound renders a 404 Not Found error page.
func (h *ErrorHandler) NotFound(w http.ResponseWriter, r *http.Request) {
	h.RenderError(w, r, http.StatusNotFound,
		"Page Not Found",
		"The page you're looking for doesn't exist or has been moved.",
		"")
}

// InternalServerError renders a 500 Internal Server Error page.
func (h *ErrorHandler) InternalServerError(w http.ResponseWriter, r *http.Request, err error) {
	details := ""
	if err != nil {
		details = err.Error()
	}
	h.RenderError(w, r, http.StatusInternalServerError,
		"Internal Server Error",
		"Something went wrong on our end. Please try again later.",
		details)
}

// BadRequest renders a 400 Bad Request error page.
func (h *ErrorHandler) BadRequest(w http.ResponseWriter, r *http.Request, message string) {
	h.RenderError(w, r, http.StatusBadRequest,
		"Bad Request",
		message,
		"")
}

// MethodNotAllowed renders a 405 Method Not Allowed error page.
func (h *ErrorHandler) MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	h.RenderError(w, r, http.StatusMethodNotAllowed,
		"Method Not Allowed",
		"The requested method is not supported for this resource.",
		"")
}

// logError logs error information with request context.
func logError(r *http.Request, statusCode int, title, message, details string) {
	logMsg := "HTTP %d - %s: %s [%s %s]"
	args := []interface{}{statusCode, title, message, r.Method, r.URL.Path}

	if details != "" {
		logMsg += " - Details: %s"
		args = append(args, details)
	}

	if r.Header.Get("HX-Request") == "true" {
		logMsg += " (HTMX)"
	}

	log.Printf(logMsg, args...)
}

// HTTPError is a convenience function for quick error responses.
// It writes an error with appropriate content type for HTMX or plain text.
func HTTPError(w http.ResponseWriter, r *http.Request, statusCode int, message string) {
	log.Printf("HTTP %d: %s [%s %s]", statusCode, message, r.Method, r.URL.Path)

	w.WriteHeader(statusCode)

	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<div class="bg-red-50 border border-red-200 rounded p-4 text-red-800">` + message + `</div>`))
		return
	}

	http.Error(w, message, statusCode)
}
