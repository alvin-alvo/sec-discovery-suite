package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"sds/agent/internal/fsscan"
	"sds/agent/internal/submit"
	"sds/agent/internal/tlsscan"
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

type ScanTLSRequest struct {
	RunID        string `json:"run_id"`
	DurationSec  int    `json:"duration_sec"`
	CallbackBase string `json:"callback_base"`
}

type HealthResponse struct {
	Version     string   `json:"version"`
	UID         int      `json:"uid"`
	Privileged  bool     `json:"privileged"`
	Tiers       []string `json:"tiers"`
	BinaryBytes int64    `json:"binary_bytes"`
}

func isAllowedCallback(callbackBase string) bool {
	if callbackBase == "" {
		return true
	}
	u, err := url.Parse(callbackBase)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "127.0.0.1" || h == "::1" || h == "localhost"
}

func NewServer(addr string, token string, logger *slog.Logger) *Server {
	if token == "" {
		token = os.Getenv("SDS_TOKEN")
	}
	if token == "" {
		for _, p := range []string{".sds_token", "data/.sds_token"} {
			if b, err := os.ReadFile(p); err == nil {
				t := strings.TrimSpace(string(b))
				if t != "" {
					token = t
					break
				}
			}
		}
	} else {
		if _, err := os.Stat("data"); err == nil {
			_ = os.WriteFile("data/.sds_token", []byte(token), 0644)
		}
		if _, err := os.Stat("bin"); err == nil {
			_ = os.WriteFile(".sds_token", []byte(token), 0644)
		}
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

	if !isAllowedCallback(req.CallbackBase) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"code":    "invalid_request",
				"message": "callback_base must be on a loopback host (127.0.0.1, ::1, or localhost)",
			},
		})
		return
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
	var req ScanTLSRequest
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

	if req.DurationSec <= 0 {
		req.DurationSec = 30
	} else if req.DurationSec > 120 {
		req.DurationSec = 120
	}

	if req.CallbackBase == "" {
		req.CallbackBase = "http://127.0.0.1:8080"
	}

	if !isAllowedCallback(req.CallbackBase) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"code":    "invalid_request",
				"message": "callback_base must be on a loopback host (127.0.0.1, ::1, or localhost)",
			},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "accepted",
		"run_id": req.RunID,
	})

	// Run TLS scan and submission asynchronously
	go func() {
		startedAt := time.Now()
		s.Logger.Info("starting tls scan", "run_id", req.RunID, "duration_sec", req.DurationSec)

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.DurationSec)*time.Second)
		defer cancel()

		var observations []*tlsscan.TLSObservation
		var obsMu sync.Mutex

		sniffer := tlsscan.NewAFPacketSniffer()
		err := sniffer.Run(ctx, func(obs *tlsscan.TLSObservation) {
			obsMu.Lock()
			observations = append(observations, obs)
			obsMu.Unlock()
		})

		finishedAt := time.Now()

		var scanErrors []*fsscan.ScanError
		var logs []submit.LogEntry

		if err != nil {
			s.Logger.Error("tls sniffer error", "run_id", req.RunID, "err", err)
			msg := err.Error()
			code := "raw_socket_denied"
			scanErrors = append(scanErrors, &fsscan.ScanError{
				Code:    code,
				Message: msg,
			})
			logs = append(logs, submit.LogEntry{
				TS:    time.Now().UTC().Format(time.RFC3339),
				Level: "error",
				Msg:   fmt.Sprintf("sniffer failed: %v", err),
			})
		}

		// Active Probe (runs AFTER capture ends)
		probedEndpoints := make(map[string]bool)
		probeCount := 0

		for _, obs := range observations {
			if obs.PeerCertFP == nil && obs.Outcome != nil && *obs.Outcome == "established" && obs.RemoteIP != nil && obs.RemotePort > 0 {
				epKey := fmt.Sprintf("%s:%d", *obs.RemoteIP, obs.RemotePort)
				if probedEndpoints[epKey] || probeCount >= 20 {
					continue
				}
				probedEndpoints[epKey] = true
				probeCount++

				sni := ""
				if obs.SNI != nil {
					sni = *obs.SNI
				}

				leafCert, fp, probeErr := tlsscan.ProbeEndpoint(*obs.RemoteIP, obs.RemotePort, sni)
				if probeErr == nil && fp != nil {
					src := "probed"
					obs.PeerCertFP = fp
					obs.PeerCertSource = &src
					obs.PeerCertificate = leafCert
					s.Logger.Info("active probe resolved tls 1.3 cert", "endpoint", epKey, "fp", *fp)
				}
			}
		}

		logs = append(logs, submit.LogEntry{
			TS:    time.Now().UTC().Format(time.RFC3339),
			Level: "info",
			Msg:   fmt.Sprintf("tls scan completed with %d observations (probed %d)", len(observations), probeCount),
		})

		client := submit.NewClient(req.CallbackBase, req.RunID, s.Token)
		if err := client.SubmitTLSResult(observations, startedAt, finishedAt, sniffer.Mode(), nil, scanErrors, logs); err != nil {
			s.Logger.Error("failed to submit tls scan results", "run_id", req.RunID, "err", err)
		}
	}()
}

func (s *Server) ListenAndServe() error {
	s.httpServer = &http.Server{
		Addr:    s.Addr,
		Handler: s.Routes(),
	}
	s.Logger.Info("agent command server listening", "addr", s.Addr)
	return s.httpServer.ListenAndServe()
}
