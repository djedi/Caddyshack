package config

import (
	"os"
	"strconv"
)

// DefaultHistoryLimit is the default number of config history entries to keep.
const DefaultHistoryLimit = 50

// Config holds all configuration for the Caddyshack application.
type Config struct {
	// Port is the HTTP server port.
	Port string

	// DevMode enables development features like serving static files from filesystem.
	DevMode bool

	// TemplatesDir is the path to the templates directory.
	TemplatesDir string

	// StaticDir is the path to the static files directory.
	StaticDir string

	// CaddyfilePath is the path to the Caddyfile to manage.
	CaddyfilePath string

	// CaddyAdminAPI is the URL to the Caddy Admin API.
	CaddyAdminAPI string

	// DBPath is the path to the SQLite database.
	DBPath string

	// AuthUser is the username for basic auth.
	AuthUser string

	// AuthPass is the password for basic auth.
	AuthPass string

	// HistoryLimit is the maximum number of config history entries to keep.
	HistoryLimit int

	// LogPath is the path to the Caddy log file.
	// If empty, will attempt to auto-detect from Caddyfile global options.
	LogPath string

	// DockerSocket is the path to the Docker socket.
	// If empty, Docker integration will be disabled.
	DockerSocket string

	// DockerEnabled indicates whether Docker integration is enabled.
	DockerEnabled bool
}

// Load reads configuration from environment variables, falling back to defaults.
func Load() *Config {
	return &Config{
		Port:          getEnv("CADDYSHACK_PORT", "8080"),
		DevMode:       getEnvBool("CADDYSHACK_DEV", false),
		TemplatesDir:  getEnv("CADDYSHACK_TEMPLATES_DIR", "templates"),
		StaticDir:     getEnv("CADDYSHACK_STATIC_DIR", "static"),
		CaddyfilePath: getEnv("CADDYSHACK_CADDYFILE", "/etc/caddy/Caddyfile"),
		CaddyAdminAPI: getEnv("CADDYSHACK_CADDY_API", "http://localhost:2019"),
		DBPath:        getEnv("CADDYSHACK_DB", "caddyshack.db"),
		AuthUser:      getEnv("CADDYSHACK_AUTH_USER", ""),
		AuthPass:      getEnv("CADDYSHACK_AUTH_PASS", ""),
		HistoryLimit:  getEnvInt("CADDYSHACK_HISTORY_LIMIT", DefaultHistoryLimit),
		LogPath:       getEnv("CADDYSHACK_LOG_PATH", ""),
		DockerSocket:  getEnv("CADDYSHACK_DOCKER_SOCKET", "/var/run/docker.sock"),
		DockerEnabled: getEnvBool("CADDYSHACK_DOCKER_ENABLED", false),
	}
}

// getEnv retrieves an environment variable or returns a default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBool retrieves an environment variable as a boolean.
// Returns defaultValue if the variable is not set or cannot be parsed.
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return b
}

// getEnvInt retrieves an environment variable as an integer.
// Returns defaultValue if the variable is not set or cannot be parsed.
func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	i, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return i
}

// AuthEnabled returns true if basic auth credentials are configured.
func (c *Config) AuthEnabled() bool {
	return c.AuthUser != "" && c.AuthPass != ""
}
