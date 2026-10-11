package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"sds/api/internal/runs"
	"sds/api/internal/store"
)

func TestRouter_Endpoints(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	d := runs.NewDispatcher(s, "http://127.0.0.1:54321", "testtoken", "http://127.0.0.1:8080")

	handler := NewRouter(ServerConfig{
		Store:        s,
		Dispatcher:   d,
		Token:        "testtoken",
		AgentAddr:    "http://127.0.0.1:54321",
		CallbackBase: "http://127.0.0.1:8080",
	})

	// 1. Health
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /health expected 200, got %d", rr.Code)
	}

	// 2. Agent status (offline safe)
	req = httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /agent/status expected 200, got %d", rr.Code)
	}
	var agentStatus map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&agentStatus)
	if agentStatus["online"] != false {
		t.Fatalf("expected online=false when agent is unreachable, got %v", agentStatus["online"])
	}

	// 3. POST /api/v1/runs
	body := bytes.NewBufferString(`{"type":"filesystem"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/runs", body)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("POST /api/v1/runs expected 202, got %d (%s)", rr.Code, rr.Body.String())
	}
	var runResp map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&runResp)
	runID := runResp["run_id"]
	if runID == "" {
		t.Fatalf("expected non-empty run_id")
	}

	// 4. GET /api/v1/runs
	req = httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/runs expected 200, got %d", rr.Code)
	}

	// 5. GET /api/v1/runs/{id}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+runID, nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/runs/{id} expected 200, got %d", rr.Code)
	}

	// 6. GET /api/v1/runs/{id}/fs/summary
	req = httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+runID+"/fs/summary", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET fs summary expected 200, got %d", rr.Code)
	}

	// 7. GET /api/v1/fs/certificates
	req = httptest.NewRequest(http.MethodGet, "/api/v1/fs/certificates", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET certificates expected 200, got %d", rr.Code)
	}

	// 8. GET /api/v1/fs/facets
	req = httptest.NewRequest(http.MethodGet, "/api/v1/fs/facets", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET facets expected 200, got %d", rr.Code)
	}

	// 9. GET /api/v1/graph?scope=global
	req = httptest.NewRequest(http.MethodGet, "/api/v1/graph?scope=global", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET global graph expected 200, got %d", rr.Code)
	}

	// 10. GET /api/v1/dashboard/summary
	req = httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET dashboard summary expected 200, got %d", rr.Code)
	}

	// 11. GET /api/v1/schedules & PUT
	req = httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET schedules expected 200, got %d", rr.Code)
	}

	putBody := bytes.NewBufferString(`{"id":"filesystem","interval_seconds":3600,"enabled":true}`)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/schedules", putBody)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT schedules expected 200, got %d", rr.Code)
	}

	// 12. SPA routing & fallback
	// Non-/api route should return 200 and index.html
	req = httptest.NewRequest(http.MethodGet, "/runs", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("SPA route /runs expected 200, got %d", rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("html")) {
		t.Fatalf("expected HTML in response, got %s", rr.Body.String())
	}

	// Unmatched /api route should return 404 JSON
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("Unmatched /api route expected 404, got %d", rr.Code)
	}
	var errResp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp["error"] == nil {
		t.Fatalf("expected error object in JSON response, got %+v", errResp)
	}
}
