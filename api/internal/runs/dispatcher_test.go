package runs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"sds/api/internal/store"
)

func TestDispatcher_SuccessAndFailure(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// 1. Test when agent succeeds (returns 202)
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/scan/fs" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))
	defer agentServer.Close()

	d := NewDispatcher(s, agentServer.URL, "secret", "http://127.0.0.1:8080")

	run, err := d.CreateAndDispatchRun(ctx, "filesystem", "manual", nil)
	if err != nil {
		t.Fatalf("CreateAndDispatchRun failed: %v", err)
	}
	if run.Status != "queued" {
		t.Fatalf("expected queued, got %s", run.Status)
	}

	// Wait for background goroutine
	time.Sleep(100 * time.Millisecond)

	updatedRun, err := s.GetRun(ctx, run.RunID)
	if err != nil || updatedRun == nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if updatedRun.Status != "running" {
		t.Fatalf("expected status running, got %s", updatedRun.Status)
	}

	// 2. Test when agent is unreachable
	dUnreachable := NewDispatcher(s, "http://127.0.0.1:54321", "secret", "http://127.0.0.1:8080")
	run2, err := dUnreachable.CreateAndDispatchRun(ctx, "filesystem", "manual", nil)
	if err != nil {
		t.Fatalf("CreateAndDispatchRun failed: %v", err)
	}

	// Wait for connection failure
	time.Sleep(200 * time.Millisecond)

	failedRun, err := s.GetRun(ctx, run2.RunID)
	if err != nil || failedRun == nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if failedRun.Status != "failed" {
		t.Fatalf("expected status failed, got %s", failedRun.Status)
	}
	if failedRun.ErrorCount != 1 || failedRun.ErrorSummary == nil {
		t.Fatalf("expected error summary on failure, got %+v", failedRun)
	}
}
