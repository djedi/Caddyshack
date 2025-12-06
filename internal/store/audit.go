package store

import (
	"database/sql"
	"fmt"
	"time"
)

// AuditAction represents the type of action performed.
type AuditAction string

const (
	// Site actions
	ActionSiteCreate  AuditAction = "site.create"
	ActionSiteUpdate  AuditAction = "site.update"
	ActionSiteDelete  AuditAction = "site.delete"

	// Snippet actions
	ActionSnippetCreate AuditAction = "snippet.create"
	ActionSnippetUpdate AuditAction = "snippet.update"
	ActionSnippetDelete AuditAction = "snippet.delete"

	// User actions
	ActionUserCreate AuditAction = "user.create"
	ActionUserUpdate AuditAction = "user.update"
	ActionUserDelete AuditAction = "user.delete"
	ActionUserLogin  AuditAction = "user.login"
	ActionUserLogout AuditAction = "user.logout"

	// Domain actions
	ActionDomainCreate AuditAction = "domain.create"
	ActionDomainUpdate AuditAction = "domain.update"
	ActionDomainDelete AuditAction = "domain.delete"

	// Config actions
	ActionConfigImport  AuditAction = "config.import"
	ActionConfigExport  AuditAction = "config.export"
	ActionConfigRestore AuditAction = "config.restore"
	ActionConfigReload  AuditAction = "config.reload"

	// Global options actions
	ActionGlobalUpdate AuditAction = "global.update"
)

// AuditResourceType represents the type of resource affected.
type AuditResourceType string

const (
	ResourceSite    AuditResourceType = "site"
	ResourceSnippet AuditResourceType = "snippet"
	ResourceUser    AuditResourceType = "user"
	ResourceDomain  AuditResourceType = "domain"
	ResourceConfig  AuditResourceType = "config"
	ResourceGlobal  AuditResourceType = "global"
)

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	ID           int64
	UserID       *int64
	Username     string
	Action       AuditAction
	ResourceType AuditResourceType
	ResourceID   string
	Details      string
	IPAddress    string
	CreatedAt    time.Time
}

// AuditListOptions contains options for listing audit entries.
type AuditListOptions struct {
	UserID       *int64
	Action       string
	ResourceType string
	StartDate    *time.Time
	EndDate      *time.Time
	Limit        int
	Offset       int
}

// CreateAuditEntry creates a new audit log entry.
func (s *Store) CreateAuditEntry(entry *AuditEntry) error {
	result, err := s.db.Exec(`
		INSERT INTO audit_log (user_id, username, action, resource_type, resource_id, details, ip_address)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, entry.UserID, entry.Username, string(entry.Action), string(entry.ResourceType),
		entry.ResourceID, entry.Details, entry.IPAddress)
	if err != nil {
		return fmt.Errorf("creating audit entry: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting audit entry ID: %w", err)
	}

	entry.ID = id
	entry.CreatedAt = time.Now()
	return nil
}

// ListAuditEntries retrieves audit entries with optional filtering.
func (s *Store) ListAuditEntries(opts AuditListOptions) ([]*AuditEntry, error) {
	query := `
		SELECT id, user_id, username, action, resource_type, resource_id, details, ip_address, created_at
		FROM audit_log
		WHERE 1=1
	`
	var args []interface{}

	if opts.UserID != nil {
		query += " AND user_id = ?"
		args = append(args, *opts.UserID)
	}

	if opts.Action != "" {
		query += " AND action = ?"
		args = append(args, opts.Action)
	}

	if opts.ResourceType != "" {
		query += " AND resource_type = ?"
		args = append(args, opts.ResourceType)
	}

	if opts.StartDate != nil {
		query += " AND created_at >= ?"
		args = append(args, *opts.StartDate)
	}

	if opts.EndDate != nil {
		query += " AND created_at <= ?"
		args = append(args, *opts.EndDate)
	}

	query += " ORDER BY created_at DESC"

	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	} else {
		query += " LIMIT 100" // Default limit
	}

	if opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing audit entries: %w", err)
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		entry := &AuditEntry{}
		var userID sql.NullInt64
		var action, resourceType string

		if err := rows.Scan(
			&entry.ID, &userID, &entry.Username, &action, &resourceType,
			&entry.ResourceID, &entry.Details, &entry.IPAddress, &entry.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning audit entry: %w", err)
		}

		if userID.Valid {
			entry.UserID = &userID.Int64
		}
		entry.Action = AuditAction(action)
		entry.ResourceType = AuditResourceType(resourceType)
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating audit entries: %w", err)
	}

	return entries, nil
}

// CountAuditEntries returns the total count of audit entries with optional filtering.
func (s *Store) CountAuditEntries(opts AuditListOptions) (int, error) {
	query := `SELECT COUNT(*) FROM audit_log WHERE 1=1`
	var args []interface{}

	if opts.UserID != nil {
		query += " AND user_id = ?"
		args = append(args, *opts.UserID)
	}

	if opts.Action != "" {
		query += " AND action = ?"
		args = append(args, opts.Action)
	}

	if opts.ResourceType != "" {
		query += " AND resource_type = ?"
		args = append(args, opts.ResourceType)
	}

	if opts.StartDate != nil {
		query += " AND created_at >= ?"
		args = append(args, *opts.StartDate)
	}

	if opts.EndDate != nil {
		query += " AND created_at <= ?"
		args = append(args, *opts.EndDate)
	}

	var count int
	if err := s.db.QueryRow(query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting audit entries: %w", err)
	}

	return count, nil
}

// GetAuditEntry retrieves a single audit entry by ID.
func (s *Store) GetAuditEntry(id int64) (*AuditEntry, error) {
	entry := &AuditEntry{}
	var userID sql.NullInt64
	var action, resourceType string

	err := s.db.QueryRow(`
		SELECT id, user_id, username, action, resource_type, resource_id, details, ip_address, created_at
		FROM audit_log WHERE id = ?
	`, id).Scan(
		&entry.ID, &userID, &entry.Username, &action, &resourceType,
		&entry.ResourceID, &entry.Details, &entry.IPAddress, &entry.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting audit entry: %w", err)
	}

	if userID.Valid {
		entry.UserID = &userID.Int64
	}
	entry.Action = AuditAction(action)
	entry.ResourceType = AuditResourceType(resourceType)

	return entry, nil
}

// GetDistinctActions returns all distinct actions from the audit log.
func (s *Store) GetDistinctActions() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT action FROM audit_log ORDER BY action`)
	if err != nil {
		return nil, fmt.Errorf("getting distinct actions: %w", err)
	}
	defer rows.Close()

	var actions []string
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			return nil, fmt.Errorf("scanning action: %w", err)
		}
		actions = append(actions, action)
	}

	return actions, nil
}

// GetDistinctUsers returns all distinct usernames from the audit log.
func (s *Store) GetDistinctUsers() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT username FROM audit_log WHERE username != '' ORDER BY username`)
	if err != nil {
		return nil, fmt.Errorf("getting distinct users: %w", err)
	}
	defer rows.Close()

	var users []string
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return nil, fmt.Errorf("scanning username: %w", err)
		}
		users = append(users, username)
	}

	return users, nil
}

// PruneAuditLog removes audit entries older than the specified duration.
func (s *Store) PruneAuditLog(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := s.db.Exec(`DELETE FROM audit_log WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("pruning audit log: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting deleted count: %w", err)
	}

	return count, nil
}
