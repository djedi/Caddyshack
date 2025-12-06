// Package middleware provides HTTP middleware for authentication and authorization.
package middleware

import (
	"net/http"

	"github.com/djedi/caddyshack/internal/auth"
)

// RBAC provides role-based access control middleware and helpers.
type RBAC struct {
	// MultiUserMode indicates if the system is in multi-user mode.
	// When false, all authenticated users are treated as admins.
	MultiUserMode bool
}

// NewRBAC creates a new RBAC instance.
func NewRBAC(multiUserMode bool) *RBAC {
	return &RBAC{
		MultiUserMode: multiUserMode,
	}
}

// RequireAdmin returns middleware that requires admin role.
func (r *RBAC) RequireAdmin() func(http.Handler) http.Handler {
	return RequireRole(auth.RoleAdmin)
}

// RequireEditor returns middleware that requires editor or admin role.
func (r *RBAC) RequireEditor() func(http.Handler) http.Handler {
	return RequireRole(auth.RoleAdmin, auth.RoleEditor)
}

// RequireViewer returns middleware that allows any authenticated user (admin, editor, or viewer).
func (r *RBAC) RequireViewer() func(http.Handler) http.Handler {
	return RequireRole(auth.RoleAdmin, auth.RoleEditor, auth.RoleViewer)
}

// RequireEditSites returns middleware that requires permission to edit sites.
func (r *RBAC) RequireEditSites() func(http.Handler) http.Handler {
	return RequirePermission(auth.PermEditSites)
}

// RequireEditSnippets returns middleware that requires permission to edit snippets.
func (r *RBAC) RequireEditSnippets() func(http.Handler) http.Handler {
	return RequirePermission(auth.PermEditSnippets)
}

// RequireEditGlobal returns middleware that requires permission to edit global settings.
func (r *RBAC) RequireEditGlobal() func(http.Handler) http.Handler {
	return RequirePermission(auth.PermEditGlobal)
}

// RequireManageUsers returns middleware that requires permission to manage users.
func (r *RBAC) RequireManageUsers() func(http.Handler) http.Handler {
	return RequirePermission(auth.PermManageUsers)
}

// RequireImportExport returns middleware that requires permission for import/export.
func (r *RBAC) RequireImportExport() func(http.Handler) http.Handler {
	return RequirePermission(auth.PermImportExport)
}

// RequireRestoreHistory returns middleware that requires permission to restore history.
func (r *RBAC) RequireRestoreHistory() func(http.Handler) http.Handler {
	return RequirePermission(auth.PermRestoreHistory)
}

// RequireEditDomains returns middleware that requires permission to edit domains.
func (r *RBAC) RequireEditDomains() func(http.Handler) http.Handler {
	return RequirePermission(auth.PermEditDomains)
}

// RequireManageContainers returns middleware that requires permission to manage containers.
func (r *RBAC) RequireManageContainers() func(http.Handler) http.Handler {
	return RequirePermission(auth.PermManageContainers)
}

// RequireManageNotifications returns middleware that requires permission to manage notifications.
func (r *RBAC) RequireManageNotifications() func(http.Handler) http.Handler {
	return RequirePermission(auth.PermManageNotifications)
}

// MethodBasedRBAC returns middleware that applies different permission checks based on HTTP method.
// GET/HEAD requests require viewPerm, while POST/PUT/PATCH/DELETE require editPerm.
func (r *RBAC) MethodBasedRBAC(viewPerm, editPerm auth.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			user := GetUserFromContext(req.Context())
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Determine which permission to check based on method
			var requiredPerm auth.Permission
			switch req.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				requiredPerm = viewPerm
			default:
				requiredPerm = editPerm
			}

			if !user.Role.HasPermission(requiredPerm) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, req)
		})
	}
}

// CanView checks if the user from context has view permission for the given resource.
func CanView(r *http.Request, perm auth.Permission) bool {
	user := GetUserFromContext(r.Context())
	if user == nil {
		return false
	}
	return user.Role.HasPermission(perm)
}

// CanEdit checks if the user from context has edit permission for the given resource.
func CanEdit(r *http.Request, perm auth.Permission) bool {
	user := GetUserFromContext(r.Context())
	if user == nil {
		return false
	}
	return user.Role.HasPermission(perm)
}

// GetUserRole returns the role of the user from context, or empty string if not found.
func GetUserRole(r *http.Request) auth.Role {
	user := GetUserFromContext(r.Context())
	if user == nil {
		return ""
	}
	return user.Role
}

// IsAdmin returns true if the user from context is an admin.
func IsAdmin(r *http.Request) bool {
	return GetUserRole(r) == auth.RoleAdmin
}

// IsEditor returns true if the user from context is an editor (not admin).
func IsEditor(r *http.Request) bool {
	return GetUserRole(r) == auth.RoleEditor
}

// IsViewer returns true if the user from context is a viewer.
func IsViewer(r *http.Request) bool {
	return GetUserRole(r) == auth.RoleViewer
}

// CanEditSites returns true if the user can edit sites.
func CanEditSites(r *http.Request) bool {
	return CanEdit(r, auth.PermEditSites)
}

// CanEditSnippets returns true if the user can edit snippets.
func CanEditSnippets(r *http.Request) bool {
	return CanEdit(r, auth.PermEditSnippets)
}

