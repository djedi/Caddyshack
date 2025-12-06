package store

import (
	"fmt"
)

// migration represents a database schema migration.
type migration struct {
	version int
	name    string
	sql     string
}

// migrations defines all database migrations in order.
// Each migration should be idempotent or additive.
var migrations = []migration{
	{
		version: 1,
		name:    "create_config_history",
		sql: `
			CREATE TABLE IF NOT EXISTS config_history (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				content TEXT NOT NULL,
				comment TEXT NOT NULL DEFAULT ''
			);
			CREATE INDEX IF NOT EXISTS idx_config_history_timestamp ON config_history(timestamp DESC);
		`,
	},
	{
		version: 2,
		name:    "create_notifications",
		sql: `
			CREATE TABLE IF NOT EXISTS notifications (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				type TEXT NOT NULL,
				severity TEXT NOT NULL,
				title TEXT NOT NULL,
				message TEXT NOT NULL,
				data TEXT NOT NULL DEFAULT '',
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				acknowledged_at DATETIME
			);
			CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications(created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_notifications_type ON notifications(type);
			CREATE INDEX IF NOT EXISTS idx_notifications_severity ON notifications(severity);
			CREATE INDEX IF NOT EXISTS idx_notifications_acknowledged ON notifications(acknowledged_at);
		`,
	},
	{
		version: 3,
		name:    "create_domains",
		sql: `
			CREATE TABLE IF NOT EXISTS domains (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL UNIQUE,
				registrar TEXT NOT NULL DEFAULT '',
				expiry_date DATETIME,
				notes TEXT NOT NULL DEFAULT '',
				auto_added BOOLEAN NOT NULL DEFAULT 0,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_domains_name ON domains(name);
			CREATE INDEX IF NOT EXISTS idx_domains_expiry_date ON domains(expiry_date);
			CREATE INDEX IF NOT EXISTS idx_domains_auto_added ON domains(auto_added);
		`,
	},
	{
		version: 4,
		name:    "create_users",
		sql: `
			CREATE TABLE IF NOT EXISTS users (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				username TEXT NOT NULL UNIQUE,
				email TEXT NOT NULL DEFAULT '',
				password_hash TEXT NOT NULL,
				role TEXT NOT NULL DEFAULT 'viewer',
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				last_login DATETIME
			);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users(username);
			CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
			CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
		`,
	},
	{
		version: 5,
		name:    "create_sessions",
		sql: `
			CREATE TABLE IF NOT EXISTS sessions (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				token TEXT NOT NULL UNIQUE,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				expires_at DATETIME NOT NULL,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
			CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
			CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
		`,
	},
	{
		version: 6,
		name:    "create_user_notification_preferences",
		sql: `
			CREATE TABLE IF NOT EXISTS user_notification_preferences (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL UNIQUE,
				notify_cert_expiry BOOLEAN NOT NULL DEFAULT 1,
				notify_domain_expiry BOOLEAN NOT NULL DEFAULT 1,
				notify_config_change BOOLEAN NOT NULL DEFAULT 1,
				notify_caddy_reload BOOLEAN NOT NULL DEFAULT 1,
				notify_container_down BOOLEAN NOT NULL DEFAULT 1,
				notify_system BOOLEAN NOT NULL DEFAULT 1,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_user_notification_preferences_user_id ON user_notification_preferences(user_id);
		`,
	},
	{
		version: 7,
		name:    "create_audit_log",
		sql: `
			CREATE TABLE IF NOT EXISTS audit_log (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER,
				username TEXT NOT NULL,
				action TEXT NOT NULL,
				resource_type TEXT NOT NULL,
				resource_id TEXT NOT NULL DEFAULT '',
				details TEXT NOT NULL DEFAULT '',
				ip_address TEXT NOT NULL DEFAULT '',
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
			);
			CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_audit_log_user_id ON audit_log(user_id);
			CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(action);
			CREATE INDEX IF NOT EXISTS idx_audit_log_resource_type ON audit_log(resource_type);
		`,
	},
}

// migrate runs all pending database migrations.
func (s *Store) migrate() error {
	// Create migrations table if it doesn't exist
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	// Get current version
	var currentVersion int
	err = s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("getting current version: %w", err)
	}

	// Run pending migrations
	for _, m := range migrations {
		if m.version <= currentVersion {
			continue
		}

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("starting transaction for migration %d: %w", m.version, err)
		}

		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("running migration %d (%s): %w", m.version, m.name, err)
		}

		if _, err := tx.Exec(
			"INSERT INTO schema_migrations (version, name) VALUES (?, ?)",
			m.version, m.name,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", m.version, err)
		}
	}

	return nil
}

// SchemaVersion returns the current schema version.
func (s *Store) SchemaVersion() (int, error) {
	var version int
	err := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("getting schema version: %w", err)
	}
	return version, nil
}
