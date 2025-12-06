package config

import (
	"os"
	"strconv"
	"strings"
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

	// Email notification settings
	EmailEnabled       bool
	SMTPHost           string
	SMTPPort           int
	SMTPUser           string
	SMTPPassword       string
	EmailFrom          string
	EmailFromName      string
	EmailTo            []string
	EmailUseTLS        bool
	EmailUseSTARTTLS   bool
	EmailInsecureSkipVerify bool
	EmailSendOnWarning bool

	// Webhook notification settings
	WebhookEnabled     bool
	WebhookURLs        []string
	WebhookHeaders     map[string]string
	WebhookMinSeverity string
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
		// Email notification settings
		EmailEnabled:            getEnvBool("CADDYSHACK_EMAIL_ENABLED", false),
		SMTPHost:                getEnv("CADDYSHACK_SMTP_HOST", ""),
		SMTPPort:                getEnvInt("CADDYSHACK_SMTP_PORT", 587),
		SMTPUser:                getEnv("CADDYSHACK_SMTP_USER", ""),
		SMTPPassword:            getEnv("CADDYSHACK_SMTP_PASSWORD", ""),
		EmailFrom:               getEnv("CADDYSHACK_EMAIL_FROM", ""),
		EmailFromName:           getEnv("CADDYSHACK_EMAIL_FROM_NAME", "Caddyshack"),
		EmailTo:                 getEnvList("CADDYSHACK_EMAIL_TO", nil),
		EmailUseTLS:             getEnvBool("CADDYSHACK_EMAIL_USE_TLS", false),
		EmailUseSTARTTLS:        getEnvBool("CADDYSHACK_EMAIL_USE_STARTTLS", true),
		EmailInsecureSkipVerify: getEnvBool("CADDYSHACK_EMAIL_INSECURE_SKIP_VERIFY", false),
		EmailSendOnWarning:      getEnvBool("CADDYSHACK_EMAIL_SEND_ON_WARNING", false),
		// Webhook notification settings
		WebhookEnabled:     getEnvBool("CADDYSHACK_WEBHOOK_ENABLED", false),
		WebhookURLs:        getEnvList("CADDYSHACK_WEBHOOK_URLS", nil),
		WebhookHeaders:     getEnvMap("CADDYSHACK_WEBHOOK_HEADERS", nil),
		WebhookMinSeverity: getEnv("CADDYSHACK_WEBHOOK_MIN_SEVERITY", "info"),
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

// getEnvList retrieves an environment variable as a comma-separated list.
// Returns defaultValue if the variable is not set.
func getEnvList(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return defaultValue
	}
	return result
}

// getEnvMap retrieves an environment variable as a key=value map.
// Format: "key1=value1,key2=value2"
// Returns defaultValue if the variable is not set.
func getEnvMap(key string, defaultValue map[string]string) map[string]string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	result := make(map[string]string)
	pairs := strings.Split(value, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			k := strings.TrimSpace(kv[0])
			v := strings.TrimSpace(kv[1])
			if k != "" {
				result[k] = v
			}
		}
	}
	if len(result) == 0 {
		return defaultValue
	}
	return result
}

// AuthEnabled returns true if basic auth credentials are configured.
func (c *Config) AuthEnabled() bool {
	return c.AuthUser != "" && c.AuthPass != ""
}

// EmailConfigured returns true if email notification settings are properly configured.
func (c *Config) EmailConfigured() bool {
	return c.EmailEnabled &&
		c.SMTPHost != "" &&
		c.EmailFrom != "" &&
		len(c.EmailTo) > 0
}

// WebhookConfigured returns true if webhook notification settings are properly configured.
func (c *Config) WebhookConfigured() bool {
	return c.WebhookEnabled && len(c.WebhookURLs) > 0
}
