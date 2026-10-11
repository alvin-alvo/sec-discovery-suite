package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Verify schedules initialized
	schedules, err := s.GetSchedules(ctx)
	if err != nil {
		t.Fatalf("GetSchedules failed: %v", err)
	}
	if len(schedules) != 2 {
		t.Fatalf("expected 2 schedules, got %d", len(schedules))
	}

	// Create run
	now := time.Now().UTC()
	runID := "01JTEST0000000000000000000"
	err = s.CreateRun(ctx, &Run{
		RunID:       runID,
		Type:        "filesystem",
		Trigger:     "manual",
		Status:      "queued",
		RequestedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	// Get run
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if run == nil || run.RunID != runID || run.Status != "queued" {
		t.Fatalf("unexpected run: %+v", run)
	}

	// HasActiveRunOfType
	active, err := s.HasActiveRunOfType(ctx, "filesystem")
	if err != nil {
		t.Fatalf("HasActiveRunOfType failed: %v", err)
	}
	if !active {
		t.Fatalf("expected active run of type filesystem")
	}

	// Update run status
	startedAt := time.Now().UTC()
	err = s.UpdateRunStatus(ctx, runID, "running", &startedAt, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateRunStatus failed: %v", err)
	}

	run, err = s.GetRun(ctx, runID)
	if err != nil || run.Status != "running" {
		t.Fatalf("expected status running, got %v (%+v)", err, run)
	}

	// List runs
	runs, _, err := s.ListRuns(ctx, "filesystem", "", "", 10)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
}
