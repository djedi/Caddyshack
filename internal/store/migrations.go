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
