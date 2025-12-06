package notifications

import (
	"database/sql"
	"fmt"
	"time"
)

// Severity represents the severity level of a notification.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
	SeverityError    Severity = "error"
)

// Type represents the category of notification.
type Type string

const (
	TypeCertExpiry    Type = "cert_expiry"
	TypeDomainExpiry  Type = "domain_expiry"
	TypeConfigChange  Type = "config_change"
	TypeCaddyReload   Type = "caddy_reload"
	TypeContainerDown Type = "container_down"
	TypeSystem        Type = "system"
)

// Notification represents a notification in the system.
type Notification struct {
	ID             int64
	Type           Type
	Severity       Severity
	Title          string
	Message        string
	Data           string    // JSON string for additional data
	CreatedAt      time.Time
	AcknowledgedAt *time.Time
}

// IsAcknowledged returns true if the notification has been acknowledged.
func (n *Notification) IsAcknowledged() bool {
	return n.AcknowledgedAt != nil
}

// Service provides methods for managing notifications.
type Service struct {
	db *sql.DB
}

// NewService creates a new notification service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

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

// parseNullableTimestamp parses an optional SQLite timestamp string.
func parseNullableTimestamp(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	t, err := parseTimestamp(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Create creates a new notification.
func (s *Service) Create(notificationType Type, severity Severity, title, message, data string) (*Notification, error) {
	result, err := s.db.Exec(
		"INSERT INTO notifications (type, severity, title, message, data) VALUES (?, ?, ?, ?, ?)",
		string(notificationType), string(severity), title, message, data,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting notification: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting last insert id: %w", err)
	}

	return s.GetByID(id)
}

// GetByID retrieves a notification by its ID.
func (s *Service) GetByID(id int64) (*Notification, error) {
	row := s.db.QueryRow(
		"SELECT id, type, severity, title, message, data, created_at, acknowledged_at FROM notifications WHERE id = ?",
		id,
	)
	return s.scanNotification(row)
}

// scanNotification scans a row into a Notification struct.
func (s *Service) scanNotification(row *sql.Row) (*Notification, error) {
	var n Notification
	var createdAtStr string
	var ackAtStr sql.NullString
	var notificationType, severity string

	if err := row.Scan(&n.ID, &notificationType, &severity, &n.Title, &n.Message, &n.Data, &createdAtStr, &ackAtStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("notification not found")
		}
		return nil, fmt.Errorf("scanning notification: %w", err)
	}

	n.Type = Type(notificationType)
	n.Severity = Severity(severity)

	createdAt, err := parseTimestamp(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}
	n.CreatedAt = createdAt

	ackAt, err := parseNullableTimestamp(ackAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing acknowledged_at: %w", err)
	}
	n.AcknowledgedAt = ackAt

	return &n, nil
}

// List retrieves notifications with optional filters.
func (s *Service) List(limit int, includeAcknowledged bool) ([]Notification, error) {
	query := "SELECT id, type, severity, title, message, data, created_at, acknowledged_at FROM notifications"
	if !includeAcknowledged {
		query += " WHERE acknowledged_at IS NULL"
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("querying notifications: %w", err)
	}
	defer rows.Close()

	return s.scanNotifications(rows)
}

// ListByType retrieves notifications of a specific type.
func (s *Service) ListByType(notificationType Type, limit int, includeAcknowledged bool) ([]Notification, error) {
	query := "SELECT id, type, severity, title, message, data, created_at, acknowledged_at FROM notifications WHERE type = ?"
	if !includeAcknowledged {
		query += " AND acknowledged_at IS NULL"
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, string(notificationType))
	if err != nil {
		return nil, fmt.Errorf("querying notifications by type: %w", err)
	}
	defer rows.Close()

	return s.scanNotifications(rows)
}

// ListBySeverity retrieves notifications of a specific severity.
func (s *Service) ListBySeverity(severity Severity, limit int, includeAcknowledged bool) ([]Notification, error) {
	query := "SELECT id, type, severity, title, message, data, created_at, acknowledged_at FROM notifications WHERE severity = ?"
	if !includeAcknowledged {
		query += " AND acknowledged_at IS NULL"
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, string(severity))
	if err != nil {
		return nil, fmt.Errorf("querying notifications by severity: %w", err)
	}
	defer rows.Close()

	return s.scanNotifications(rows)
}

// scanNotifications scans rows into a slice of Notification structs.
func (s *Service) scanNotifications(rows *sql.Rows) ([]Notification, error) {
	var notifications []Notification

	for rows.Next() {
		var n Notification
		var createdAtStr string
		var ackAtStr sql.NullString
		var notificationType, severity string

		if err := rows.Scan(&n.ID, &notificationType, &severity, &n.Title, &n.Message, &n.Data, &createdAtStr, &ackAtStr); err != nil {
			return nil, fmt.Errorf("scanning notification row: %w", err)
		}

		n.Type = Type(notificationType)
		n.Severity = Severity(severity)

		createdAt, err := parseTimestamp(createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing created_at: %w", err)
		}
		n.CreatedAt = createdAt

		ackAt, err := parseNullableTimestamp(ackAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing acknowledged_at: %w", err)
		}
		n.AcknowledgedAt = ackAt

		notifications = append(notifications, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating notification rows: %w", err)
	}

	return notifications, nil
}

// Acknowledge marks a notification as acknowledged.
func (s *Service) Acknowledge(id int64) error {
	result, err := s.db.Exec(
		"UPDATE notifications SET acknowledged_at = CURRENT_TIMESTAMP WHERE id = ? AND acknowledged_at IS NULL",
		id,
	)
	if err != nil {
		return fmt.Errorf("acknowledging notification: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("notification not found or already acknowledged")
	}

	return nil
}

// AcknowledgeAll marks all unacknowledged notifications as acknowledged.
func (s *Service) AcknowledgeAll() (int64, error) {
	result, err := s.db.Exec(
		"UPDATE notifications SET acknowledged_at = CURRENT_TIMESTAMP WHERE acknowledged_at IS NULL",
	)
	if err != nil {
		return 0, fmt.Errorf("acknowledging all notifications: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting rows affected: %w", err)
	}

	return rows, nil
}

// Delete deletes a notification by ID.
func (s *Service) Delete(id int64) error {
	result, err := s.db.Exec("DELETE FROM notifications WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting notification: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("notification not found")
	}

	return nil
}

// DeleteOlderThan deletes acknowledged notifications older than the given duration.
func (s *Service) DeleteOlderThan(d time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-d)
	result, err := s.db.Exec(
		"DELETE FROM notifications WHERE acknowledged_at IS NOT NULL AND created_at < ?",
		cutoff.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return 0, fmt.Errorf("deleting old notifications: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting rows affected: %w", err)
	}

	return rows, nil
}

// UnreadCount returns the count of unacknowledged notifications.
func (s *Service) UnreadCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM notifications WHERE acknowledged_at IS NULL").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting unread notifications: %w", err)
	}
	return count, nil
}

// UnreadCountBySeverity returns the count of unacknowledged notifications by severity.
func (s *Service) UnreadCountBySeverity(severity Severity) (int, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM notifications WHERE acknowledged_at IS NULL AND severity = ?",
		string(severity),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting unread notifications by severity: %w", err)
	}
	return count, nil
}

// ExistsUnacknowledged checks if there's an unacknowledged notification with the given type and data.
// This is useful to avoid creating duplicate notifications (e.g., for the same certificate expiry).
func (s *Service) ExistsUnacknowledged(notificationType Type, data string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM notifications WHERE type = ? AND data = ? AND acknowledged_at IS NULL",
		string(notificationType), data,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking for existing notification: %w", err)
	}
	return count > 0, nil
}
