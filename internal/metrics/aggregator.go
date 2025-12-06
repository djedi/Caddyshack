package metrics

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/store"
)

// Aggregator collects and aggregates performance metrics from Caddy logs.
type Aggregator struct {
	store        *store.Store
	config       *config.Config
	mu           sync.Mutex
	lastPosition int64
	stopCh       chan struct{}
	running      bool
}

// NewAggregator creates a new metrics aggregator.
func NewAggregator(s *store.Store, cfg *config.Config) *Aggregator {
	return &Aggregator{
		store:  s,
		config: cfg,
		stopCh: make(chan struct{}),
	}
}

// Start begins periodic log aggregation.
func (a *Aggregator) Start() {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return
	}
	a.running = true
	a.mu.Unlock()

	go a.runAggregationLoop()
}

// Stop stops the aggregation loop.
func (a *Aggregator) Stop() {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return
	}
	a.running = false
	a.mu.Unlock()

	close(a.stopCh)
}

// runAggregationLoop runs the periodic aggregation.
func (a *Aggregator) runAggregationLoop() {
	// Run immediately on start
	a.aggregate()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.aggregate()
		case <-a.stopCh:
			return
		}
	}
}

// aggregate reads new log entries and creates aggregated metrics.
func (a *Aggregator) aggregate() {
	logPath := a.getLogPath()
	if logPath == "" {
		return
	}

	entries, err := a.readNewLogEntries(logPath)
	if err != nil {
		log.Printf("Error reading log entries for aggregation: %v", err)
		return
	}

	if len(entries) == 0 {
		return
	}

	// Group entries by time bucket and domain
	buckets := a.groupByBucket(entries, 5*time.Minute)

	// Save aggregated metrics
	for _, bucket := range buckets {
		if err := a.store.SavePerformanceMetric(bucket); err != nil {
			log.Printf("Error saving performance metric: %v", err)
		}
	}

	// Prune old metrics (keep 30 days)
	pruneTime := time.Now().Add(-30 * 24 * time.Hour)
	if _, err := a.store.PrunePerformanceMetrics(pruneTime); err != nil {
		log.Printf("Error pruning old metrics: %v", err)
	}
}

// logEntry represents a parsed Caddy log entry for aggregation.
type logEntry struct {
	Timestamp time.Time
	Domain    string
	Status    int
	Duration  float64 // in seconds
	Size      int64
}

// caddyLogEntry represents the JSON structure of a Caddy log entry.
type caddyLogEntry struct {
	TS      float64 `json:"ts"`
	Request *struct {
		Host string `json:"host"`
	} `json:"request"`
	Status   int     `json:"status"`
	Duration float64 `json:"duration"`
	Size     int64   `json:"size"`
}

// readNewLogEntries reads log entries from the file starting from the last position.
func (a *Aggregator) readNewLogEntries(logPath string) ([]logEntry, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	// If file was truncated (log rotation), start from beginning
	if stat.Size() < a.lastPosition {
		a.lastPosition = 0
	}

	// Seek to last position
	if a.lastPosition > 0 {
		_, err = file.Seek(a.lastPosition, io.SeekStart)
		if err != nil {
			return nil, err
		}
	}

	var entries []logEntry
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "{") {
			continue
		}

		var logData caddyLogEntry
		if err := json.Unmarshal([]byte(line), &logData); err != nil {
			continue
		}

		if logData.TS == 0 {
			continue
		}

		entry := logEntry{
			Timestamp: time.Unix(int64(logData.TS), int64((logData.TS-float64(int64(logData.TS)))*1e9)),
			Status:    logData.Status,
			Duration:  logData.Duration,
			Size:      logData.Size,
		}

		if logData.Request != nil {
			entry.Domain = logData.Request.Host
		}

		entries = append(entries, entry)
	}

	// Update position
	newPosition, _ := file.Seek(0, io.SeekCurrent)
	a.lastPosition = newPosition

	return entries, scanner.Err()
}

// groupByBucket groups log entries into time buckets and calculates aggregated metrics.
func (a *Aggregator) groupByBucket(entries []logEntry, bucketDuration time.Duration) []*store.PerformanceMetric {
	// Group by bucket time and domain
	type bucketKey struct {
		bucketTime time.Time
		domain     string
	}

	bucketData := make(map[bucketKey]*metricAccumulator)

	for _, entry := range entries {
		// Round down to bucket time
		bucketTime := entry.Timestamp.Truncate(bucketDuration)

		// Create bucket for specific domain
		key := bucketKey{bucketTime: bucketTime, domain: entry.Domain}
		if bucketData[key] == nil {
			bucketData[key] = &metricAccumulator{}
		}
		bucketData[key].add(entry)

		// Also create aggregate bucket (empty domain)
		aggKey := bucketKey{bucketTime: bucketTime, domain: ""}
		if bucketData[aggKey] == nil {
			bucketData[aggKey] = &metricAccumulator{}
		}
		bucketData[aggKey].add(entry)
	}

	// Convert to performance metrics
	var result []*store.PerformanceMetric
	for key, acc := range bucketData {
		m := acc.toMetric(key.bucketTime, bucketDurationString(bucketDuration), key.domain)
		result = append(result, m)
	}

	return result
}

