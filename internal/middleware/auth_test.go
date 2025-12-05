package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBasicAuth(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	tests := []struct {
		name           string
		username       string
		password       string
		reqUser        string
		reqPass        string
		hasAuth        bool
		expectedStatus int
	}{
		{
			name:           "no auth configured - allows request",
			username:       "",
			password:       "",
			hasAuth:        false,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "auth configured - correct credentials",
			username:       "admin",
			password:       "secret",
			reqUser:        "admin",
			reqPass:        "secret",
			hasAuth:        true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "auth configured - wrong username",
			username:       "admin",
			password:       "secret",
			reqUser:        "wrong",
			reqPass:        "secret",
			hasAuth:        true,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "auth configured - wrong password",
			username:       "admin",
			password:       "secret",
			reqUser:        "admin",
			reqPass:        "wrong",
			hasAuth:        true,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "auth configured - no credentials provided",
			username:       "admin",
			password:       "secret",
			hasAuth:        false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "only username configured - allows request",
			username:       "admin",
			password:       "",
			hasAuth:        false,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "only password configured - allows request",
			username:       "",
			password:       "secret",
			hasAuth:        false,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := BasicAuth(tt.username, tt.password)
			wrappedHandler := middleware(handler)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.hasAuth {
				req.SetBasicAuth(tt.reqUser, tt.reqPass)
			}

			rec := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			if tt.expectedStatus == http.StatusUnauthorized {
				wwwAuth := rec.Header().Get("WWW-Authenticate")
				if wwwAuth == "" {
					t.Error("expected WWW-Authenticate header on 401 response")
				}
			}
		})
	}
}
