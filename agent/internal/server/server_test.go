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

	// 3. Scan FS with non-loopback callback_base -> 400
	bodyBad, _ := json.Marshal(ScanFSRequest{RunID: "run-99", CallbackBase: "http://attacker.com:8080"})
	req = httptest.NewRequest(http.MethodPost, "/v1/scan/fs", bytes.NewReader(bodyBad))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for external callback_base, got %d", rec.Code)
	}

	// 4. Scan FS with loopback callback_base -> 202
	body, _ := json.Marshal(ScanFSRequest{RunID: "run-99", CallbackBase: "http://127.0.0.1:8080"})
	req = httptest.NewRequest(http.MethodPost, "/v1/scan/fs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}

	// 5. Scan TLS with non-loopback callback_base -> 400
	bodyTLSBad, _ := json.Marshal(ScanTLSRequest{RunID: "run-tls-1", CallbackBase: "http://10.0.0.5:8080"})
	req = httptest.NewRequest(http.MethodPost, "/v1/scan/tls", bytes.NewReader(bodyTLSBad))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for external callback_base, got %d", rec.Code)
	}

	// 6. Scan TLS with loopback callback_base -> 202
	bodyTLS, _ := json.Marshal(ScanTLSRequest{RunID: "run-tls-1", DurationSec: 1, CallbackBase: "http://localhost:8080"})
	req = httptest.NewRequest(http.MethodPost, "/v1/scan/tls", bytes.NewReader(bodyTLS))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202 for loopback TLS scan, got %d", rec.Code)
	}
}
