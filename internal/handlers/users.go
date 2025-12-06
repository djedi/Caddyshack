package handlers

import (
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/djedi/caddyshack/internal/auth"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/middleware"
	"github.com/djedi/caddyshack/internal/templates"
)

// UserView represents a user for display in templates.
type UserView struct {
	ID              int64
	Username        string
	Email           string
	Role            auth.Role
	RoleDisplay     string
	CreatedAt       string
	LastLogin       string
	LastLoginText   string
	IsCurrentUser   bool
	CanDelete       bool
	TOTPEnabled     bool
}

// UsersData holds data displayed on the users list page.
type UsersData struct {
	Users          []UserView
	Error          string
	HasError       bool
	SuccessMessage string
	TotalCount     int
	AdminCount     int
	EditorCount    int
	ViewerCount    int
}

// UserFormData holds data for the user add/edit form.
type UserFormData struct {
	User            *UserFormValues
	Error           string
	HasError        bool
	IsEdit          bool
	Roles           []RoleOption
	IsCurrentUser   bool
}

// UserFormValues represents the form field values for creating/editing a user.
type UserFormValues struct {
	ID       int64
	Username string
	Email    string
	Role     string
	Password string
}

// RoleOption represents a role option for the select dropdown.
type RoleOption struct {
	Value    string
	Label    string
	Selected bool
}

// UsersHandler handles requests for the users pages.
type UsersHandler struct {
	templates    *templates.Templates
	config       *config.Config
	userStore    *auth.UserStore
	totpStore    *auth.TOTPStore
	errorHandler *ErrorHandler
}

// NewUsersHandler creates a new UsersHandler.
func NewUsersHandler(tmpl *templates.Templates, cfg *config.Config, userStore *auth.UserStore) *UsersHandler {
	return &UsersHandler{
		templates:    tmpl,
		config:       cfg,
		userStore:    userStore,
		totpStore:    auth.NewTOTPStore(userStore.DB()),
		errorHandler: NewErrorHandler(tmpl),
	}
}

