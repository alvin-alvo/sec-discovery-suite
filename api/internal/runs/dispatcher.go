package runs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"sds/api/internal/store"
)

var (
	ErrInvalidRunType = errors.New("type must be filesystem or tls")
	ErrActiveRun      = errors.New("a run of this type is already active")
)

type Dispatcher struct {
	store        *store.Store
	agentAddr    string
	token        string
	callbackBase string
	httpClient   *http.Client
}

func NewDispatcher(s *store.Store, agentAddr, token, callbackBase string) *Dispatcher {
	if agentAddr == "" {
		agentAddr = "http://127.0.0.1:7471"
	}
	if !strings.HasPrefix(agentAddr, "http://") && !strings.HasPrefix(agentAddr, "https://") {
		agentAddr = "http://" + agentAddr
	}
	agentAddr = strings.TrimRight(agentAddr, "/")

	if callbackBase == "" {
		callbackBase = "http://127.0.0.1:8080"
	}
	if !strings.HasPrefix(callbackBase, "http://") && !strings.HasPrefix(callbackBase, "https://") {
		callbackBase = "http://" + callbackBase
	}
	callbackBase = strings.TrimRight(callbackBase, "/")

	return &Dispatcher{
		store:        s,
		agentAddr:    agentAddr,
		token:        token,
		callbackBase: callbackBase,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type StartRunRequest struct {
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params,omitempty"`
}

type AgentFSCommand struct {
	RunID        string   `json:"run_id"`
	Roots        []string `json:"roots"`
	CallbackBase string   `json:"callback_base"`
}

type AgentTLSCommand struct {
	RunID        string `json:"run_id"`
	DurationSec  int    `json:"duration_sec"`
	CallbackBase string `json:"callback_base"`
}

// CreateAndDispatchRun creates a queued run and triggers the agent command.
func (d *Dispatcher) CreateAndDispatchRun(ctx context.Context, runType, trigger string, params json.RawMessage) (*store.Run, error) {
	if runType != "filesystem" && runType != "tls" {
		return nil, ErrInvalidRunType
	}

	runID := NewULID()
	now := time.Now().UTC()

	run := &store.Run{
		RunID:       runID,
		Type:        runType,
		Trigger:     trigger,
		Status:      "queued",
		Params:      params,
		RequestedAt: now,
	}

	if err := d.store.CreateRun(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to create run: %w", err)
	}

	// Dispatch to agent in background
	go d.dispatch(runID, runType, params)

	return run, nil
}

func (d *Dispatcher) dispatch(runID, runType string, params json.RawMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var endpoint string
	var reqBody []byte

	if runType == "filesystem" {
		endpoint = d.agentAddr + "/v1/scan/fs"
		var p struct {
			Roots []string `json:"roots"`
		}
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		cmd := AgentFSCommand{
			RunID:        runID,
			Roots:        p.Roots,
			CallbackBase: d.callbackBase,
		}
		reqBody, _ = json.Marshal(cmd)
	} else {
		endpoint = d.agentAddr + "/v1/scan/tls"
		var p struct {
			DurationSec int `json:"duration_sec"`
		}
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		if p.DurationSec <= 0 {
			p.DurationSec = 30
		}
		cmd := AgentTLSCommand{
			RunID:        runID,
			DurationSec:  p.DurationSec,
			CallbackBase: d.callbackBase,
		}
		reqBody, _ = json.Marshal(cmd)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		d.markFailed(runID, fmt.Sprintf("Failed to construct agent request: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if d.token != "" {
		req.Header.Set("Authorization", "Bearer "+d.token)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		slog.Warn("Agent unreachable", "addr", d.agentAddr, "run_id", runID, "err", err)
		d.markFailed(runID, fmt.Sprintf("Agent unreachable at %s: %v", d.agentAddr, err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		slog.Warn("Agent rejected scan", "status", resp.StatusCode, "body", string(body))
		d.markFailed(runID, fmt.Sprintf("Agent returned HTTP %d: %s", resp.StatusCode, string(body)))
		return
	}

	// Successfully acknowledged
	now := time.Now().UTC()
	if err := d.store.UpdateRunStatus(ctx, runID, "running", &now, nil, nil, nil, nil); err != nil {
		slog.Error("Failed to update run status to running", "run_id", runID, "err", err)
	}
}

func (d *Dispatcher) markFailed(runID, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC()
	dur := int64(0)
	errCount := 1

	if err := d.store.UpdateRunStatus(ctx, runID, "failed", &now, &now, &dur, &reason, &errCount); err != nil {
		slog.Error("Failed to update run status to failed", "run_id", runID, "err", err)
	}
}