// metricAccumulator accumulates metrics for a bucket.
type metricAccumulator struct {
	requestCount int64
	errorCount   int64
	totalBytes   int64
	latencies    []float64
	status2xx    int64
	status3xx    int64
	status4xx    int64
	status5xx    int64
}

func (m *metricAccumulator) add(entry logEntry) {
	m.requestCount++
	m.totalBytes += entry.Size

	// Track latency in milliseconds
	if entry.Duration > 0 {
		m.latencies = append(m.latencies, entry.Duration*1000)
	}

	// Categorize status codes
	switch {
	case entry.Status >= 200 && entry.Status < 300:
		m.status2xx++
	case entry.Status >= 300 && entry.Status < 400:
		m.status3xx++
	case entry.Status >= 400 && entry.Status < 500:
		m.status4xx++
	case entry.Status >= 500:
		m.status5xx++
		m.errorCount++
	}
}

func (m *metricAccumulator) toMetric(bucketTime time.Time, bucketDuration, domain string) *store.PerformanceMetric {
	metric := &store.PerformanceMetric{
		BucketTime:     bucketTime,
		BucketDuration: bucketDuration,
		Domain:         domain,
		RequestCount:   m.requestCount,
		ErrorCount:     m.errorCount,
		TotalBytes:     m.totalBytes,
		Status2xx:      m.status2xx,
		Status3xx:      m.status3xx,
		Status4xx:      m.status4xx,
		Status5xx:      m.status5xx,
	}

	if len(m.latencies) > 0 {
		sort.Float64s(m.latencies)

		// Calculate average
		var sum float64
		for _, l := range m.latencies {
			sum += l
		}
		metric.AvgLatencyMs = sum / float64(len(m.latencies))

		// Min/Max
		metric.MinLatencyMs = m.latencies[0]
		metric.MaxLatencyMs = m.latencies[len(m.latencies)-1]

		// Percentiles
		metric.P50LatencyMs = percentile(m.latencies, 50)
		metric.P95LatencyMs = percentile(m.latencies, 95)
		metric.P99LatencyMs = percentile(m.latencies, 99)
	}

	return metric
}

// percentile calculates the p-th percentile of a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}

	index := (p / 100) * float64(len(sorted)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))
	weight := index - float64(lower)

	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// bucketDurationString converts a duration to a string identifier.
func bucketDurationString(d time.Duration) string {
	switch d {
	case 5 * time.Minute:
		return "5m"
	case time.Hour:
		return "1h"
	case 24 * time.Hour:
		return "1d"
	default:
		return "5m"
	}
}

// getLogPath determines the log file path from config or Caddyfile.
func (a *Aggregator) getLogPath() string {
	if a.config.LogPath != "" {
		return a.config.LogPath
	}

	// Try to auto-detect from Caddyfile global options
	reader := caddy.NewReader(a.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		return ""
	}

	parser := caddy.NewParser(content)
	globalOpts, err := parser.ParseGlobalOptions()
	if err != nil || globalOpts == nil || globalOpts.LogConfig == nil {
		return ""
	}

	output := globalOpts.LogConfig.Output
	if output == "" {
		return ""
	}

	if strings.HasPrefix(output, "file ") {
		parts := strings.SplitN(output, " ", 2)
		if len(parts) >= 2 {
			return strings.TrimSpace(parts[1])
		}
	}

	if output == "stdout" || output == "stderr" {
		return ""
	}

	return output
}

// AggregateHistorical processes historical log data for a specific time range.
// This is useful for populating initial metrics from existing logs.
func (a *Aggregator) AggregateHistorical(logPath string, startTime, endTime time.Time) error {
	file, err := os.Open(logPath)
	if err != nil {
		return err
	}
	defer file.Close()

	var entries []logEntry
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "{") {
			continue
		}

		var logData caddyLogEntry
		if err := json.Unmarshal([]byte(line), &logData); err != nil {
			continue
		}

		if logData.TS == 0 {
			continue
		}

		timestamp := time.Unix(int64(logData.TS), int64((logData.TS-float64(int64(logData.TS)))*1e9))

		// Filter by time range
		if timestamp.Before(startTime) || timestamp.After(endTime) {
			continue
		}

		entry := logEntry{
			Timestamp: timestamp,
			Status:    logData.Status,
			Duration:  logData.Duration,
			Size:      logData.Size,
		}

		if logData.Request != nil {
			entry.Domain = logData.Request.Host
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	// Group entries by 5-minute buckets
	buckets := a.groupByBucket(entries, 5*time.Minute)

	// Save aggregated metrics
	for _, bucket := range buckets {
		if err := a.store.SavePerformanceMetric(bucket); err != nil {
			return err
		}
	}

	return nil
}
