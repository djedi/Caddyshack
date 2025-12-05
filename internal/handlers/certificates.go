package handlers

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/templates"
)

// CertificatesData holds data displayed on the certificates page.
type CertificatesData struct {
	Certificates   []CertificateView
	CAInfo         *caddy.CAInfo
	Error          string
	HasError       bool
	Summary        CertificateSummary
	CaddyReachable bool
}

// CertificateView is a view model for certificate information.
type CertificateView struct {
	Domain        string
	Issuer        string
	NotBefore     string
	NotAfter      string
	Status        string // "valid", "expiring", "expired", "unknown"
	StatusColor   string // Tailwind color class
	DaysRemaining int
}

// CertificateSummary provides aggregate certificate statistics.
type CertificateSummary struct {
	Total    int
	Valid    int
	Expiring int
	Expired  int
	Unknown  int
}

// CertificatesHandler handles requests for the certificates pages.
type CertificatesHandler struct {
	templates    *templates.Templates
	adminClient  *caddy.AdminClient
	errorHandler *ErrorHandler
}

// NewCertificatesHandler creates a new CertificatesHandler.
func NewCertificatesHandler(tmpl *templates.Templates, cfg *config.Config) *CertificatesHandler {
	return &CertificatesHandler{
		templates:    tmpl,
		adminClient:  caddy.NewAdminClient(cfg.CaddyAdminAPI),
		errorHandler: NewErrorHandler(tmpl),
	}
}

// List handles GET requests for the certificates list page.
func (h *CertificatesHandler) List(w http.ResponseWriter, r *http.Request) {
	data := CertificatesData{}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Check if Caddy is reachable
	status, _ := h.adminClient.GetStatus(ctx)
	data.CaddyReachable = status != nil && status.Running

	if !data.CaddyReachable {
		data.Error = "Unable to connect to Caddy Admin API. Please ensure Caddy is running."
		data.HasError = true
	} else {
		// Get CA info
		caInfo, err := h.adminClient.GetPKICAInfo(ctx, "local")
		if err != nil {
			// Non-fatal - PKI might not be configured
			data.CAInfo = nil
		} else {
			data.CAInfo = caInfo
		}

		// Get certificate info
		certs, err := h.adminClient.GetCertificates(ctx)
		if err != nil {
			data.Error = "Failed to retrieve certificate information: " + err.Error()
			data.HasError = true
		} else {
			// Convert to view models and calculate summary
			data.Certificates = make([]CertificateView, 0, len(certs))
			for _, cert := range certs {
				view := certificateToView(cert)
				data.Certificates = append(data.Certificates, view)

				// Update summary
				data.Summary.Total++
				switch view.Status {
				case "valid":
					data.Summary.Valid++
				case "expiring":
					data.Summary.Expiring++
				case "expired":
					data.Summary.Expired++
				default:
					data.Summary.Unknown++
				}
			}

			// Sort certificates by domain
			sort.Slice(data.Certificates, func(i, j int) bool {
				return data.Certificates[i].Domain < data.Certificates[j].Domain
			})
		}
	}

	pageData := templates.PageData{
		Title:     "Certificates",
		ActiveNav: "certificates",
		Data:      data,
	}

	if err := h.templates.Render(w, "certificates.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Widget handles GET requests for the certificate status widget (for HTMX polling).
func (h *CertificatesHandler) Widget(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	summary := CertificateSummary{}

	// Check if Caddy is reachable
	status, _ := h.adminClient.GetStatus(ctx)
	caddyReachable := status != nil && status.Running

	if caddyReachable {
		// Get certificate info
		certs, err := h.adminClient.GetCertificates(ctx)
		if err == nil {
			for _, cert := range certs {
				summary.Total++
				switch cert.Status {
				case "valid":
					summary.Valid++
				case "expiring":
					summary.Expiring++
				case "expired":
					summary.Expired++
				default:
					summary.Unknown++
				}
			}
		}
	}

	data := struct {
		CaddyReachable bool
		Summary        CertificateSummary
	}{
		CaddyReachable: caddyReachable,
		Summary:        summary,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "certificate-widget.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// certificateToView converts a CertificateInfo to a CertificateView.
func certificateToView(cert caddy.CertificateInfo) CertificateView {
	view := CertificateView{
		Domain:        cert.Domain,
		Issuer:        cert.Issuer,
		DaysRemaining: cert.DaysRemaining,
		Status:        cert.Status,
	}

	// Format dates if available
	if !cert.NotBefore.IsZero() {
		view.NotBefore = cert.NotBefore.Format("Jan 02, 2006")
	}
	if !cert.NotAfter.IsZero() {
		view.NotAfter = cert.NotAfter.Format("Jan 02, 2006")
	}

	// Set status color based on certificate status
	switch cert.Status {
	case "valid":
		view.StatusColor = "green"
	case "expiring":
		view.StatusColor = "yellow"
	case "expired":
		view.StatusColor = "red"
	default:
		view.Status = "unknown"
		view.StatusColor = "gray"
	}

	return view
}
