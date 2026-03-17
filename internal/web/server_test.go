package web

import (
	"testing"
	"time"

	"github.com/lev/internet-monitor/internal/store"
)

func TestBuildSummaryCountsOutages(t *testing.T) {
	base := time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)
	records := []store.Record{
		{Timestamp: base, Status: "up", LatencyMS: 20},
		{Timestamp: base.Add(15 * time.Second), Status: "down"},
		{Timestamp: base.Add(30 * time.Second), Status: "down"},
		{Timestamp: base.Add(45 * time.Second), Status: "degraded", LatencyMS: 90},
		{Timestamp: base.Add(60 * time.Second), Status: "down"},
	}

	summary := buildSummary(records, base, base.Add(75*time.Second), 15*time.Second)
	if summary.Checks != 5 {
		t.Fatalf("checks = %d, want 5", summary.Checks)
	}
	if summary.Outages != 2 {
		t.Fatalf("outages = %d, want 2", summary.Outages)
	}
	if summary.DownChecks != 3 {
		t.Fatalf("down checks = %d, want 3", summary.DownChecks)
	}
	if summary.DegradedChecks != 1 {
		t.Fatalf("degraded checks = %d, want 1", summary.DegradedChecks)
	}
	if summary.DowntimeSec != 45 {
		t.Fatalf("downtime sec = %d, want 45", summary.DowntimeSec)
	}
}

func TestCollapseIntervalsMergesAdjacentStates(t *testing.T) {
	base := time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)
	records := []store.Record{
		{Timestamp: base, Status: "up", Note: "ok"},
		{Timestamp: base.Add(15 * time.Second), Status: "up", Note: "ok"},
		{Timestamp: base.Add(30 * time.Second), Status: "down", Note: "no link"},
		{Timestamp: base.Add(45 * time.Second), Status: "down", Note: "no link"},
	}

	intervals := collapseIntervals(records, base, base.Add(60*time.Second), 15*time.Second)
	if len(intervals) != 2 {
		t.Fatalf("interval count = %d, want 2", len(intervals))
	}
	if intervals[0].Status != "up" || intervals[1].Status != "down" {
		t.Fatalf("unexpected statuses: %+v", intervals)
	}
	if intervals[0].Start != base || intervals[0].End != base.Add(30*time.Second) {
		t.Fatalf("unexpected up interval: %+v", intervals[0])
	}
	if intervals[1].End != base.Add(60*time.Second) {
		t.Fatalf("unexpected down interval end: %+v", intervals[1])
	}
}
