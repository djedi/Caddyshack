package handlers

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/templates"
)

// DefaultLogLines is the default number of log lines to read.
const DefaultLogLines = 100

// LogEntry represents a parsed log entry.
type LogEntry struct {
	Timestamp   string
	Level       string
	LevelColor  string
	Message     string
	Domain      string
	Method      string
	URI         string
	Status      int
	Duration    string
	RemoteIP    string
	RawLine     string
	IsJSON      bool
}

// LogsData holds data displayed on the logs page.
type LogsData struct {
	Entries        []LogEntry
	Lines          []string
	LogPath        string
	Error          string
	HasError       bool
	FileExists     bool
	TotalLines     int
	DisplayedLines int
	// Filter parameters
	FilterLevel    string
	FilterDomain   string
	FilterSearch   string
	// Available filter options
	AvailableLevels  []string
	AvailableDomains []string
}

// LogsHandler handles requests for the logs pages.
type LogsHandler struct {
	templates    *templates.Templates
	config       *config.Config
	errorHandler *ErrorHandler
}

// NewLogsHandler creates a new LogsHandler.
func NewLogsHandler(tmpl *templates.Templates, cfg *config.Config) *LogsHandler {
	return &LogsHandler{
		templates:    tmpl,
		config:       cfg,
		errorHandler: NewErrorHandler(tmpl),
	}
}

