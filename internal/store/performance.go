package store

import (
	"database/sql"
	"fmt"
	"time"
)

// PerformanceMetric represents aggregated metrics for a time bucket.
type PerformanceMetric struct {
	ID             int64
	BucketTime     time.Time
	BucketDuration string // "1h", "1d", "5m"
	Domain         string // empty string means aggregate across all domains
	RequestCount   int64
	ErrorCount     int64
	TotalBytes     int64
	AvgLatencyMs   float64
	MinLatencyMs   float64
	MaxLatencyMs   float64
	P50LatencyMs   float64
	P95LatencyMs   float64
	P99LatencyMs   float64
	Status2xx      int64
	Status3xx      int64
	Status4xx      int64
	Status5xx      int64
	CreatedAt      time.Time
}

// SavePerformanceMetric saves or updates a performance metric.
func (s *Store) SavePerformanceMetric(m *PerformanceMetric) error {
	_, err := s.db.Exec(`
		INSERT INTO performance_metrics (
			bucket_time, bucket_duration, domain,
			request_count, error_count, total_bytes,
			avg_latency_ms, min_latency_ms, max_latency_ms,
			p50_latency_ms, p95_latency_ms, p99_latency_ms,
			status_2xx, status_3xx, status_4xx, status_5xx
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (bucket_time, bucket_duration, domain) DO UPDATE SET
			request_count = excluded.request_count,
			error_count = excluded.error_count,
			total_bytes = excluded.total_bytes,
			avg_latency_ms = excluded.avg_latency_ms,
			min_latency_ms = excluded.min_latency_ms,
			max_latency_ms = excluded.max_latency_ms,
			p50_latency_ms = excluded.p50_latency_ms,
			p95_latency_ms = excluded.p95_latency_ms,
			p99_latency_ms = excluded.p99_latency_ms,
			status_2xx = excluded.status_2xx,
			status_3xx = excluded.status_3xx,
			status_4xx = excluded.status_4xx,
			status_5xx = excluded.status_5xx
	`,
		m.BucketTime, m.BucketDuration, m.Domain,
		m.RequestCount, m.ErrorCount, m.TotalBytes,
		m.AvgLatencyMs, m.MinLatencyMs, m.MaxLatencyMs,
		m.P50LatencyMs, m.P95LatencyMs, m.P99LatencyMs,
		m.Status2xx, m.Status3xx, m.Status4xx, m.Status5xx,
	)
	if err != nil {
		return fmt.Errorf("saving performance metric: %w", err)
	}
	return nil
}

// GetPerformanceMetrics retrieves metrics for a time range.
func (s *Store) GetPerformanceMetrics(bucketDuration string, domain string, start, end time.Time) ([]PerformanceMetric, error) {
	query := `
		SELECT id, bucket_time, bucket_duration, domain,
			request_count, error_count, total_bytes,
			avg_latency_ms, min_latency_ms, max_latency_ms,
			p50_latency_ms, p95_latency_ms, p99_latency_ms,
			status_2xx, status_3xx, status_4xx, status_5xx,
			created_at
		FROM performance_metrics
		WHERE bucket_duration = ?
		AND bucket_time >= ?
		AND bucket_time <= ?
	`
	args := []interface{}{bucketDuration, start, end}

	if domain != "" {
		query += " AND domain = ?"
		args = append(args, domain)
	} else {
		query += " AND domain = ''"
	}

	query += " ORDER BY bucket_time ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying performance metrics: %w", err)
	}
	defer rows.Close()

	var metrics []PerformanceMetric
	for rows.Next() {
		var m PerformanceMetric
		err := rows.Scan(
			&m.ID, &m.BucketTime, &m.BucketDuration, &m.Domain,
			&m.RequestCount, &m.ErrorCount, &m.TotalBytes,
			&m.AvgLatencyMs, &m.MinLatencyMs, &m.MaxLatencyMs,
			&m.P50LatencyMs, &m.P95LatencyMs, &m.P99LatencyMs,
			&m.Status2xx, &m.Status3xx, &m.Status4xx, &m.Status5xx,
			&m.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning performance metric: %w", err)
		}
		metrics = append(metrics, m)
	}

	return metrics, rows.Err()
}

