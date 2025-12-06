package store

import (
	"testing"
	"time"
)

func TestStore_SaveAndGetPerformanceMetric(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().Truncate(5 * time.Minute)
	metric := &PerformanceMetric{
		BucketTime:     now,
		BucketDuration: "5m",
		Domain:         "",
		RequestCount:   100,
		ErrorCount:     5,
		TotalBytes:     1024 * 1024,
		AvgLatencyMs:   50.5,
		MinLatencyMs:   10.0,
		MaxLatencyMs:   200.0,
		P50LatencyMs:   45.0,
		P95LatencyMs:   150.0,
		P99LatencyMs:   180.0,
		Status2xx:      90,
		Status3xx:      2,
		Status4xx:      3,
		Status5xx:      5,
	}

	// Save metric
	err := s.SavePerformanceMetric(metric)
	if err != nil {
		t.Fatalf("SavePerformanceMetric() error = %v", err)
	}

	// Retrieve metric
	metrics, err := s.GetPerformanceMetrics("5m", "", now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("GetPerformanceMetrics() error = %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	got := metrics[0]
	if got.RequestCount != metric.RequestCount {
		t.Errorf("RequestCount = %d, want %d", got.RequestCount, metric.RequestCount)
	}
	if got.ErrorCount != metric.ErrorCount {
		t.Errorf("ErrorCount = %d, want %d", got.ErrorCount, metric.ErrorCount)
	}
	if got.AvgLatencyMs != metric.AvgLatencyMs {
		t.Errorf("AvgLatencyMs = %f, want %f", got.AvgLatencyMs, metric.AvgLatencyMs)
	}
}

func TestStore_SavePerformanceMetric_Upsert(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().Truncate(5 * time.Minute)
	metric := &PerformanceMetric{
		BucketTime:     now,
		BucketDuration: "5m",
		Domain:         "",
		RequestCount:   100,
		ErrorCount:     5,
	}

	// Save initial metric
	if err := s.SavePerformanceMetric(metric); err != nil {
		t.Fatalf("SavePerformanceMetric() first call error = %v", err)
	}

	// Update with new values
	metric.RequestCount = 200
	metric.ErrorCount = 10

	if err := s.SavePerformanceMetric(metric); err != nil {
		t.Fatalf("SavePerformanceMetric() second call error = %v", err)
	}

	// Retrieve and verify update
	metrics, err := s.GetPerformanceMetrics("5m", "", now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("GetPerformanceMetrics() error = %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	if metrics[0].RequestCount != 200 {
		t.Errorf("RequestCount = %d, want 200", metrics[0].RequestCount)
	}
}

func TestStore_GetPerformanceMetricsByDomain(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().Truncate(5 * time.Minute)

	// Save metrics for different domains
	for _, domain := range []string{"example.com", "test.com"} {
		metric := &PerformanceMetric{
			BucketTime:     now,
			BucketDuration: "5m",
			Domain:         domain,
			RequestCount:   50,
		}
		if err := s.SavePerformanceMetric(metric); err != nil {
			t.Fatalf("SavePerformanceMetric() error = %v", err)
		}
	}

	// Retrieve metrics by domain
	metrics, err := s.GetPerformanceMetricsByDomain("5m", now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("GetPerformanceMetricsByDomain() error = %v", err)
	}

	if len(metrics) != 2 {
		t.Errorf("Expected 2 metrics, got %d", len(metrics))
	}
}

func TestStore_GetDomainBandwidthSummary(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().Truncate(5 * time.Minute)

	// Save metrics for different domains
	domains := []struct {
		domain string
		bytes  int64
	}{
		{"example.com", 1024 * 1024},
		{"test.com", 2 * 1024 * 1024},
	}

	for _, d := range domains {
		metric := &PerformanceMetric{
			BucketTime:     now,
			BucketDuration: "5m",
			Domain:         d.domain,
			RequestCount:   100,
			TotalBytes:     d.bytes,
		}
		if err := s.SavePerformanceMetric(metric); err != nil {
			t.Fatalf("SavePerformanceMetric() error = %v", err)
		}
	}

	// Get bandwidth summary
	summary, err := s.GetDomainBandwidthSummary("5m", now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("GetDomainBandwidthSummary() error = %v", err)
	}

	if len(summary) != 2 {
		t.Errorf("Expected 2 domains in summary, got %d", len(summary))
	}

	// Should be sorted by bytes descending
	if summary[0].Domain != "test.com" {
		t.Errorf("Expected test.com first (most bytes), got %s", summary[0].Domain)
	}
}

func TestStore_PrunePerformanceMetrics(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().Truncate(5 * time.Minute)
	old := now.Add(-48 * time.Hour)

	// Save old metric
	oldMetric := &PerformanceMetric{
		BucketTime:     old,
		BucketDuration: "5m",
		Domain:         "",
		RequestCount:   100,
	}
	if err := s.SavePerformanceMetric(oldMetric); err != nil {
		t.Fatalf("SavePerformanceMetric() error = %v", err)
	}

	// Save new metric
	newMetric := &PerformanceMetric{
		BucketTime:     now,
		BucketDuration: "5m",
		Domain:         "",
		RequestCount:   200,
	}
	if err := s.SavePerformanceMetric(newMetric); err != nil {
		t.Fatalf("SavePerformanceMetric() error = %v", err)
	}

	// Prune metrics older than 24 hours
	pruneTime := now.Add(-24 * time.Hour)
	deleted, err := s.PrunePerformanceMetrics(pruneTime)
	if err != nil {
		t.Fatalf("PrunePerformanceMetrics() error = %v", err)
	}

	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Verify only new metric remains
	metrics, err := s.GetPerformanceMetrics("5m", "", old.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetPerformanceMetrics() error = %v", err)
	}

	if len(metrics) != 1 {
		t.Errorf("Expected 1 metric remaining, got %d", len(metrics))
	}
}

func TestStore_GetLatestMetricTime(t *testing.T) {
	s := newTestStore(t)

	// Should return zero time when no metrics
	latestTime, err := s.GetLatestMetricTime("5m")
	if err != nil {
		t.Fatalf("GetLatestMetricTime() error = %v", err)
	}
	if !latestTime.IsZero() {
		t.Errorf("Expected zero time for empty table, got %v", latestTime)
	}

	// Add some metrics
	now := time.Now().Truncate(5 * time.Minute)
	for i := 0; i < 3; i++ {
		metric := &PerformanceMetric{
			BucketTime:     now.Add(-time.Duration(i) * 5 * time.Minute),
			BucketDuration: "5m",
			Domain:         "",
			RequestCount:   100,
		}
		if err := s.SavePerformanceMetric(metric); err != nil {
			t.Fatalf("SavePerformanceMetric() error = %v", err)
		}
	}

	// Get latest
	latestTime, err = s.GetLatestMetricTime("5m")
	if err != nil {
		t.Fatalf("GetLatestMetricTime() error = %v", err)
	}

	if !latestTime.Equal(now) {
		t.Errorf("Expected latest time %v, got %v", now, latestTime)
	}
}
