package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// WHOISCache represents cached WHOIS data for a domain.
type WHOISCache struct {
	ID          int64
	DomainID    int64
	Registrar   string
	ExpiryDate  *time.Time
	CreatedDate *time.Time
	UpdatedDate *time.Time
	NameServers []string
	Status      []string
	RawData     string
	LookupTime  time.Time
}

// GetWHOISCache retrieves cached WHOIS data for a domain by domain ID.
func (s *Store) GetWHOISCache(domainID int64) (*WHOISCache, error) {
	query := `
		SELECT id, domain_id, registrar, expiry_date, created_date, updated_date,
		       name_servers, status, raw_data, lookup_time
		FROM whois_cache WHERE domain_id = ?
	`

	cache := &WHOISCache{}
	var expiryDate, createdDate, updatedDate sql.NullTime
	var nameServersStr, statusStr string

	err := s.db.QueryRow(query, domainID).Scan(
		&cache.ID, &cache.DomainID, &cache.Registrar, &expiryDate, &createdDate, &updatedDate,
		&nameServersStr, &statusStr, &cache.RawData, &cache.LookupTime,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting WHOIS cache: %w", err)
	}

	if expiryDate.Valid {
		cache.ExpiryDate = &expiryDate.Time
	}
	if createdDate.Valid {
		cache.CreatedDate = &createdDate.Time
	}
	if updatedDate.Valid {
		cache.UpdatedDate = &updatedDate.Time
	}

	// Parse comma-separated name servers and status
	if nameServersStr != "" {
		cache.NameServers = strings.Split(nameServersStr, ",")
	}
	if statusStr != "" {
		cache.Status = strings.Split(statusStr, ",")
	}

	return cache, nil
}

// SaveWHOISCache saves or updates WHOIS cache data for a domain.
func (s *Store) SaveWHOISCache(cache *WHOISCache) error {
	// Convert slices to comma-separated strings
	nameServersStr := strings.Join(cache.NameServers, ",")
	statusStr := strings.Join(cache.Status, ",")

	// Use INSERT OR REPLACE to handle both insert and update
	query := `
		INSERT OR REPLACE INTO whois_cache
			(domain_id, registrar, expiry_date, created_date, updated_date,
			 name_servers, status, raw_data, lookup_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := s.db.Exec(query,
		cache.DomainID, cache.Registrar, cache.ExpiryDate, cache.CreatedDate, cache.UpdatedDate,
		nameServersStr, statusStr, cache.RawData, cache.LookupTime,
	)
	if err != nil {
		return fmt.Errorf("saving WHOIS cache: %w", err)
	}

	if cache.ID == 0 {
		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("getting last insert id: %w", err)
		}
		cache.ID = id
	}

	return nil
}

// DeleteWHOISCache deletes cached WHOIS data for a domain.
func (s *Store) DeleteWHOISCache(domainID int64) error {
	_, err := s.db.Exec("DELETE FROM whois_cache WHERE domain_id = ?", domainID)
	if err != nil {
		return fmt.Errorf("deleting WHOIS cache: %w", err)
	}
	return nil
}

// IsWHOISCacheStale checks if the WHOIS cache for a domain is older than the given duration.
func (s *Store) IsWHOISCacheStale(domainID int64, maxAge time.Duration) (bool, error) {
	cache, err := s.GetWHOISCache(domainID)
	if err != nil {
		return true, err
	}
	if cache == nil {
		return true, nil // No cache, treat as stale
	}
	return time.Since(cache.LookupTime) > maxAge, nil
}
