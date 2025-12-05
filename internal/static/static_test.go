package static

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHandler_WithEmbeddedFS(t *testing.T) {
	// Create a mock filesystem
	mockFS := fstest.MapFS{
		"css/test.css": &fstest.MapFile{Data: []byte("body { color: red; }")},
	}

	handler := Handler(mockFS, "")

	req := httptest.NewRequest("GET", "/static/css/test.css", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "body { color: red; }" {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestHandler_WithFilesystem(t *testing.T) {
	// When embeddedFS is nil, it should use os.DirFS
	// We can't easily test this without actual files, so we just verify it doesn't panic
	handler := Handler(nil, ".")

	if handler == nil {
		t.Error("handler should not be nil")
	}
}

func TestHandler_FileNotFound(t *testing.T) {
	mockFS := fstest.MapFS{}

	handler := Handler(mockFS, "")

	req := httptest.NewRequest("GET", "/static/nonexistent.css", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandler_SubDirectory(t *testing.T) {
	mockFS := fstest.MapFS{
		"css/output.css": &fstest.MapFile{Data: []byte("/* tailwind */")},
		"js/app.js":      &fstest.MapFile{Data: []byte("console.log('hello');")},
	}

	handler := Handler(mockFS, "")

	tests := []struct {
		path     string
		expected string
	}{
		{"/static/css/output.css", "/* tailwind */"},
		{"/static/js/app.js", "console.log('hello');"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
			}

			if w.Body.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, w.Body.String())
			}
		})
	}
}

// Verify that Handler works with fs.FS interface
var _ fs.FS = fstest.MapFS{}
