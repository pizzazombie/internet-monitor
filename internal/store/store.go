package store

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const dayLayout = "2006-01-02"

type Record struct {
	Timestamp  time.Time `json:"ts"`
	Status     string    `json:"status"`
	Internet   bool      `json:"internet"`
	LANOK      bool      `json:"lan_ok,omitempty"`
	TCPOK      bool      `json:"tcp_ok"`
	HTTPStatus int       `json:"http_status,omitempty"`
	HTTPOK     bool      `json:"http_ok"`
	DNSOK      bool      `json:"dns_ok"`
	LatencyMS  int64     `json:"latency_ms,omitempty"`
	SpeedMbps  float64   `json:"speed_mbps,omitempty"`
	Note       string    `json:"note,omitempty"`
}

type FileStore struct {
	root          string
	retentionDays int
	mu            sync.Mutex
}

func New(root string, retentionDays int) *FileStore {
	return &FileStore{root: root, retentionDays: retentionDays}
}

func (s *FileStore) EnsureLayout() error {
	return os.MkdirAll(s.root, 0o755)
}

func (s *FileStore) Append(record Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.pathFor(record.Timestamp)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(record); err != nil {
		return err
	}

	if s.retentionDays > 0 {
		return s.cleanupLocked(record.Timestamp)
	}
	return nil
}

func (s *FileStore) LoadRange(ctx context.Context, from, to time.Time) ([]Record, error) {
	if to.Before(from) {
		return nil, errors.New("invalid range")
	}

	records := make([]Record, 0, 1024)
	for current := dayStart(from); !current.After(to); current = current.Add(24 * time.Hour) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		path := s.pathFor(current)
		file, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}

		dayRecords, err := decodeRecords(file, from, to)
		file.Close()
		if err != nil {
			return nil, err
		}
		records = append(records, dayRecords...)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp.Before(records[j].Timestamp)
	})

	return records, nil
}

func decodeRecords(reader io.Reader, from, to time.Time) ([]Record, error) {
	scanner := bufio.NewScanner(reader)
	records := make([]Record, 0, 1024)

	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		if record.Timestamp.Before(from) || record.Timestamp.After(to) {
			continue
		}
		records = append(records, record)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *FileStore) pathFor(ts time.Time) string {
	return filepath.Join(s.root, fmt.Sprintf("%s.ndjson", ts.Format(dayLayout)))
}

func (s *FileStore) cleanupLocked(now time.Time) error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}

	cutoff := dayStart(now).AddDate(0, 0, -s.retentionDays)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		datePart := entry.Name()
		if filepath.Ext(datePart) != ".ndjson" {
			continue
		}
		datePart = datePart[:len(datePart)-len(filepath.Ext(datePart))]
		recordDay, err := time.ParseInLocation(dayLayout, datePart, now.Location())
		if err != nil {
			continue
		}
		if recordDay.Before(cutoff) {
			if err := os.Remove(filepath.Join(s.root, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func dayStart(ts time.Time) time.Time {
	return time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, ts.Location())
}
