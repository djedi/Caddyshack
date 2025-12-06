package handlers

import (
	"net/http"

	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/middleware"
	"github.com/djedi/caddyshack/internal/templates"
)

// WithPermissions creates PageData with the user's permissions from the request context.
// This should be used by handlers when rendering templates to include permission data.
func WithPermissions(r *http.Request, title, activeNav string, data any) templates.PageData {
	return templates.PageData{
		Title:       title,
		ActiveNav:   activeNav,
		Data:        data,
		Permissions: middleware.GetUserPermissions(r),
	}
}

// WithPermissionsAndConfig creates PageData with the user's permissions, including multi-user mode flag.
// This should be used by handlers that have access to config and need to include multi-user mode info.
func WithPermissionsAndConfig(r *http.Request, cfg *config.Config, title, activeNav string, data any) templates.PageData {
	return templates.PageData{
		Title:       title,
		ActiveNav:   activeNav,
		Data:        data,
		Permissions: middleware.GetUserPermissionsWithMultiUser(r, cfg.MultiUserMode),
	}
}

// GetPermissions returns the user's permissions from the request context.
// This can be used by handlers that need to check permissions or include them in data.
func GetPermissions(r *http.Request) *middleware.UserPermissions {
	return middleware.GetUserPermissions(r)
}

// GetPermissionsWithConfig returns the user's permissions, including multi-user mode flag.
func GetPermissionsWithConfig(r *http.Request, cfg *config.Config) *middleware.UserPermissions {
	return middleware.GetUserPermissionsWithMultiUser(r, cfg.MultiUserMode)
}
