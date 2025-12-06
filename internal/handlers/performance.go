package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

// PerformanceHandler handles performance metrics requests.
type PerformanceHandler struct {
	templates    *templates.Templates
	store        *store.Store
	errorHandler *ErrorHandler
}

// NewPerformanceHandler creates a new PerformanceHandler.
func NewPerformanceHandler(tmpl *templates.Templates, s *store.Store) *PerformanceHandler {
	return &PerformanceHandler{
		templates:    tmpl,
		store:        s,
		errorHandler: NewErrorHandler(tmpl),
	}
}

// PerformanceData holds performance metrics for display.
type PerformanceData struct {
	TimeRange       string
	Labels          []string
	RequestCounts   []int64
	ErrorCounts     []int64
	AvgLatencies    []float64
	P95Latencies    []float64
	Status2xx       []int64
	Status3xx       []int64
	Status4xx       []int64
	Status5xx       []int64
	DomainBandwidth []DomainBandwidthData
	Summary         PerformanceSummary
}

// DomainBandwidthData holds bandwidth data for a domain.
type DomainBandwidthData struct {
	Domain        string
	TotalRequests int64
	TotalBytes    int64
	TotalErrors   int64
	BytesFormatted string
}

// PerformanceSummary holds summary statistics.
type PerformanceSummary struct {
	TotalRequests    int64
	TotalErrors      int64
	ErrorRate        float64
	AvgLatencyMs     float64
	P95LatencyMs     float64
	TotalBytes       int64
	TotalBytesFormatted string
	RequestsPerMin   float64
}

// Data handles GET requests for performance data (JSON format for charts).
func (h *PerformanceHandler) Data(w http.ResponseWriter, r *http.Request) {
	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "1h"
	}

	now := time.Now()
	var start time.Time
	var bucketDuration string

	switch timeRange {
	case "1h":
		start = now.Add(-1 * time.Hour)
		bucketDuration = "5m"
	case "24h":
		start = now.Add(-24 * time.Hour)
		bucketDuration = "5m"
	case "7d":
		start = now.Add(-7 * 24 * time.Hour)
		bucketDuration = "5m"
	case "30d":
		start = now.Add(-30 * 24 * time.Hour)
		bucketDuration = "5m"
	default:
		start = now.Add(-1 * time.Hour)
		bucketDuration = "5m"
	}

	// Get aggregate metrics (empty domain)
	metrics, err := h.store.GetPerformanceMetrics(bucketDuration, "", start, now)
	if err != nil {
		http.Error(w, "Failed to get metrics", http.StatusInternalServerError)
		return
	}

	// Get domain bandwidth summary
	domainBandwidth, err := h.store.GetDomainBandwidthSummary(bucketDuration, start, now)
	if err != nil {
		http.Error(w, "Failed to get bandwidth summary", http.StatusInternalServerError)
		return
	}

	data := h.buildPerformanceData(timeRange, metrics, domainBandwidth)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// Widget handles GET requests for the performance widget partial.
