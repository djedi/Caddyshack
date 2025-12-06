package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djedi/caddyshack/internal/auth"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/middleware"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

func setupUsersTestHandler(t *testing.T) (*UsersHandler, *auth.UserStore) {
	t.Helper()

	// Create a temporary directory for the database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Find the templates directory relative to the test file
	templatesDir := "../../templates"

	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	cfg := &config.Config{
		MultiUserMode: true,
	}

	// Initialize the store for testing
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	t.Cleanup(func() {
		s.Close()
	})

	userStore := auth.NewUserStore(s.DB())
	handler := NewUsersHandler(tmpl, cfg, userStore)
	return handler, userStore
}

// addUserToContext adds a user to the request context
func addUserToContext(r *http.Request, user *auth.User) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserContextKey, user)
	return r.WithContext(ctx)
}

func TestUsersList_Empty(t *testing.T) {
	handler, _ := setupUsersTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Users") {
		t.Error("Response should contain 'Users' page title")
	}
}

func TestUsersList_WithUsers(t *testing.T) {
	handler, userStore := setupUsersTestHandler(t)

	// Create test users
	_, err := userStore.Create("admin", "admin@test.com", "password123", auth.RoleAdmin)
	if err != nil {
		t.Fatalf("Failed to create admin user: %v", err)
	}
	_, err = userStore.Create("editor", "editor@test.com", "password123", auth.RoleEditor)
	if err != nil {
		t.Fatalf("Failed to create editor user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "admin") {
		t.Error("Response should contain 'admin'")
	}
	if !strings.Contains(body, "editor") {
		t.Error("Response should contain 'editor'")
	}
}

func TestUsersList_WithSuccessMessage(t *testing.T) {
	handler, _ := setupUsersTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/users?success=User+created", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "User created") {
		t.Error("Response should contain success message")
	}
}

func TestUsersNew_Success(t *testing.T) {
	handler, _ := setupUsersTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/users/new", nil)
	rec := httptest.NewRecorder()

	handler.New(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Add") {
		t.Error("Response should contain 'Add'")
	}
}

func TestUsersCreate_Valid(t *testing.T) {
	handler, userStore := setupUsersTestHandler(t)

	form := url.Values{}
	form.Set("username", "newuser")
	form.Set("email", "newuser@test.com")
	form.Set("password", "securepass123")
	form.Set("confirm_password", "securepass123")
	form.Set("role", "editor")

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	// Check for HX-Redirect header (indicates success)
	redirect := rec.Header().Get("HX-Redirect")
	if !strings.HasPrefix(redirect, "/users") {
		t.Errorf("Expected HX-Redirect to /users, got %q", redirect)
		t.Logf("Response body: %s", rec.Body.String())
	}

	// Verify the user was created
	user, err := userStore.GetByUsername("newuser")
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	if user == nil {
		t.Error("User should have been created")
	}
	if user.Email != "newuser@test.com" {
		t.Errorf("Expected email 'newuser@test.com', got %q", user.Email)
	}
	if user.Role != auth.RoleEditor {
		t.Errorf("Expected role 'editor', got %q", user.Role)
	}
}

func TestUsersCreate_MissingUsername(t *testing.T) {
	handler, _ := setupUsersTestHandler(t)

	form := url.Values{}
	form.Set("email", "test@test.com")
	form.Set("password", "password123")
	form.Set("confirm_password", "password123")
	form.Set("role", "viewer")

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	// Should NOT redirect on error
	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect on validation error")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Username is required") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestUsersCreate_MissingPassword(t *testing.T) {
	handler, _ := setupUsersTestHandler(t)

	form := url.Values{}
	form.Set("username", "testuser")
	form.Set("email", "test@test.com")
	form.Set("role", "viewer")

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect on validation error")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Password is required") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestUsersCreate_PasswordMismatch(t *testing.T) {
	handler, _ := setupUsersTestHandler(t)

	form := url.Values{}
	form.Set("username", "testuser")
	form.Set("email", "test@test.com")
	form.Set("password", "password123")
	form.Set("confirm_password", "differentpassword")
	form.Set("role", "viewer")

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect on validation error")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Passwords do not match") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestUsersCreate_PasswordTooShort(t *testing.T) {
	handler, _ := setupUsersTestHandler(t)

	form := url.Values{}
	form.Set("username", "testuser")
	form.Set("email", "test@test.com")
	form.Set("password", "short")
	form.Set("confirm_password", "short")
	form.Set("role", "viewer")

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect on validation error")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "at least 8 characters") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestUsersCreate_InvalidRole(t *testing.T) {
	handler, _ := setupUsersTestHandler(t)

	form := url.Values{}
	form.Set("username", "testuser")
	form.Set("email", "test@test.com")
	form.Set("password", "password123")
	form.Set("confirm_password", "password123")
	form.Set("role", "invalid_role")

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect on validation error")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Invalid role") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestUsersCreate_DuplicateUsername(t *testing.T) {
	handler, userStore := setupUsersTestHandler(t)

	// Create an existing user
	_, err := userStore.Create("existinguser", "existing@test.com", "password123", auth.RoleViewer)
	if err != nil {
		t.Fatalf("Failed to create existing user: %v", err)
	}

	// Try to create a user with the same username
	form := url.Values{}
	form.Set("username", "existinguser")
	form.Set("email", "new@test.com")
	form.Set("password", "password123")
	form.Set("confirm_password", "password123")
	form.Set("role", "viewer")

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect when username already exists")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "already exists") {
		t.Errorf("Response should contain error message about duplicate, got: %s", body)
	}
}

