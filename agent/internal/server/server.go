package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"sds/agent/internal/fsscan"
	"sds/agent/internal/submit"
)

type Server struct {
	Addr       string
	Token      string
	Logger     *slog.Logger
	httpServer *http.Server
}

type ScanFSRequest struct {
	RunID        string   `json:"run_id"`
	Roots        []string `json:"roots"`
	CallbackBase string   `json:"callback_base"`
}

type HealthResponse struct {
	Version     string   `json:"version"`
	UID         int      `json:"uid"`
	Privileged  bool     `json:"privileged"`
	Tiers       []string `json:"tiers"`
	BinaryBytes int64    `json:"binary_bytes"`
}

func NewServer(addr string, token string, logger *slog.Logger) *Server {
	if token == "" {
		token = os.Getenv("SDS_TOKEN")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		Addr:   addr,
		Token:  token,
		Logger: logger,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.authMiddleware(s.handleHealth))
	mux.HandleFunc("POST /v1/scan/fs", s.authMiddleware(s.handleScanFS))
	mux.HandleFunc("POST /v1/scan/tls", s.authMiddleware(s.handleScanTLS))
	return mux
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Token != "" {
			authHeader := r.Header.Get("Authorization")
			expected := "Bearer " + s.Token
			if authHeader != expected {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{
						"code":    "unauthorized",
						"message": "missing or invalid authorization bearer token",
					},
				})
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	var binBytes int64
	exe, err := os.Executable()
	if err == nil {
		fi, err := os.Stat(exe)
		if err == nil {
			binBytes = fi.Size()
		}
	}

	tiers := []string{"af_packet", "procfs+probe"}
	resp := HealthResponse{
		Version:     submit.Version,
		UID:         os.Getuid(),
		Privileged:  os.Getuid() == 0,
		Tiers:       tiers,
		BinaryBytes: binBytes,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleScanFS(w http.ResponseWriter, r *http.Request) {
	var req ScanFSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"code":    "invalid_request",
				"message": fmt.Sprintf("invalid JSON body: %v", err),
			},
		})
		return
	}

	if req.RunID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"code":    "invalid_request",
				"message": "run_id is required",
			},
		})
		return
	}

	if req.CallbackBase == "" {
		req.CallbackBase = "http://127.0.0.1:8080"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "accepted",
		"run_id": req.RunID,
	})

	// Run scan and submission asynchronously
	go func() {
		startedAt := time.Now()
		s.Logger.Info("starting fs scan", "run_id", req.RunID, "roots", req.Roots)

		res, err := fsscan.Scan(req.Roots, startedAt)
		finishedAt := time.Now()

		var logs []submit.LogEntry
		if err != nil {
			s.Logger.Error("fs scan failed", "run_id", req.RunID, "err", err)
			logs = append(logs, submit.LogEntry{
				TS:    time.Now().UTC().Format(time.RFC3339),
				Level: "error",
				Msg:   fmt.Sprintf("scan failed: %v", err),
			})
			res = &fsscan.ScanResult{
				Observations: nil,
				Errors: []*fsscan.ScanError{{
					Code:    "walk_error",
					Message: err.Error(),
					Path:    nil,
				}},
			}
		} else {
			s.Logger.Info("fs scan complete", "run_id", req.RunID, "observations", len(res.Observations), "errors", len(res.Errors))
			logs = append(logs, submit.LogEntry{
				TS:    time.Now().UTC().Format(time.RFC3339),
				Level: "info",
				Msg:   fmt.Sprintf("scan finished with %d observations and %d errors", len(res.Observations), len(res.Errors)),
			})
		}

		client := submit.NewClient(req.CallbackBase, req.RunID, s.Token)
		if err := client.SubmitFSResult(res, startedAt, finishedAt, logs); err != nil {
			s.Logger.Error("failed to submit scan results", "run_id", req.RunID, "err", err)
		}
	}()
}

func (s *Server) handleScanTLS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":    "not_implemented",
			"message": "tls scanning is not implemented yet",
		},
	})
}

func (s *Server) ListenAndServe() error {
	s.httpServer = &http.Server{
		Addr:    s.Addr,
		Handler: s.Routes(),
	}
	s.Logger.Info("agent command server listening", "addr", s.Addr)
	return s.httpServer.ListenAndServe()
}