func (h *PerformanceHandler) Widget(w http.ResponseWriter, r *http.Request) {
	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "1h"
	}

	now := time.Now()
	var start time.Time
	bucketDuration := "5m"

	switch timeRange {
	case "1h":
		start = now.Add(-1 * time.Hour)
	case "24h":
		start = now.Add(-24 * time.Hour)
	case "7d":
		start = now.Add(-7 * 24 * time.Hour)
	case "30d":
		start = now.Add(-30 * 24 * time.Hour)
	default:
		start = now.Add(-1 * time.Hour)
	}

	metrics, err := h.store.GetPerformanceMetrics(bucketDuration, "", start, now)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	domainBandwidth, err := h.store.GetDomainBandwidthSummary(bucketDuration, start, now)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	data := h.buildPerformanceData(timeRange, metrics, domainBandwidth)

	if err := h.templates.RenderPartial(w, "performance-widget.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Page handles GET requests for the full performance page.
func (h *PerformanceHandler) Page(w http.ResponseWriter, r *http.Request) {
	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "1h"
	}

	now := time.Now()
	var start time.Time
	bucketDuration := "5m"

	switch timeRange {
	case "1h":
		start = now.Add(-1 * time.Hour)
	case "24h":
		start = now.Add(-24 * time.Hour)
	case "7d":
		start = now.Add(-7 * 24 * time.Hour)
	case "30d":
		start = now.Add(-30 * 24 * time.Hour)
	default:
		start = now.Add(-1 * time.Hour)
	}

	metrics, err := h.store.GetPerformanceMetrics(bucketDuration, "", start, now)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	domainBandwidth, err := h.store.GetDomainBandwidthSummary(bucketDuration, start, now)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	data := h.buildPerformanceData(timeRange, metrics, domainBandwidth)

	pageData := templates.PageData{
		Title:     "Performance",
		ActiveNav: "performance",
		Data:      data,
	}

	if err := h.templates.Render(w, "performance.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

func (h *PerformanceHandler) buildPerformanceData(timeRange string, metrics []store.PerformanceMetric, domainBandwidth []store.DomainBandwidth) PerformanceData {
	data := PerformanceData{
		TimeRange: timeRange,
		Labels:    make([]string, 0, len(metrics)),
		RequestCounts: make([]int64, 0, len(metrics)),
		ErrorCounts:   make([]int64, 0, len(metrics)),
		AvgLatencies:  make([]float64, 0, len(metrics)),
		P95Latencies:  make([]float64, 0, len(metrics)),
		Status2xx:     make([]int64, 0, len(metrics)),
		Status3xx:     make([]int64, 0, len(metrics)),
		Status4xx:     make([]int64, 0, len(metrics)),
		Status5xx:     make([]int64, 0, len(metrics)),
	}

	var totalRequests, totalErrors, totalBytes int64
	var totalLatency, totalP95 float64
	var latencyCount int

	for _, m := range metrics {
		// Format label based on time range
		var label string
		switch timeRange {
		case "1h", "24h":
			label = m.BucketTime.Format("15:04")
		case "7d":
			label = m.BucketTime.Format("Mon 15:04")
		case "30d":
			label = m.BucketTime.Format("Jan 2")
		default:
			label = m.BucketTime.Format("15:04")
		}

		data.Labels = append(data.Labels, label)
		data.RequestCounts = append(data.RequestCounts, m.RequestCount)
		data.ErrorCounts = append(data.ErrorCounts, m.ErrorCount)
		data.AvgLatencies = append(data.AvgLatencies, m.AvgLatencyMs)
		data.P95Latencies = append(data.P95Latencies, m.P95LatencyMs)
		data.Status2xx = append(data.Status2xx, m.Status2xx)
		data.Status3xx = append(data.Status3xx, m.Status3xx)
		data.Status4xx = append(data.Status4xx, m.Status4xx)
		data.Status5xx = append(data.Status5xx, m.Status5xx)

		totalRequests += m.RequestCount
		totalErrors += m.ErrorCount
		totalBytes += m.TotalBytes
		if m.AvgLatencyMs > 0 {
			totalLatency += m.AvgLatencyMs
			totalP95 += m.P95LatencyMs
			latencyCount++
		}
	}

	// Calculate summary
	data.Summary.TotalRequests = totalRequests
	data.Summary.TotalErrors = totalErrors
	data.Summary.TotalBytes = totalBytes
	data.Summary.TotalBytesFormatted = formatBytes(totalBytes)

	if totalRequests > 0 {
		data.Summary.ErrorRate = float64(totalErrors) / float64(totalRequests) * 100
	}

	if latencyCount > 0 {
		data.Summary.AvgLatencyMs = totalLatency / float64(latencyCount)
		data.Summary.P95LatencyMs = totalP95 / float64(latencyCount)
	}

	// Calculate requests per minute based on time range
	var minutes float64
	switch timeRange {
	case "1h":
		minutes = 60
	case "24h":
		minutes = 24 * 60
	case "7d":
		minutes = 7 * 24 * 60
	case "30d":
		minutes = 30 * 24 * 60
	default:
		minutes = 60
	}
	if minutes > 0 {
		data.Summary.RequestsPerMin = float64(totalRequests) / minutes
	}

	// Build domain bandwidth data
	for _, d := range domainBandwidth {
		data.DomainBandwidth = append(data.DomainBandwidth, DomainBandwidthData{
			Domain:         d.Domain,
			TotalRequests:  d.TotalRequests,
			TotalBytes:     d.TotalBytes,
			TotalErrors:    d.TotalErrors,
			BytesFormatted: formatBytes(d.TotalBytes),
		})
	}

	return data
}

// formatBytes formats bytes into human-readable format.
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return formatFloat(float64(bytes)/float64(TB)) + " TB"
	case bytes >= GB:
		return formatFloat(float64(bytes)/float64(GB)) + " GB"
	case bytes >= MB:
		return formatFloat(float64(bytes)/float64(MB)) + " MB"
	case bytes >= KB:
		return formatFloat(float64(bytes)/float64(KB)) + " KB"
	default:
		return formatFloat(float64(bytes)) + " B"
	}
}

func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return string(rune(int64(f)+'0'))
	}
	// Simple formatting without fmt
	whole := int64(f)
	frac := int64((f - float64(whole)) * 100)
	if frac == 0 {
		return intToString(whole)
	}
	if frac%10 == 0 {
		return intToString(whole) + "." + intToString(frac/10)
	}
	return intToString(whole) + "." + intToString(frac)
}

func intToString(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