// List handles GET requests for the logs page.
func (h *LogsHandler) List(w http.ResponseWriter, r *http.Request) {
	data := LogsData{
		AvailableLevels: []string{"debug", "info", "warn", "error"},
	}

	// Get filter parameters from query string
	data.FilterLevel = r.URL.Query().Get("level")
	data.FilterDomain = r.URL.Query().Get("domain")
	data.FilterSearch = r.URL.Query().Get("search")

	// Determine log path
	logPath := h.getLogPath()
	data.LogPath = logPath

	if logPath == "" {
		data.Error = "No log file path configured. Set CADDYSHACK_LOG_PATH or configure logging in Caddyfile global options."
		data.HasError = true
	} else {
		// Check if file exists
		if _, err := os.Stat(logPath); os.IsNotExist(err) {
			data.Error = "Log file not found: " + logPath
			data.HasError = true
			data.FileExists = false
		} else if err != nil {
			data.Error = "Error accessing log file: " + err.Error()
			data.HasError = true
		} else {
			data.FileExists = true

			// Read last N lines
			lines, total, err := readLastNLines(logPath, DefaultLogLines)
			if err != nil {
				data.Error = "Error reading log file: " + err.Error()
				data.HasError = true
			} else {
				data.Lines = lines
				data.TotalLines = total

				// Parse log entries
				allEntries := parseLogEntries(lines)

				// Collect available domains
				domainSet := make(map[string]bool)
				for _, entry := range allEntries {
					if entry.Domain != "" {
						domainSet[entry.Domain] = true
					}
				}
				for domain := range domainSet {
					data.AvailableDomains = append(data.AvailableDomains, domain)
				}

				// Apply filters
				data.Entries = filterLogEntries(allEntries, data.FilterLevel, data.FilterDomain, data.FilterSearch)
				data.DisplayedLines = len(data.Entries)
			}
		}
	}

	// Check if this is an HTMX request for partial update
	if r.Header.Get("HX-Request") == "true" {
		// Return only the log entries table
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "logs-entries.html", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	pageData := templates.PageData{
		Title:     "Logs",
		ActiveNav: "logs",
		Data:      data,
	}

	if err := h.templates.Render(w, "logs.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// getLogPath determines the log file path from config or Caddyfile.
func (h *LogsHandler) getLogPath() string {
	// First, check if explicitly configured
	if h.config.LogPath != "" {
		return h.config.LogPath
	}

	// Try to auto-detect from Caddyfile global options
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		return ""
	}

	parser := caddy.NewParser(content)
	globalOpts, err := parser.ParseGlobalOptions()
	if err != nil || globalOpts == nil || globalOpts.LogConfig == nil {
		return ""
	}

	// Parse the output field - it could be "file /path/to/log" or just a path
	output := globalOpts.LogConfig.Output
	if output == "" {
		return ""
	}

	// Handle "file /path/to/log" format
	if strings.HasPrefix(output, "file ") {
		parts := strings.SplitN(output, " ", 2)
		if len(parts) >= 2 {
			return strings.TrimSpace(parts[1])
		}
	}

	// Handle direct path (without "file" prefix)
	// This might be "stdout", "stderr", or a path
	if output == "stdout" || output == "stderr" {
		return ""
	}

	return output
}

// readLastNLines reads the last n lines from a file.
// Returns the lines, total line count, and any error.
func readLastNLines(filePath string, n int) ([]string, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}

	// For small files, just read everything
	if stat.Size() < 1024*1024 { // Less than 1MB
		return readAllLines(file, n)
	}

	// For larger files, read from the end
	return readLinesFromEnd(file, stat.Size(), n)
}

// readAllLines reads all lines from a file and returns the last n.
func readAllLines(file *os.File, n int) ([]string, int, error) {
	var lines []string
	scanner := bufio.NewScanner(file)

	// Set a larger buffer for potentially long log lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	totalLines := len(lines)

	// Return last n lines
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return lines, totalLines, nil
}

// readLinesFromEnd reads lines from the end of a large file.
func readLinesFromEnd(file *os.File, fileSize int64, n int) ([]string, int, error) {
	// Read in chunks from the end
	chunkSize := int64(64 * 1024) // 64KB chunks
	if chunkSize > fileSize {
		chunkSize = fileSize
	}

	var lines []string
	var leftover []byte
	position := fileSize

	// We'll estimate total lines based on what we read
	estimatedTotal := 0

	for len(lines) < n && position > 0 {
		// Calculate read position
		readStart := position - chunkSize
		if readStart < 0 {
			chunkSize = position
			readStart = 0
		}

		// Seek and read
		_, err := file.Seek(readStart, io.SeekStart)
		if err != nil {
			return nil, 0, err
		}

		chunk := make([]byte, chunkSize)
		_, err = io.ReadFull(file, chunk)
		if err != nil && err != io.ErrUnexpectedEOF {
			return nil, 0, err
		}

		// Prepend leftover from previous chunk
		if len(leftover) > 0 {
			chunk = append(chunk, leftover...)
		}

		// Split into lines
		chunkLines := strings.Split(string(chunk), "\n")

		// First element might be partial line (save for next iteration)
		if readStart > 0 {
			leftover = []byte(chunkLines[0])
			chunkLines = chunkLines[1:]
		}

		// Prepend lines (in reverse order to maintain chronology)
		for i := len(chunkLines) - 1; i >= 0; i-- {
			line := chunkLines[i]
			if line != "" || (i > 0) { // Include empty lines except trailing
				lines = append([]string{line}, lines...)
				estimatedTotal++
			}
		}

		position = readStart
	}

	// If we have leftover at the beginning, prepend it
	if len(leftover) > 0 {
		lines = append([]string{string(leftover)}, lines...)
		estimatedTotal++
	}

	// Trim to last n lines
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	// Filter out empty trailing lines
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines, estimatedTotal, nil
}

// filterLogEntries filters log entries based on level, domain, and search criteria.
func filterLogEntries(entries []LogEntry, level, domain, search string) []LogEntry {
	if level == "" && domain == "" && search == "" {
		return entries
	}

	filtered := make([]LogEntry, 0, len(entries))
	search = strings.ToLower(search)

	for _, entry := range entries {
		// Filter by level
		if level != "" && !strings.EqualFold(entry.Level, level) {
			continue
		}

		// Filter by domain
		if domain != "" && !strings.EqualFold(entry.Domain, domain) {
			continue
		}

		// Filter by search term
		if search != "" {
			searchable := strings.ToLower(entry.RawLine + " " + entry.Message + " " + entry.URI)
			if !strings.Contains(searchable, search) {
				continue
			}
		}

		filtered = append(filtered, entry)
	}

	return filtered
}

// parseLogEntries parses raw log lines into structured LogEntry objects.
func parseLogEntries(lines []string) []LogEntry {
	entries := make([]LogEntry, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		entry := LogEntry{RawLine: line}

		// Try to parse as JSON
		if strings.HasPrefix(line, "{") {
			if parsed := parseJSONLogLine(line); parsed != nil {
				entry = *parsed
				entry.RawLine = line
				entry.IsJSON = true
			}
		}

		entries = append(entries, entry)
	}

	return entries
}

// caddyLogEntry represents the structure of a Caddy JSON log entry.
type caddyLogEntry struct {
	Level   string  `json:"level"`
	TS      float64 `json:"ts"`
	Logger  string  `json:"logger"`
	Msg     string  `json:"msg"`
	Request *struct {
		RemoteIP   string `json:"remote_ip"`
		RemotePort string `json:"remote_port"`
		ClientIP   string `json:"client_ip"`
		Proto      string `json:"proto"`
		Method     string `json:"method"`
		Host       string `json:"host"`
		URI        string `json:"uri"`
		Headers    map[string][]string `json:"headers"`
	} `json:"request"`
	Status      int     `json:"status"`
	Duration    float64 `json:"duration"`
	Size        int64   `json:"size"`
	RespHeaders map[string][]string `json:"resp_headers"`
}

// parseJSONLogLine parses a JSON log line into a LogEntry.
func parseJSONLogLine(line string) *LogEntry {
	var logData caddyLogEntry
	if err := json.Unmarshal([]byte(line), &logData); err != nil {
		return nil
	}

	entry := &LogEntry{
		Level:   logData.Level,
		Message: logData.Msg,
	}

	// Set level color
	switch strings.ToLower(logData.Level) {
	case "debug":
		entry.LevelColor = "gray"
	case "info":
		entry.LevelColor = "blue"
	case "warn", "warning":
		entry.LevelColor = "yellow"
	case "error":
		entry.LevelColor = "red"
	case "fatal", "panic":
		entry.LevelColor = "red"
	default:
		entry.LevelColor = "gray"
	}

	// Parse timestamp
	if logData.TS > 0 {
		sec := int64(logData.TS)
		nsec := int64((logData.TS - float64(sec)) * 1e9)
		t := time.Unix(sec, nsec)
		entry.Timestamp = t.Format("2006-01-02 15:04:05")
	}

	// Parse request details
	if logData.Request != nil {
		entry.Domain = logData.Request.Host
		entry.Method = logData.Request.Method
		entry.URI = logData.Request.URI
		if logData.Request.ClientIP != "" {
			entry.RemoteIP = logData.Request.ClientIP
		} else {
			entry.RemoteIP = logData.Request.RemoteIP
		}
	}

	// Parse response details
	if logData.Status > 0 {
		entry.Status = logData.Status
	}

	// Parse duration
	if logData.Duration > 0 {
		// Duration is in seconds
		if logData.Duration < 0.001 {
			entry.Duration = formatDuration(logData.Duration * 1e6, "µs")
		} else if logData.Duration < 1 {
			entry.Duration = formatDuration(logData.Duration * 1e3, "ms")
		} else {
			entry.Duration = formatDuration(logData.Duration, "s")
		}
	}

	return entry
}

// formatDuration formats a duration value with the given unit.
func formatDuration(value float64, unit string) string {
	if value < 1 {
		return "<1" + unit
	}
	if value < 10 {
		// Format with one decimal place
		whole := int(value)
		frac := int((value - float64(whole)) * 10)
		if frac > 0 {
			return string([]byte{byte('0' + whole), '.', byte('0' + frac)}) + unit
		}
		return string([]byte{byte('0' + whole)}) + unit
	}
	if value < 100 {
		return string([]byte{byte('0' + int(value/10)%10), byte('0' + int(value)%10)}) + unit
	}
	if value < 1000 {
		return string([]byte{byte('0' + int(value/100)%10), byte('0' + int(value/10)%10), byte('0' + int(value)%10)}) + unit
	}
	return ">1000" + unit
}