// GetPerformanceMetricsByDomain retrieves metrics grouped by domain for a time range.
func (s *Store) GetPerformanceMetricsByDomain(bucketDuration string, start, end time.Time) ([]PerformanceMetric, error) {
	query := `
		SELECT id, bucket_time, bucket_duration, domain,
			request_count, error_count, total_bytes,
			avg_latency_ms, min_latency_ms, max_latency_ms,
			p50_latency_ms, p95_latency_ms, p99_latency_ms,
			status_2xx, status_3xx, status_4xx, status_5xx,
			created_at
		FROM performance_metrics
		WHERE bucket_duration = ?
		AND bucket_time >= ?
		AND bucket_time <= ?
		AND domain != ''
		ORDER BY bucket_time ASC, domain ASC
	`

	rows, err := s.db.Query(query, bucketDuration, start, end)
	if err != nil {
		return nil, fmt.Errorf("querying performance metrics by domain: %w", err)
	}
	defer rows.Close()

	var metrics []PerformanceMetric
	for rows.Next() {
		var m PerformanceMetric
		err := rows.Scan(
			&m.ID, &m.BucketTime, &m.BucketDuration, &m.Domain,
			&m.RequestCount, &m.ErrorCount, &m.TotalBytes,
			&m.AvgLatencyMs, &m.MinLatencyMs, &m.MaxLatencyMs,
			&m.P50LatencyMs, &m.P95LatencyMs, &m.P99LatencyMs,
			&m.Status2xx, &m.Status3xx, &m.Status4xx, &m.Status5xx,
			&m.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning performance metric: %w", err)
		}
		metrics = append(metrics, m)
	}

	return metrics, rows.Err()
}

// GetDomainBandwidthSummary retrieves bandwidth totals per domain for a time range.
func (s *Store) GetDomainBandwidthSummary(bucketDuration string, start, end time.Time) ([]DomainBandwidth, error) {
	query := `
		SELECT domain,
			SUM(request_count) as total_requests,
			SUM(total_bytes) as total_bytes,
			SUM(error_count) as total_errors
		FROM performance_metrics
		WHERE bucket_duration = ?
		AND bucket_time >= ?
		AND bucket_time <= ?
		AND domain != ''
		GROUP BY domain
		ORDER BY total_bytes DESC
	`

	rows, err := s.db.Query(query, bucketDuration, start, end)
	if err != nil {
		return nil, fmt.Errorf("querying domain bandwidth summary: %w", err)
	}
	defer rows.Close()

	var results []DomainBandwidth
	for rows.Next() {
		var d DomainBandwidth
		err := rows.Scan(&d.Domain, &d.TotalRequests, &d.TotalBytes, &d.TotalErrors)
		if err != nil {
			return nil, fmt.Errorf("scanning domain bandwidth: %w", err)
		}
		results = append(results, d)
	}

	return results, rows.Err()
}

// DomainBandwidth represents bandwidth usage for a domain.
type DomainBandwidth struct {
	Domain        string
	TotalRequests int64
	TotalBytes    int64
	TotalErrors   int64
}

// PrunePerformanceMetrics removes old metrics.
func (s *Store) PrunePerformanceMetrics(olderThan time.Time) (int64, error) {
	result, err := s.db.Exec(
		"DELETE FROM performance_metrics WHERE bucket_time < ?",
		olderThan,
	)
	if err != nil {
		return 0, fmt.Errorf("pruning performance metrics: %w", err)
	}
	return result.RowsAffected()
}

// GetLatestMetricTime returns the most recent metric bucket time.
func (s *Store) GetLatestMetricTime(bucketDuration string) (time.Time, error) {
	var t sql.NullString
	err := s.db.QueryRow(
		"SELECT MAX(bucket_time) FROM performance_metrics WHERE bucket_duration = ?",
		bucketDuration,
	).Scan(&t)
	if err != nil {
		return time.Time{}, fmt.Errorf("getting latest metric time: %w", err)
	}
	if !t.Valid || t.String == "" {
		return time.Time{}, nil
	}
	// Parse the datetime string using common formats
	parsed, err := parseTimestampExtended(t.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing latest metric time: %w", err)
	}
	return parsed, nil
}

// parseTimestampExtended parses SQLite timestamp strings in various formats.
// This handles the formats that SQLite and Go's time.Time can produce.
func parseTimestampExtended(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05 -0700 MST",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", s)
}
