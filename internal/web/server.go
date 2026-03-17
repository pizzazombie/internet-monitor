package web

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lev/internet-monitor/internal/config"
	"github.com/lev/internet-monitor/internal/store"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	cfg   config.Config
	store *store.FileStore
}

type interval struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Status string    `json:"status"`
	Note   string    `json:"note"`
}

type point struct {
	Timestamp time.Time `json:"ts"`
	Value     float64   `json:"value"`
}

type summary struct {
	From           time.Time `json:"from"`
	To             time.Time `json:"to"`
	Checks         int       `json:"checks"`
	Outages        int       `json:"outages"`
	DownChecks     int       `json:"down_checks"`
	DegradedChecks int       `json:"degraded_checks"`
	DowntimeSec    int64     `json:"downtime_sec"`
	UptimePct      float64   `json:"uptime_pct"`
	AvgLatencyMS   float64   `json:"avg_latency_ms"`
	LastStatus     string    `json:"last_status"`
	LastCheckedAt  time.Time `json:"last_checked_at"`
}

type overviewResponse struct {
	Summary  summary    `json:"summary"`
	Timeline []interval `json:"timeline"`
	Latency  []point    `json:"latency"`
	Speed    []point    `json:"speed"`
}

func NewServer(cfg config.Config, fileStore *store.FileStore) http.Handler {
	server := &Server{cfg: cfg, store: fileStore}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/overview", server.handleOverview)
	mux.HandleFunc("/healthz", server.handleHealth)

	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))
	return withJSONHeaders(mux)
}

func (s *Server) handleHealth(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleOverview(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
	defer cancel()

	from, to, err := parseRange(request)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	records, err := s.store.LoadRange(ctx, from, to)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}

	response := overviewResponse{
		Summary:  buildSummary(records, from, to, s.cfg.CheckInterval),
		Timeline: collapseIntervals(records, from, to, s.cfg.CheckInterval),
		Latency:  sampleSeries(records, s.cfg.MaxRenderedSamples, func(record store.Record) (float64, bool) { return float64(record.LatencyMS), record.LatencyMS > 0 }),
		Speed:    sampleSeries(records, s.cfg.MaxRenderedPoints, func(record store.Record) (float64, bool) { return record.SpeedMbps, record.SpeedMbps > 0 }),
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(response)
}

func parseRange(request *http.Request) (time.Time, time.Time, error) {
	query := request.URL.Query()
	now := time.Now().UTC()

	to := parseTime(query.Get("to"), now)
	if value := strings.TrimSpace(query.Get("hours")); value != "" {
		hours, err := strconv.Atoi(value)
		if err != nil || hours <= 0 {
			return time.Time{}, time.Time{}, errInvalid("invalid hours")
		}
		return to.Add(-time.Duration(hours) * time.Hour), to, nil
	}

	from := parseTime(query.Get("from"), to.Add(-24*time.Hour))
	if !to.After(from) {
		return time.Time{}, time.Time{}, errInvalid("range must be positive")
	}
	return from, to, nil
}

func parseTime(raw string, fallback time.Time) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return fallback
	}
	return parsed.UTC()
}

func buildSummary(records []store.Record, from, to time.Time, interval time.Duration) summary {
	result := summary{From: from, To: to}
	if len(records) == 0 {
		return result
	}

	var uptimeChecks int
	var latencyTotal int64
	var latencyCount int64
	var outages int
	var downtimeSec int64
	prevUp := true

	for _, record := range records {
		result.Checks++
		if record.Status == "down" {
			result.DownChecks++
			downtimeSec += int64(interval.Seconds())
		}
		if record.Status == "degraded" {
			result.DegradedChecks++
		}
		if record.Status != "down" {
			uptimeChecks++
		}
		if record.LatencyMS > 0 {
			latencyTotal += record.LatencyMS
			latencyCount++
		}

		isUp := record.Status != "down"
		if !isUp && prevUp {
			outages++
		}
		prevUp = isUp
	}

	result.Outages = outages
	result.DowntimeSec = downtimeSec
	result.UptimePct = 100 * float64(uptimeChecks) / float64(len(records))
	if latencyCount > 0 {
		result.AvgLatencyMS = float64(latencyTotal) / float64(latencyCount)
	}
	last := records[len(records)-1]
	result.LastStatus = last.Status
	result.LastCheckedAt = last.Timestamp
	return result
}

func collapseIntervals(records []store.Record, from, to time.Time, checkInterval time.Duration) []interval {
	if len(records) == 0 {
		return nil
	}

	intervals := make([]interval, 0, len(records))
	current := interval{
		Start:  maxTime(records[0].Timestamp, from),
		End:    minTime(records[0].Timestamp.Add(checkInterval), to),
		Status: records[0].Status,
		Note:   records[0].Note,
	}

	for i := 1; i < len(records); i++ {
		record := records[i]
		start := maxTime(record.Timestamp, from)
		end := minTime(record.Timestamp.Add(checkInterval), to)
		if record.Status == current.Status && start.Sub(current.End) <= checkInterval {
			current.End = end
			current.Note = record.Note
			continue
		}
		intervals = append(intervals, current)
		current = interval{
			Start:  start,
			End:    end,
			Status: record.Status,
			Note:   record.Note,
		}
	}

	intervals = append(intervals, current)
	return intervals
}

func sampleSeries(records []store.Record, maxPoints int, extractor func(store.Record) (float64, bool)) []point {
	source := make([]point, 0, len(records))
	for _, record := range records {
		value, ok := extractor(record)
		if !ok {
			continue
		}
		source = append(source, point{Timestamp: record.Timestamp, Value: value})
	}
	if len(source) <= maxPoints || maxPoints <= 0 {
		return source
	}

	bucketSize := int(math.Ceil(float64(len(source)) / float64(maxPoints)))
	if bucketSize < 1 {
		bucketSize = 1
	}

	reduced := make([]point, 0, maxPoints)
	for i := 0; i < len(source); i += bucketSize {
		end := i + bucketSize
		if end > len(source) {
			end = len(source)
		}

		var sum float64
		for _, sample := range source[i:end] {
			sum += sample.Value
		}
		reduced = append(reduced, point{
			Timestamp: source[end-1].Timestamp,
			Value:     sum / float64(end-i),
		})
	}
	return reduced
}

func withJSONHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(writer, request)
	})
}

type invalidError struct {
	message string
}

func (e invalidError) Error() string { return e.message }

func errInvalid(message string) error {
	return invalidError{message: message}
}

func maxTime(left, right time.Time) time.Time {
	if left.After(right) {
		return left
	}
	return right
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}