// CanEditGlobal returns true if the user can edit global settings.
func CanEditGlobal(r *http.Request) bool {
	return CanEdit(r, auth.PermEditGlobal)
}

// CanManageUsers returns true if the user can manage users.
func CanManageUsers(r *http.Request) bool {
	return CanEdit(r, auth.PermManageUsers)
}

// CanRestoreHistory returns true if the user can restore history.
func CanRestoreHistory(r *http.Request) bool {
	return CanEdit(r, auth.PermRestoreHistory)
}

// CanImportExport returns true if the user can import/export configuration.
func CanImportExport(r *http.Request) bool {
	return CanEdit(r, auth.PermImportExport)
}

// CanEditDomains returns true if the user can edit domains.
func CanEditDomains(r *http.Request) bool {
	return CanEdit(r, auth.PermEditDomains)
}

// CanManageContainers returns true if the user can manage containers.
func CanManageContainers(r *http.Request) bool {
	return CanEdit(r, auth.PermManageContainers)
}

// CanManageNotifications returns true if the user can manage notifications.
func CanManageNotifications(r *http.Request) bool {
	return CanEdit(r, auth.PermManageNotifications)
}

// UserPermissions holds the permission state for a user, suitable for passing to templates.
type UserPermissions struct {
	Role auth.Role

	// View permissions
	CanViewDashboard     bool
	CanViewSites         bool
	CanViewSnippets      bool
	CanViewGlobal        bool
	CanViewHistory       bool
	CanViewLogs          bool
	CanViewCerts         bool
	CanViewContainers    bool
	CanViewDomains       bool
	CanViewNotifications bool
	CanViewUsers         bool
	CanViewAuditLog      bool

	// Edit permissions
	CanEditSites            bool
	CanEditSnippets         bool
	CanEditGlobal           bool
	CanEditDomains          bool
	CanRestoreHistory       bool
	CanImportExport         bool
	CanManageUsers          bool
	CanManageContainers     bool
	CanManageNotifications  bool

	// Convenience flags
	IsAdmin     bool
	IsEditor    bool
	IsViewer    bool
	CanEdit     bool // Can edit sites or snippets
	IsMultiUser bool // Whether multi-user mode is enabled
}

// globalMultiUserMode stores whether the application is running in multi-user mode.
// This is set once during initialization via SetMultiUserMode.
var globalMultiUserMode bool

// SetMultiUserMode sets the global multi-user mode flag.
// This should be called once during application initialization.
func SetMultiUserMode(enabled bool) {
	globalMultiUserMode = enabled
}

// IsMultiUserMode returns whether the application is running in multi-user mode.
func IsMultiUserMode() bool {
	return globalMultiUserMode
}

// GetUserPermissions returns the permissions for the user from context.
// This is useful for passing permission data to templates.
func GetUserPermissions(r *http.Request) *UserPermissions {
	return GetUserPermissionsWithMultiUser(r, globalMultiUserMode)
}

// GetUserPermissionsWithMultiUser returns the permissions for the user from context,
// with the IsMultiUser flag set based on the provided value.
func GetUserPermissionsWithMultiUser(r *http.Request, multiUserMode bool) *UserPermissions {
	user := GetUserFromContext(r.Context())
	if user == nil {
		// Return empty permissions if no user
		return &UserPermissions{IsMultiUser: multiUserMode}
	}

	role := user.Role
	return &UserPermissions{
		Role: role,

		// View permissions
		CanViewDashboard:     role.HasPermission(auth.PermViewDashboard),
		CanViewSites:         role.HasPermission(auth.PermViewSites),
		CanViewSnippets:      role.HasPermission(auth.PermViewSnippets),
		CanViewGlobal:        role.HasPermission(auth.PermViewGlobal),
		CanViewHistory:       role.HasPermission(auth.PermViewHistory),
		CanViewLogs:          role.HasPermission(auth.PermViewLogs),
		CanViewCerts:         role.HasPermission(auth.PermViewCerts),
		CanViewContainers:    role.HasPermission(auth.PermViewContainers),
		CanViewDomains:       role.HasPermission(auth.PermViewDomains),
		CanViewNotifications: role.HasPermission(auth.PermViewNotifications),
		CanViewUsers:         role.HasPermission(auth.PermViewUsers),
		CanViewAuditLog:      role.HasPermission(auth.PermViewAuditLog),

		// Edit permissions
		CanEditSites:           role.HasPermission(auth.PermEditSites),
		CanEditSnippets:        role.HasPermission(auth.PermEditSnippets),
		CanEditGlobal:          role.HasPermission(auth.PermEditGlobal),
		CanEditDomains:         role.HasPermission(auth.PermEditDomains),
		CanRestoreHistory:      role.HasPermission(auth.PermRestoreHistory),
		CanImportExport:        role.HasPermission(auth.PermImportExport),
		CanManageUsers:         role.HasPermission(auth.PermManageUsers),
		CanManageContainers:    role.HasPermission(auth.PermManageContainers),
		CanManageNotifications: role.HasPermission(auth.PermManageNotifications),

		// Convenience flags
		IsAdmin:     role == auth.RoleAdmin,
		IsEditor:    role == auth.RoleEditor,
		IsViewer:    role == auth.RoleViewer,
		CanEdit:     role.CanEdit(),
		IsMultiUser: multiUserMode,
	}
}