// List handles GET requests for the users list page.
func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	data := UsersData{}

	// Check for success message from query params
	if successMsg := r.URL.Query().Get("success"); successMsg != "" {
		data.SuccessMessage = successMsg
	}

	// Get current user from context
	currentUser := getCurrentUser(r)

	// Get all users
	users, err := h.userStore.List()
	if err != nil {
		data.Error = "Failed to list users: " + err.Error()
		data.HasError = true
	} else {
		data.Users = make([]UserView, len(users))
		for i, u := range users {
			data.Users[i] = toUserView(u, currentUser)
			// Check TOTP status
			if h.totpStore != nil {
				enabled, _, _, _ := h.totpStore.GetTOTPStatus(u.ID)
				data.Users[i].TOTPEnabled = enabled
			}
			switch u.Role {
			case auth.RoleAdmin:
				data.AdminCount++
			case auth.RoleEditor:
				data.EditorCount++
			case auth.RoleViewer:
				data.ViewerCount++
			}
		}
		data.TotalCount = len(users)
	}

	// Check if this is an HTMX request for partial update
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "users-list.html", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	pageData := templates.PageData{
		Title:     "Users",
		ActiveNav: "users",
		Data:      data,
	}

	if err := h.templates.Render(w, "users.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// New handles GET requests for the new user form page.
func (h *UsersHandler) New(w http.ResponseWriter, r *http.Request) {
	data := UserFormData{
		User:  &UserFormValues{},
		IsEdit: false,
		Roles: getRoleOptions(""),
	}

	pageData := templates.PageData{
		Title:     "Add User",
		ActiveNav: "users",
		Data:      data,
	}

	if err := h.templates.Render(w, "user-new.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Create handles POST requests to create a new user.
func (h *UsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderFormError(w, r, "Failed to parse form data", nil, false, false)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")
	role := strings.TrimSpace(r.FormValue("role"))

	formValues := &UserFormValues{
		Username: username,
		Email:    email,
		Role:     role,
	}

	// Validate required fields
	if username == "" {
		h.renderFormError(w, r, "Username is required", formValues, false, false)
		return
	}

	if password == "" {
		h.renderFormError(w, r, "Password is required", formValues, false, false)
		return
	}

	if password != confirmPassword {
		h.renderFormError(w, r, "Passwords do not match", formValues, false, false)
		return
	}

	if len(password) < 8 {
		h.renderFormError(w, r, "Password must be at least 8 characters", formValues, false, false)
		return
	}

	// Validate role
	roleValue := auth.Role(role)
	if !roleValue.IsValid() {
		h.renderFormError(w, r, "Invalid role selected", formValues, false, false)
		return
	}

	// Create the user
	_, err := h.userStore.Create(username, email, password, roleValue)
	if err != nil {
		if err == auth.ErrUsernameExists {
			h.renderFormError(w, r, "A user with this username already exists", formValues, false, false)
			return
		}
		h.renderFormError(w, r, "Failed to create user: "+err.Error(), formValues, false, false)
		return
	}

	// Redirect to users list with success message
	w.Header().Set("HX-Redirect", "/users?success="+url.QueryEscape("User created successfully"))
	w.WriteHeader(http.StatusOK)
}

// Edit handles GET requests for the user edit form page.
func (h *UsersHandler) Edit(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from URL path (e.g., /users/123/edit)
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/users/")
	path = strings.TrimSuffix(path, "/edit")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		h.errorHandler.BadRequest(w, r, "Invalid user ID")
		return
	}

	user, err := h.userStore.GetByID(id)
	if err != nil {
		if err == auth.ErrUserNotFound {
			h.errorHandler.NotFound(w, r)
			return
		}
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	currentUser := getCurrentUser(r)
	isCurrentUser := currentUser != nil && currentUser.ID == user.ID

	formValues := &UserFormValues{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Role:     string(user.Role),
	}

	data := UserFormData{
		User:          formValues,
		IsEdit:        true,
		Roles:         getRoleOptions(string(user.Role)),
		IsCurrentUser: isCurrentUser,
	}

	pageData := templates.PageData{
		Title:     "Edit User - " + user.Username,
		ActiveNav: "users",
		Data:      data,
	}

	if err := h.templates.Render(w, "user-edit.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Update handles PUT requests to update an existing user.
func (h *UsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from URL path (e.g., /users/123)
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/users/")
	path = strings.TrimSuffix(path, "/")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		h.errorHandler.BadRequest(w, r, "Invalid user ID")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderFormError(w, r, "Failed to parse form data", nil, true, false)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	role := strings.TrimSpace(r.FormValue("role"))
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	formValues := &UserFormValues{
		ID:       id,
		Username: username,
		Email:    email,
		Role:     role,
	}

	currentUser := getCurrentUser(r)
	isCurrentUser := currentUser != nil && currentUser.ID == id

	// Validate required fields
	if username == "" {
		h.renderFormError(w, r, "Username is required", formValues, true, isCurrentUser)
		return
	}

	// Validate role
	roleValue := auth.Role(role)
	if !roleValue.IsValid() {
		h.renderFormError(w, r, "Invalid role selected", formValues, true, isCurrentUser)
		return
	}

	// If password is provided, validate it
	if password != "" {
		if password != confirmPassword {
			h.renderFormError(w, r, "Passwords do not match", formValues, true, isCurrentUser)
			return
		}
		if len(password) < 8 {
			h.renderFormError(w, r, "Password must be at least 8 characters", formValues, true, isCurrentUser)
			return
		}
	}

	// Check if user is trying to demote themselves
	if isCurrentUser && roleValue != currentUser.Role {
		h.renderFormError(w, r, "You cannot change your own role", formValues, true, isCurrentUser)
		return
	}

	// Get existing user
	user, err := h.userStore.GetByID(id)
	if err != nil {
		if err == auth.ErrUserNotFound {
			h.errorHandler.NotFound(w, r)
			return
		}
		h.renderFormError(w, r, "Failed to get user: "+err.Error(), formValues, true, isCurrentUser)
		return
	}

	// Update user info
	if err := h.userStore.Update(id, username, email, roleValue); err != nil {
		if err == auth.ErrUsernameExists {
			h.renderFormError(w, r, "A user with this username already exists", formValues, true, isCurrentUser)
			return
		}
		h.renderFormError(w, r, "Failed to update user: "+err.Error(), formValues, true, isCurrentUser)
		return
	}

	// Update password if provided
	if password != "" {
		if err := h.userStore.UpdatePassword(id, password); err != nil {
			h.renderFormError(w, r, "Failed to update password: "+err.Error(), formValues, true, isCurrentUser)
			return
		}
	}

	// Redirect to users list with success message
	successMsg := "User updated successfully"
	if user.Username != username {
		successMsg = "User " + username + " updated successfully"
	}
	w.Header().Set("HX-Redirect", "/users?success="+url.QueryEscape(successMsg))
	w.WriteHeader(http.StatusOK)
}

// Delete handles DELETE requests to remove a user.
func (h *UsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from URL path (e.g., /users/123)
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/users/")
	path = strings.TrimSuffix(path, "/")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		h.errorHandler.BadRequest(w, r, "Invalid user ID")
		return
	}

	// Check if user is trying to delete themselves
	currentUser := getCurrentUser(r)
	if currentUser != nil && currentUser.ID == id {
		h.errorHandler.BadRequest(w, r, "You cannot delete your own account")
		return
	}

	// Get user to check if they exist
	user, err := h.userStore.GetByID(id)
	if err != nil {
		if err == auth.ErrUserNotFound {
			h.errorHandler.NotFound(w, r)
			return
		}
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Delete all user sessions first
	if err := h.userStore.DeleteUserSessions(id); err != nil {
		log.Printf("Warning: failed to delete user sessions: %v", err)
	}

	// Delete the user
	if err := h.userStore.Delete(id); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// For HTMX requests, redirect to refresh the list
	if isHTMXRequest(r) {
		w.Header().Set("HX-Redirect", "/users?success="+url.QueryEscape("User '"+user.Username+"' deleted successfully"))
		w.WriteHeader(http.StatusOK)
		return
	}

	// For regular requests, redirect to users list
	http.Redirect(w, r, "/users?success="+url.QueryEscape("User '"+user.Username+"' deleted successfully"), http.StatusFound)
}

// Disable2FA disables two-factor authentication for a user (admin only).
func (h *UsersHandler) Disable2FA(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from URL path (e.g., /users/123/2fa)
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/users/")
	path = strings.TrimSuffix(path, "/2fa")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		h.errorHandler.BadRequest(w, r, "Invalid user ID")
		return
	}

	// Get user to check if they exist
	user, err := h.userStore.GetByID(id)
	if err != nil {
		if err == auth.ErrUserNotFound {
			h.errorHandler.NotFound(w, r)
			return
		}
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Disable 2FA
	if h.totpStore != nil {
		if err := h.totpStore.DisableTOTP(id); err != nil && err != auth.ErrUserNotFound {
			h.errorHandler.InternalServerError(w, r, err)
			return
		}
	}

	// For HTMX requests, return success message
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<span class="text-green-600 dark:text-green-400 text-sm">2FA disabled</span>`))
		return
	}

	// For regular requests, redirect to users list
	http.Redirect(w, r, "/users?success="+url.QueryEscape("2FA disabled for user '"+user.Username+"'"), http.StatusFound)
}

// toUserView converts a User to a UserView with display information.
func toUserView(u *auth.User, currentUser *auth.User) UserView {
	view := UserView{
		ID:       u.ID,
		Username: u.Username,
		Email:    u.Email,
		Role:     u.Role,
		CreatedAt: u.CreatedAt.Format("Jan 2, 2006"),
	}

	// Role display name
	switch u.Role {
	case auth.RoleAdmin:
		view.RoleDisplay = "Administrator"
	case auth.RoleEditor:
		view.RoleDisplay = "Editor"
	case auth.RoleViewer:
		view.RoleDisplay = "Viewer"
	default:
		view.RoleDisplay = string(u.Role)
	}

	// Last login
	if u.LastLogin != nil {
		view.LastLogin = u.LastLogin.Format("Jan 2, 2006 3:04 PM")
		view.LastLoginText = view.LastLogin
	} else {
		view.LastLoginText = "Never"
	}

	// Check if this is the current user
	if currentUser != nil {
		view.IsCurrentUser = currentUser.ID == u.ID
		// Can delete if not current user
		view.CanDelete = !view.IsCurrentUser
	}

	return view
}

// getRoleOptions returns role options for the select dropdown.
func getRoleOptions(selectedRole string) []RoleOption {
	return []RoleOption{
		{Value: string(auth.RoleAdmin), Label: "Administrator", Selected: selectedRole == string(auth.RoleAdmin)},
		{Value: string(auth.RoleEditor), Label: "Editor", Selected: selectedRole == string(auth.RoleEditor)},
		{Value: string(auth.RoleViewer), Label: "Viewer", Selected: selectedRole == string(auth.RoleViewer)},
	}
}

// getCurrentUser retrieves the current user from the request context.
func getCurrentUser(r *http.Request) *auth.User {
	return middleware.GetUserFromContext(r.Context())
}

// renderFormError renders the form with an error message.
func (h *UsersHandler) renderFormError(w http.ResponseWriter, r *http.Request, errMsg string, formValues *UserFormValues, isEdit bool, isCurrentUser bool) {
	log.Printf("User form error: %s", errMsg)

	if formValues == nil {
		formValues = &UserFormValues{}
	}

	data := UserFormData{
		User:          formValues,
		Error:         errMsg,
		HasError:      true,
		IsEdit:        isEdit,
		Roles:         getRoleOptions(formValues.Role),
		IsCurrentUser: isCurrentUser,
	}

	// For HTMX requests, return just the form partial
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "user-form.html", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	// For regular requests, render the full page
	templateName := "user-new.html"
	title := "Add User"
	if isEdit {
		templateName = "user-edit.html"
		title = "Edit User"
	}

	pageData := templates.PageData{
		Title:     title,
		ActiveNav: "users",
		Data:      data,
	}

	if err := h.templates.Render(w, templateName, pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}
