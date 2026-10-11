package http

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"sds/api/internal/ingest"
	"sds/api/internal/runs"
	"sds/api/internal/store"
	"sds/api/internal/web"
)

type ServerConfig struct {
	Store        *store.Store
	Dispatcher   *runs.Dispatcher
	Token        string
	AgentAddr    string
	CallbackBase string
}

func NewRouter(cfg ServerConfig) http.Handler {
	mux := http.NewServeMux()

	// Helpers
	writeError := func(w http.ResponseWriter, status int, code, message string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"code":    code,
				"message": message,
			},
		})
	}

	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}

	checkAuth := func(r *http.Request) bool {
		if cfg.Token == "" {
			return true
		}
		auth := r.Header.Get("Authorization")
		expected := "Bearer " + cfg.Token
		return subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) == 1
	}

	// 1. Health
	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
		})
	}
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /api/v1/health", healthHandler)

	// 2. Agent status (proxy agent /v1/health, offline-safe)
	agentStatusHandler := func(w http.ResponseWriter, r *http.Request) {
		targetURL := cfg.AgentAddr + "/v1/health"
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"online": false,
				"status": "offline",
				"error":  err.Error(),
			})
			return
		}
		if cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
		}

		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"online": false,
				"status": "offline",
				"error":  err.Error(),
			})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			writeJSON(w, http.StatusOK, map[string]any{
				"online": false,
				"status": "offline",
				"error":  "agent returned HTTP " + strconv.Itoa(resp.StatusCode),
			})
			return
		}

		var agentResp map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&agentResp); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"online": false,
				"status": "offline",
				"error":  "failed to decode agent health response",
			})
			return
		}

		agentResp["online"] = true
		writeJSON(w, http.StatusOK, agentResp)
	}
	mux.HandleFunc("GET /agent/status", agentStatusHandler)
	mux.HandleFunc("GET /api/v1/agent/status", agentStatusHandler)

	// 3. Runs API
	// POST /runs
	startRunHandler := func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Type   string          `json:"type"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "malformed request body: "+err.Error())
			return
		}

		run, err := cfg.Dispatcher.CreateAndDispatchRun(r.Context(), req.Type, "manual", req.Params)
		if err != nil {
			if errors.Is(err, runs.ErrInvalidRunType) {
				writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]string{
			"run_id": run.RunID,
			"status": run.Status,
		})
	}
	mux.HandleFunc("POST /api/v1/runs", startRunHandler)
	mux.HandleFunc("POST /api/runs", startRunHandler)

	// GET /runs
	listRunsHandler := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		runType := q.Get("type")
		status := q.Get("status")
		cursor := q.Get("cursor")
		limit := 50
		if lStr := q.Get("limit"); lStr != "" {
			if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
				limit = l
			}
		}

		runsList, nextCursor, err := cfg.Store.ListRuns(r.Context(), runType, status, cursor, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"runs":        runsList,
			"next_cursor": nextCursor,
		})
	}
	mux.HandleFunc("GET /api/v1/runs", listRunsHandler)
	mux.HandleFunc("GET /api/runs", listRunsHandler)

	// GET /runs/{id}
	getRunHandler := func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")
		run, err := cfg.Store.GetRun(r.Context(), runID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		if run == nil {
			writeError(w, http.StatusNotFound, "not_found", "run not found")
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
	mux.HandleFunc("GET /api/v1/runs/{id}", getRunHandler)
	mux.HandleFunc("GET /api/runs/{id}", getRunHandler)

	// GET /runs/{id}/logs
	getRunLogsHandler := func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")
		logs, err := cfg.Store.GetRunLogs(r.Context(), runID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"logs": logs,
		})
	}
	mux.HandleFunc("GET /api/v1/runs/{id}/logs", getRunLogsHandler)
	mux.HandleFunc("GET /api/runs/{id}/logs", getRunLogsHandler)

	// GET /runs/{id}/fs/summary
	getFSSummaryHandler := func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")
		summary, err := cfg.Store.GetFSSummary(r.Context(), runID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, summary)
	}
	mux.HandleFunc("GET /api/v1/runs/{id}/fs/summary", getFSSummaryHandler)
	mux.HandleFunc("GET /api/runs/{id}/fs/summary", getFSSummaryHandler)

	// GET /runs/{id}/fs/certificates & GET /fs/certificates
	listCertificatesHandler := func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")
		if runID == "" {
			runID = r.URL.Query().Get("run_id")
		}

		q := r.URL.Query()
		queryStr := q.Get("q")
		status := q.Get("status")
		issuer := q.Get("issuer")
		algo := q.Get("algo")
		format := q.Get("format")
		parseStatus := q.Get("parse_status")
		sort := q.Get("sort")
		limit := 50
		if lStr := q.Get("limit"); lStr != "" {
			if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
				limit = l
			}
		}
		offset := 0
		if oStr := q.Get("offset"); oStr != "" {
			if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
				offset = o
			}
		}

		items, total, err := cfg.Store.ListFSCertificates(r.Context(), runID, queryStr, status, issuer, algo, format, parseStatus, sort, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"total":  total,
			"limit":  limit,
			"offset": offset,
			"items":  items,
		})
	}
	mux.HandleFunc("GET /api/v1/runs/{id}/fs/certificates", listCertificatesHandler)
	mux.HandleFunc("GET /api/runs/{id}/fs/certificates", listCertificatesHandler)
	mux.HandleFunc("GET /api/v1/fs/certificates", listCertificatesHandler)
	mux.HandleFunc("GET /api/fs/certificates", listCertificatesHandler)

	// GET /runs/{id}/fs/facets & GET /fs/facets
	getFacetsHandler := func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")
		if runID == "" {
			runID = r.URL.Query().Get("run_id")
		}

		facets, err := cfg.Store.GetFSFacets(r.Context(), runID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, facets)
	}
	mux.HandleFunc("GET /api/v1/runs/{id}/fs/facets", getFacetsHandler)
	mux.HandleFunc("GET /api/runs/{id}/fs/facets", getFacetsHandler)
	mux.HandleFunc("GET /api/v1/fs/facets", getFacetsHandler)
	mux.HandleFunc("GET /api/fs/facets", getFacetsHandler)

	// GET /runs/{id}/graph
	getRunGraphHandler := func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")
		graph, err := cfg.Store.GetRunGraph(r.Context(), runID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, graph)
	}
	mux.HandleFunc("GET /api/v1/runs/{id}/graph", getRunGraphHandler)
	mux.HandleFunc("GET /api/runs/{id}/graph", getRunGraphHandler)

	// GET /graph?scope=global
	getGraphHandler := func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("scope")
		if scope == "run" {
			runID := r.URL.Query().Get("run_id")
			graph, err := cfg.Store.GetRunGraph(r.Context(), runID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, graph)
			return
		}

		graph, err := cfg.Store.GetGlobalGraph(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, graph)
	}
	mux.HandleFunc("GET /api/v1/graph", getGraphHandler)
	mux.HandleFunc("GET /api/graph", getGraphHandler)

	// GET /dashboard/summary
	getDashboardSummaryHandler := func(w http.ResponseWriter, r *http.Request) {
		summary, err := cfg.Store.GetDashboardSummary(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, summary)
	}
	mux.HandleFunc("GET /api/v1/dashboard/summary", getDashboardSummaryHandler)
	mux.HandleFunc("GET /api/dashboard/summary", getDashboardSummaryHandler)

	// Schedules API
	getSchedulesHandler := func(w http.ResponseWriter, r *http.Request) {
		schedules, err := cfg.Store.GetSchedules(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schedules": schedules,
		})
	}
	mux.HandleFunc("GET /api/v1/schedules", getSchedulesHandler)
	mux.HandleFunc("GET /api/schedules", getSchedulesHandler)

	putSchedulesHandler := func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID              string          `json:"id"`
			IntervalSeconds int             `json:"interval_seconds"`
			Enabled         bool            `json:"enabled"`
			Params          json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "malformed schedule body: "+err.Error())
			return
		}

		if req.ID == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "schedule id is required")
			return
		}

		err := cfg.Store.UpdateSchedule(r.Context(), store.Schedule{
			ID:              req.ID,
			IntervalSeconds: req.IntervalSeconds,
			Enabled:         req.Enabled,
			Params:          req.Params,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
		})
	}
	mux.HandleFunc("PUT /api/v1/schedules", putSchedulesHandler)
	mux.HandleFunc("PUT /api/schedules", putSchedulesHandler)

	// Ingest Batches
	ingestBatchesHandler := func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized: invalid or missing bearer token")
			return
		}

		runID := r.PathValue("id")
		var env ingest.BatchEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "failed to parse batch envelope: "+err.Error())
			return
		}

		alreadyProcessed, err := ingest.ProcessBatch(r.Context(), cfg.Store, runID, env)
		if err != nil {
			if errors.Is(err, ingest.ErrRunNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "run not found")
				return
			}
			if errors.Is(err, ingest.ErrWrongRunType) {
				writeError(w, http.StatusUnprocessableEntity, "wrong_run_type", err.Error())
				return
			}
			if errors.Is(err, ingest.ErrInvalidFingerprint) || errors.Is(err, ingest.ErrInvalidPort) || errors.Is(err, ingest.ErrInvalidRequest) {
				writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			slog.Error("Failed to process ingest batch", "run_id", runID, "seq", env.Seq, "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":            "ok",
			"already_processed": alreadyProcessed,
		})
	}
	mux.HandleFunc("POST /api/v1/ingest/runs/{id}/batches", ingestBatchesHandler)
	mux.HandleFunc("POST /api/ingest/runs/{id}/batches", ingestBatchesHandler)

	// Ingest Complete
	ingestCompleteHandler := func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized: invalid or missing bearer token")
			return
		}

		runID := r.PathValue("id")
		var req ingest.CompleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "failed to parse complete request: "+err.Error())
			return
		}

		finalRun, err := ingest.CompleteRun(r.Context(), cfg.Store, runID, req)
		if err != nil {
			if errors.Is(err, ingest.ErrRunNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "run not found")
				return
			}
			slog.Error("Failed to complete run", "run_id", runID, "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":       "ok",
			"run_id":       finalRun.RunID,
			"final_status": finalRun.Status,
			"run":          finalRun,
		})
	}
	mux.HandleFunc("POST /api/v1/ingest/runs/{id}/complete", ingestCompleteHandler)
	mux.HandleFunc("POST /api/ingest/runs/{id}/complete", ingestCompleteHandler)

	// TLS Report endpoints per docs/07
	getTLSSummaryHandler := func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")
		summary, err := cfg.Store.GetTLSSummary(r.Context(), runID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, summary)
	}
	mux.HandleFunc("GET /api/v1/runs/{id}/tls/summary", getTLSSummaryHandler)
	mux.HandleFunc("GET /api/runs/{id}/tls/summary", getTLSSummaryHandler)

	getTLSObservationsHandler := func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")
		q := r.URL.Query()
		group := q.Get("group")
		queryStr := q.Get("q")
		version := q.Get("version")
		cipher := q.Get("cipher")
		direction := q.Get("direction")
		attribution := q.Get("attribution")
		limit := 50
		if lStr := q.Get("limit"); lStr != "" {
			if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
				limit = l
			}
		}
		offset := 0
		if oStr := q.Get("offset"); oStr != "" {
			if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
				offset = o
			}
		}

		items, total, err := cfg.Store.ListTLSObservations(r.Context(), runID, group, queryStr, version, cipher, direction, attribution, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}

		if group == "endpoint" {
			writeJSON(w, http.StatusOK, map[string]any{
				"items": items,
				"total": total,
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"items":  items,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	}
	mux.HandleFunc("GET /api/v1/runs/{id}/tls/observations", getTLSObservationsHandler)
	mux.HandleFunc("GET /api/runs/{id}/tls/observations", getTLSObservationsHandler)

	getTLSEvidenceHandler := func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")
		evidence, err := cfg.Store.GetTLSEvidence(r.Context(), runID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, evidence)
	}
	mux.HandleFunc("GET /api/v1/runs/{id}/tls/evidence", getTLSEvidenceHandler)
	mux.HandleFunc("GET /api/runs/{id}/tls/evidence", getTLSEvidenceHandler)

	// Sample data endpoint
	sampleLoadHandler := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"message": "sample data endpoint ready",
		})
	}
	mux.HandleFunc("POST /api/v1/sample/load", sampleLoadHandler)
	mux.HandleFunc("POST /api/sample/load", sampleLoadHandler)

	// Traffic Generation endpoint
	generateTrafficHandler := func(w http.ResponseWriter, r *http.Request) {
		duration := 30
		var req struct {
			DurationSec int `json:"duration_sec"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.DurationSec > 0 {
				duration = req.DurationSec
			}
		}

		go func() {
			cmd := exec.Command("./scripts/gen-traffic.sh", strconv.Itoa(duration))
			_ = cmd.Run()
		}()

		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":       "started",
			"duration_sec": duration,
			"message":      "Generating simulated TLS 1.2 and TLS 1.3 traffic for observation",
		})
	}
	mux.HandleFunc("POST /api/v1/traffic/generate", generateTrafficHandler)
	mux.HandleFunc("POST /api/traffic/generate", generateTrafficHandler)

	// Root handler: SPA with index.html fallback for non-/api paths
	subFS, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		slog.Error("Failed to create sub filesystem for web dist", "err", err)
	}

	indexHTML, err := web.DistFS.ReadFile("dist/index.html")
	if err != nil {
		indexHTML = []byte("<!doctype html><title>UI</title>")
	}

	fileServer := http.FileServer(http.FS(subFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// If route starts with /api/, return 404 JSON error
		if strings.HasPrefix(path, "/api/") {
			writeError(w, http.StatusNotFound, "not_found", "API endpoint not found")
			return
		}

		// Try to see if file exists in subFS
		relPath := strings.TrimPrefix(path, "/")
		if relPath != "" && subFS != nil {
			f, err := subFS.Open(relPath)
			if err == nil {
				stat, err := f.Stat()
				f.Close()
				if err == nil && !stat.IsDir() {
					fileServer.ServeHTTP(w, r)
					return
				}
			}
		}

		// Fallback to index.html for SPA routes (e.g. /runs, /runs/123/fs)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexHTML)
	})

	return mux
}
