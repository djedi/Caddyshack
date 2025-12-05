package store

import (
	"fmt"
	"time"
)

// parseTimestamp parses SQLite timestamp strings in various formats.
func parseTimestamp(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", s)
}

// SaveConfig saves a new configuration version to history.
func (s *Store) SaveConfig(content, comment string) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO config_history (content, comment) VALUES (?, ?)",
		content, comment,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting config history: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting last insert id: %w", err)
	}

	return id, nil
}

// GetConfig retrieves a specific configuration version by ID.
func (s *Store) GetConfig(id int64) (*ConfigHistory, error) {
	row := s.db.QueryRow(
		"SELECT id, timestamp, content, comment FROM config_history WHERE id = ?",
		id,
	)

	var ch ConfigHistory
	var timestamp string
	if err := row.Scan(&ch.ID, &timestamp, &ch.Content, &ch.Comment); err != nil {
		return nil, fmt.Errorf("scanning config history: %w", err)
	}

	t, err := parseTimestamp(timestamp)
	if err != nil {
		return nil, fmt.Errorf("parsing timestamp: %w", err)
	}
	ch.Timestamp = t

	return &ch, nil
}

// ListConfigs retrieves configuration history with optional limit.
// Results are ordered by ID descending (newest first).
func (s *Store) ListConfigs(limit int) ([]ConfigHistory, error) {
	query := "SELECT id, timestamp, content, comment FROM config_history ORDER BY id DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("querying config history: %w", err)
	}
	defer rows.Close()

	var configs []ConfigHistory
	for rows.Next() {
		var ch ConfigHistory
		var timestamp string
		if err := rows.Scan(&ch.ID, &timestamp, &ch.Content, &ch.Comment); err != nil {
			return nil, fmt.Errorf("scanning config history row: %w", err)
		}

		t, err := parseTimestamp(timestamp)
		if err != nil {
			return nil, fmt.Errorf("parsing timestamp: %w", err)
		}
		ch.Timestamp = t

		configs = append(configs, ch)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating config history rows: %w", err)
	}

	return configs, nil
}

// LatestConfig retrieves the most recent configuration version.
// Returns nil if no configurations exist.
func (s *Store) LatestConfig() (*ConfigHistory, error) {
	configs, err := s.ListConfigs(1)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, nil
	}
	return &configs[0], nil
}

// PruneHistory deletes old configuration entries, keeping only the most recent n entries.
func (s *Store) PruneHistory(keepCount int) (int64, error) {
	result, err := s.db.Exec(`
		DELETE FROM config_history
		WHERE id NOT IN (
			SELECT id FROM config_history
			ORDER BY id DESC
			LIMIT ?
		)
	`, keepCount)
	if err != nil {
		return 0, fmt.Errorf("pruning config history: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting rows affected: %w", err)
	}

	return deleted, nil
}

// ConfigCount returns the total number of configuration entries.
func (s *Store) ConfigCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM config_history").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting config history: %w", err)
	}
	return count, nil
}
