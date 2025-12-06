package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Domain represents a tracked domain in the system.
type Domain struct {
	ID         int64
	Name       string
	Registrar  string
	ExpiryDate *time.Time
	Notes      string
	AutoAdded  bool // True if auto-detected from Caddyfile
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CreateDomain creates a new domain record.
func (s *Store) CreateDomain(d *Domain) error {
	query := `
		INSERT INTO domains (name, registrar, expiry_date, notes, auto_added, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`

	result, err := s.db.Exec(query, d.Name, d.Registrar, d.ExpiryDate, d.Notes, d.AutoAdded)
	if err != nil {
		return fmt.Errorf("creating domain: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting last insert id: %w", err)
	}
	d.ID = id

	return nil
}

// GetDomain retrieves a domain by ID.
func (s *Store) GetDomain(id int64) (*Domain, error) {
	query := `
		SELECT id, name, registrar, expiry_date, notes, auto_added, created_at, updated_at
		FROM domains WHERE id = ?
	`

	d := &Domain{}
	var expiryDate sql.NullTime
	err := s.db.QueryRow(query, id).Scan(
		&d.ID, &d.Name, &d.Registrar, &expiryDate, &d.Notes, &d.AutoAdded, &d.CreatedAt, &d.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting domain: %w", err)
	}

	if expiryDate.Valid {
		d.ExpiryDate = &expiryDate.Time
	}

	return d, nil
}

// GetDomainByName retrieves a domain by its name.
func (s *Store) GetDomainByName(name string) (*Domain, error) {
	query := `
		SELECT id, name, registrar, expiry_date, notes, auto_added, created_at, updated_at
		FROM domains WHERE name = ?
	`

	d := &Domain{}
	var expiryDate sql.NullTime
	err := s.db.QueryRow(query, name).Scan(
		&d.ID, &d.Name, &d.Registrar, &expiryDate, &d.Notes, &d.AutoAdded, &d.CreatedAt, &d.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting domain by name: %w", err)
	}

	if expiryDate.Valid {
		d.ExpiryDate = &expiryDate.Time
	}

	return d, nil
}

// ListDomains retrieves all domains ordered by name.
func (s *Store) ListDomains() ([]Domain, error) {
	query := `
		SELECT id, name, registrar, expiry_date, notes, auto_added, created_at, updated_at
		FROM domains ORDER BY name ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("listing domains: %w", err)
	}
	defer rows.Close()

	var domains []Domain
	for rows.Next() {
		var d Domain
		var expiryDate sql.NullTime
		err := rows.Scan(&d.ID, &d.Name, &d.Registrar, &expiryDate, &d.Notes, &d.AutoAdded, &d.CreatedAt, &d.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scanning domain row: %w", err)
		}
		if expiryDate.Valid {
			d.ExpiryDate = &expiryDate.Time
		}
		domains = append(domains, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating domain rows: %w", err)
	}

	return domains, nil
}

// UpdateDomain updates an existing domain record.
func (s *Store) UpdateDomain(d *Domain) error {
	query := `
		UPDATE domains
		SET name = ?, registrar = ?, expiry_date = ?, notes = ?, auto_added = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	result, err := s.db.Exec(query, d.Name, d.Registrar, d.ExpiryDate, d.Notes, d.AutoAdded, d.ID)
	if err != nil {
		return fmt.Errorf("updating domain: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("domain not found: %d", d.ID)
	}

	return nil
}

// DeleteDomain deletes a domain by ID.
func (s *Store) DeleteDomain(id int64) error {
	result, err := s.db.Exec("DELETE FROM domains WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting domain: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("domain not found: %d", id)
	}

	return nil
}

// DeleteDomainByName deletes a domain by name.
func (s *Store) DeleteDomainByName(name string) error {
	result, err := s.db.Exec("DELETE FROM domains WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("deleting domain by name: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("domain not found: %s", name)
	}

	return nil
}

// ListAutoAddedDomains retrieves all auto-added domains.
func (s *Store) ListAutoAddedDomains() ([]Domain, error) {
	query := `
		SELECT id, name, registrar, expiry_date, notes, auto_added, created_at, updated_at
		FROM domains WHERE auto_added = 1 ORDER BY name ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("listing auto-added domains: %w", err)
	}
	defer rows.Close()

	var domains []Domain
	for rows.Next() {
		var d Domain
		var expiryDate sql.NullTime
		err := rows.Scan(&d.ID, &d.Name, &d.Registrar, &expiryDate, &d.Notes, &d.AutoAdded, &d.CreatedAt, &d.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scanning domain row: %w", err)
		}
		if expiryDate.Valid {
			d.ExpiryDate = &expiryDate.Time
		}
		domains = append(domains, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating domain rows: %w", err)
	}

	return domains, nil
}

// SyncAutoAddedDomains syncs auto-added domains with the given list of domain names.
// It adds new domains and removes stale auto-added domains that are no longer in the list.
func (s *Store) SyncAutoAddedDomains(domainNames []string) error {
	// Get existing auto-added domains
	existing, err := s.ListAutoAddedDomains()
	if err != nil {
		return fmt.Errorf("listing existing auto-added domains: %w", err)
	}

	// Create a map of existing domain names
	existingMap := make(map[string]bool)
	for _, d := range existing {
		existingMap[d.Name] = true
	}

	// Create a map of new domain names
	newMap := make(map[string]bool)
	for _, name := range domainNames {
		newMap[name] = true
	}

	// Add new domains
	for _, name := range domainNames {
		if !existingMap[name] {
			// Check if domain exists but is not auto-added (manually added)
			d, err := s.GetDomainByName(name)
			if err != nil {
				return fmt.Errorf("checking existing domain: %w", err)
			}
			if d != nil {
				// Domain already exists (manually added), skip it
				continue
			}

			// Create new auto-added domain
			domain := &Domain{
				Name:      name,
				AutoAdded: true,
			}
			if err := s.CreateDomain(domain); err != nil {
				return fmt.Errorf("creating auto-added domain: %w", err)
			}
		}
	}

	// Remove stale auto-added domains
	for _, d := range existing {
		if !newMap[d.Name] {
			if err := s.DeleteDomain(d.ID); err != nil {
				return fmt.Errorf("deleting stale domain: %w", err)
			}
		}
	}

	return nil
}

// CountExpiringDomains returns the count of domains expiring within the given days.
func (s *Store) CountExpiringDomains(days int) (int, error) {
	query := `
		SELECT COUNT(*) FROM domains
		WHERE expiry_date IS NOT NULL
		AND expiry_date <= datetime('now', '+' || ? || ' days')
		AND expiry_date > datetime('now')
	`

	var count int
	err := s.db.QueryRow(query, days).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting expiring domains: %w", err)
	}

	return count, nil
}

// CountExpiredDomains returns the count of domains that have expired.
func (s *Store) CountExpiredDomains() (int, error) {
	query := `
		SELECT COUNT(*) FROM domains
		WHERE expiry_date IS NOT NULL
		AND expiry_date <= datetime('now')
	`

	var count int
	err := s.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting expired domains: %w", err)
	}

	return count, nil
}
