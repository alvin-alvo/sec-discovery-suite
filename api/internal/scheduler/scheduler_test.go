package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"sds/api/internal/runs"
	"sds/api/internal/store"
)

func TestScheduler_Tick(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Enable the filesystem schedule and set next_run_at to past
	past := time.Now().Add(-10 * time.Minute).UTC()
	err = s.UpdateSchedule(ctx, store.Schedule{
		ID:              "filesystem",
		IntervalSeconds: 60,
		Enabled:         true,
		NextRunAt:       &past,
	})
	if err != nil {
		t.Fatalf("UpdateSchedule failed: %v", err)
	}

	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer agentServer.Close()

	d := runs.NewDispatcher(s, agentServer.URL, "secret", "http://127.0.0.1:8080")
	sched := NewScheduler(s, d, 50*time.Millisecond)

	// Run one tick manually
	sched.tick()

	// Check that a run was created
	runList, _, err := s.ListRuns(ctx, "filesystem", "", "", 10)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runList) != 1 {
		t.Fatalf("expected 1 scheduled run, got %d", len(runList))
	}
	if runList[0].Trigger != "scheduled" {
		t.Fatalf("expected trigger=scheduled, got %s", runList[0].Trigger)
	}

	// Tick again immediately while run is active => should skip and NOT create another run
	sched.tick()
	runList2, _, err := s.ListRuns(ctx, "filesystem", "", "", 10)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runList2) != 1 {
		t.Fatalf("expected still 1 run because previous run was active, got %d", len(runList2))
	}
}