func TestUsersEdit_Success(t *testing.T) {
	handler, userStore := setupUsersTestHandler(t)

	// Create a test user
	user, err := userStore.Create("testuser", "test@test.com", "password123", auth.RoleEditor)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/users/1/edit", nil)
	rec := httptest.NewRecorder()

	// Simulate URL path with user ID
	req.URL.Path = "/users/" + itoa(user.ID) + "/edit"

	handler.Edit(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "testuser") {
		t.Error("Response should contain 'testuser'")
	}
}

func TestUsersEdit_NotFound(t *testing.T) {
	handler, _ := setupUsersTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/users/9999/edit", nil)
	rec := httptest.NewRecorder()

	handler.Edit(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestUsersEdit_InvalidID(t *testing.T) {
	handler, _ := setupUsersTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/users/invalid/edit", nil)
	rec := httptest.NewRecorder()

	handler.Edit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}
}

func TestUsersUpdate_Valid(t *testing.T) {
	handler, userStore := setupUsersTestHandler(t)

	// Create a test user
	user, err := userStore.Create("testuser", "test@test.com", "password123", auth.RoleEditor)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	form := url.Values{}
	form.Set("username", "updateduser")
	form.Set("email", "updated@test.com")
	form.Set("role", "admin")

	req := httptest.NewRequest(http.MethodPut, "/users/"+itoa(user.ID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Update(rec, req)

	redirect := rec.Header().Get("HX-Redirect")
	if !strings.HasPrefix(redirect, "/users") {
		t.Errorf("Expected HX-Redirect to /users, got %q", redirect)
		t.Logf("Response body: %s", rec.Body.String())
	}

	// Verify the user was updated
	updatedUser, err := userStore.GetByID(user.ID)
	if err != nil {
		t.Fatalf("Failed to get updated user: %v", err)
	}
	if updatedUser.Username != "updateduser" {
		t.Errorf("Expected username 'updateduser', got %q", updatedUser.Username)
	}
	if updatedUser.Email != "updated@test.com" {
		t.Errorf("Expected email 'updated@test.com', got %q", updatedUser.Email)
	}
	if updatedUser.Role != auth.RoleAdmin {
		t.Errorf("Expected role 'admin', got %q", updatedUser.Role)
	}
}

func TestUsersUpdate_WithPassword(t *testing.T) {
	handler, userStore := setupUsersTestHandler(t)

	// Create a test user
	user, err := userStore.Create("testuser", "test@test.com", "password123", auth.RoleEditor)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	form := url.Values{}
	form.Set("username", "testuser")
	form.Set("email", "test@test.com")
	form.Set("role", "editor")
	form.Set("password", "newpassword123")
	form.Set("confirm_password", "newpassword123")

	req := httptest.NewRequest(http.MethodPut, "/users/"+itoa(user.ID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Update(rec, req)

	redirect := rec.Header().Get("HX-Redirect")
	if !strings.HasPrefix(redirect, "/users") {
		t.Errorf("Expected HX-Redirect to /users, got %q", redirect)
	}

	// Verify password was changed by trying to authenticate
	_, err = userStore.Authenticate("testuser", "newpassword123")
	if err != nil {
		t.Error("Should be able to authenticate with new password")
	}
}

func TestUsersUpdate_NotFound(t *testing.T) {
	handler, _ := setupUsersTestHandler(t)

	form := url.Values{}
	form.Set("username", "updateduser")
	form.Set("email", "updated@test.com")
	form.Set("role", "admin")

	req := httptest.NewRequest(http.MethodPut, "/users/9999", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Update(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestUsersUpdate_CannotChangeOwnRole(t *testing.T) {
	handler, userStore := setupUsersTestHandler(t)

	// Create a test user
	user, err := userStore.Create("testuser", "test@test.com", "password123", auth.RoleAdmin)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	form := url.Values{}
	form.Set("username", "testuser")
	form.Set("email", "test@test.com")
	form.Set("role", "viewer") // Trying to demote self

	req := httptest.NewRequest(http.MethodPut, "/users/"+itoa(user.ID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	// Add current user to context
	req = addUserToContext(req, user)

	rec := httptest.NewRecorder()
	handler.Update(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect when trying to change own role")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "cannot change your own role") {
		t.Errorf("Response should contain error message about changing own role, got: %s", body)
	}
}

func TestUsersDelete_Valid(t *testing.T) {
	handler, userStore := setupUsersTestHandler(t)

	// Create users
	currentUser, err := userStore.Create("currentuser", "current@test.com", "password123", auth.RoleAdmin)
	if err != nil {
		t.Fatalf("Failed to create current user: %v", err)
	}
	userToDelete, err := userStore.Create("todelete", "todelete@test.com", "password123", auth.RoleViewer)
	if err != nil {
		t.Fatalf("Failed to create user to delete: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/users/"+itoa(userToDelete.ID), nil)
	req.Header.Set("HX-Request", "true")
	req = addUserToContext(req, currentUser)

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	redirect := rec.Header().Get("HX-Redirect")
	if !strings.HasPrefix(redirect, "/users") {
		t.Errorf("Expected HX-Redirect to /users, got %q", redirect)
	}

	// Verify the user was deleted
	_, err = userStore.GetByID(userToDelete.ID)
	if err != auth.ErrUserNotFound {
		t.Error("User should have been deleted")
	}
}

func TestUsersDelete_CannotDeleteSelf(t *testing.T) {
	handler, userStore := setupUsersTestHandler(t)

	// Create a user
	user, err := userStore.Create("testuser", "test@test.com", "password123", auth.RoleAdmin)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/users/"+itoa(user.ID), nil)
	req.Header.Set("HX-Request", "true")
	req = addUserToContext(req, user)

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "cannot delete your own account") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestUsersDelete_NotFound(t *testing.T) {
	handler, userStore := setupUsersTestHandler(t)

	// Create a current user
	currentUser, err := userStore.Create("currentuser", "current@test.com", "password123", auth.RoleAdmin)
	if err != nil {
		t.Fatalf("Failed to create current user: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/users/9999", nil)
	req.Header.Set("HX-Request", "true")
	req = addUserToContext(req, currentUser)

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestUsersDelete_InvalidID(t *testing.T) {
	handler, _ := setupUsersTestHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/users/invalid", nil)
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}
}

func TestGetRoleOptions(t *testing.T) {
	tests := []struct {
		name         string
		selectedRole string
		wantSelected string
	}{
		{"no selection", "", ""},
		{"admin selected", "admin", "admin"},
		{"editor selected", "editor", "editor"},
		{"viewer selected", "viewer", "viewer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := getRoleOptions(tt.selectedRole)

			if len(options) != 3 {
				t.Errorf("Expected 3 role options, got %d", len(options))
			}

			var selectedCount int
			for _, opt := range options {
				if opt.Selected {
					selectedCount++
					if opt.Value != tt.wantSelected {
						t.Errorf("Expected %q to be selected, got %q", tt.wantSelected, opt.Value)
					}
				}
			}

			expectedCount := 0
			if tt.wantSelected != "" {
				expectedCount = 1
			}
			if selectedCount != expectedCount {
				t.Errorf("Expected %d selected options, got %d", expectedCount, selectedCount)
			}
		})
	}
}

func TestToUserView(t *testing.T) {
	user := &auth.User{
		ID:       1,
		Username: "testuser",
		Email:    "test@test.com",
		Role:     auth.RoleEditor,
	}
	currentUser := &auth.User{
		ID:       2,
		Username: "currentuser",
		Role:     auth.RoleAdmin,
	}

	view := toUserView(user, currentUser)

	if view.ID != 1 {
		t.Errorf("Expected ID 1, got %d", view.ID)
	}
	if view.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got %q", view.Username)
	}
	if view.RoleDisplay != "Editor" {
		t.Errorf("Expected role display 'Editor', got %q", view.RoleDisplay)
	}
	if view.IsCurrentUser {
		t.Error("Should not be current user")
	}
	if !view.CanDelete {
		t.Error("Should be able to delete")
	}
}

func TestToUserView_CurrentUser(t *testing.T) {
	user := &auth.User{
		ID:       1,
		Username: "testuser",
		Email:    "test@test.com",
		Role:     auth.RoleAdmin,
	}

	view := toUserView(user, user)

	if !view.IsCurrentUser {
		t.Error("Should be current user")
	}
	if view.CanDelete {
		t.Error("Should not be able to delete self")
	}
}

// itoa converts an int64 to a string
func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
