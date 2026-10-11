package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServer_AuthAndRoutes(t *testing.T) {
	srv := NewServer("127.0.0.1:0", "secret-token", nil)
	handler := srv.Routes()

	// 1. Health without token -> 401
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}

	// 2. Health with token -> 200
	req = httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var health HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&health); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if health.Version == "" {
		t.Errorf("expected version, got empty")
	}

	// 3. Scan TLS -> 501
	req = httptest.NewRequest(http.MethodPost, "/v1/scan/tls", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rec.Code)
	}

	// 4. Scan FS -> 202
	body, _ := json.Marshal(ScanFSRequest{RunID: "run-99", CallbackBase: "http://127.0.0.1:8080"})
	req = httptest.NewRequest(http.MethodPost, "/v1/scan/fs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}
}
