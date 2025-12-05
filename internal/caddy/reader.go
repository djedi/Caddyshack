package caddy

import (
	"errors"
	"io/fs"
	"os"
)

// ErrCaddyfileNotFound is returned when the Caddyfile does not exist.
var ErrCaddyfileNotFound = errors.New("caddyfile not found")

// Reader handles reading Caddyfile content from the filesystem.
type Reader struct {
	path string
}

// NewReader creates a new Reader for the given Caddyfile path.
func NewReader(path string) *Reader {
	return &Reader{path: path}
}

// Read reads the entire Caddyfile and returns its content as a string.
// Returns ErrCaddyfileNotFound if the file does not exist.
// Returns other errors for permission issues or other read failures.
func (r *Reader) Read() (string, error) {
	content, err := os.ReadFile(r.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ErrCaddyfileNotFound
		}
		return "", err
	}
	return string(content), nil
}

// Exists checks if the Caddyfile exists at the configured path.
func (r *Reader) Exists() bool {
	_, err := os.Stat(r.path)
	return err == nil
}

// Path returns the configured Caddyfile path.
func (r *Reader) Path() string {
	return r.path
}
