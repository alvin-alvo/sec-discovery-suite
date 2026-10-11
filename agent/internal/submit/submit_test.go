package submit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"sds/agent/internal/fsscan"
)

func TestSubmitFSResult(t *testing.T) {
	var batchCalls int32
	var completeCalls int32
	var receivedToken string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("Authorization")

		switch r.URL.Path {
		case "/api/v1/ingest/runs/run-123/batches":
			atomic.AddInt32(&batchCalls, 1)
			var env BatchEnvelope
			if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if env.Kind != "cert_observations" {
				http.Error(w, "bad kind", 422)
				return
			}
			w.WriteHeader(http.StatusOK)
		case "/api/v1/ingest/runs/run-123/complete":
			atomic.AddInt32(&completeCalls, 1)
			var comp CompletePayload
			if err := json.NewDecoder(r.Body).Decode(&comp); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if comp.StatusHint != "completed" {
				http.Error(w, "bad status hint", 400)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "run-123", "secret-test-token")

	// Create 1200 dummy observations to test >2 batches (BatchSize=500 -> 3 batches)
	var obs []*fsscan.CertObservation
	for i := 0; i < 1200; i++ {
		obs = append(obs, &fsscan.CertObservation{
			Path:        "/etc/ssl/cert.pem",
			ParseStatus: "ok",
		})
	}

	res := &fsscan.ScanResult{
		Observations: obs,
		Errors:       nil,
	}

	started := time.Now().Add(-10 * time.Second)
	finished := time.Now()

	err := client.SubmitFSResult(res, started, finished, []LogEntry{{TS: time.Now().UTC().Format(time.RFC3339), Level: "info", Msg: "scan done"}})
	if err != nil {
		t.Fatalf("SubmitFSResult failed: %v", err)
	}

	expectedBatches := int32((len(obs) + BatchSize - 1) / BatchSize)
	if atomic.LoadInt32(&batchCalls) != expectedBatches {
		t.Errorf("expected %d batches for 1200 records, got %d", expectedBatches, atomic.LoadInt32(&batchCalls))
	}
	if atomic.LoadInt32(&completeCalls) != 1 {
		t.Errorf("expected 1 complete call, got %d", atomic.LoadInt32(&completeCalls))
	}
	if receivedToken != "Bearer secret-test-token" {
		t.Errorf("expected Bearer secret-test-token, got %s", receivedToken)
	}
}
