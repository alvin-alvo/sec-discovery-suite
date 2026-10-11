package submit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"sds/agent/internal/fsscan"
)

const (
	BatchSize  = 500
	MaxRetries = 3
	Version    = "0.1.0"
)

// HostInfo represents the host object in docs/06.
type HostInfo struct {
	MachineIDHash string `json:"machine_id_hash"`
	Hostname      string `json:"hostname"`
}

// BatchEnvelope represents the batch payload sent from agent to backend.
type BatchEnvelope struct {
	Seq     int                       `json:"seq"`
	Kind    string                    `json:"kind"`
	Host    HostInfo                  `json:"host"`
	Records []*fsscan.CertObservation `json:"records"`
}

// AgentInfo represents agent metadata in /complete.
type AgentInfo struct {
	Version    string `json:"version"`
	UID        int    `json:"uid"`
	Privileged bool   `json:"privileged"`
}

// LogEntry represents a single log line in /complete.
type LogEntry struct {
	TS    string `json:"ts"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// CompletePayload represents the final status report sent to /complete.
type CompletePayload struct {
	StatusHint    string              `json:"status_hint"`
	StartedAt     string              `json:"started_at"`
	FinishedAt    string              `json:"finished_at"`
	Agent         AgentInfo           `json:"agent"`
	CaptureMethod *string             `json:"capture_method"`
	CaptureNote   *string             `json:"capture_note"`
	Errors        []*fsscan.ScanError `json:"errors"`
	Logs          []LogEntry          `json:"logs"`
}

// GetHostInfo resolves host metadata.
func GetHostInfo() HostInfo {
	hostname, _ := os.Hostname()
	machineID := readMachineID()
	sum := sha256.Sum256([]byte(machineID))
	return HostInfo{
		MachineIDHash: hex.EncodeToString(sum[:]),
		Hostname:      hostname,
	}
}

func readMachineID() string {
	for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		data, err := os.ReadFile(p)
		if err == nil {
			trimmed := strings.TrimSpace(string(data))
			if trimmed != "" {
				return trimmed
			}
		}
	}
	h, _ := os.Hostname()
	return h
}

// Client handles submitting scan batches and completion status to the backend API.
type Client struct {
	CallbackBase string
	RunID        string
	Token        string
	HTTPClient   *http.Client
}

// NewClient creates a submit client.
func NewClient(callbackBase, runID, token string) *Client {
	if token == "" {
		token = os.Getenv("SDS_TOKEN")
	}
	return &Client{
		CallbackBase: strings.TrimRight(callbackBase, "/"),
		RunID:        runID,
		Token:        token,
		HTTPClient:   &http.Client{Timeout: 15 * time.Second},
	}
}

// SubmitFSResult sends all observations in chunks of <= 500, then sends /complete.
func (c *Client) SubmitFSResult(res *fsscan.ScanResult, startedAt, finishedAt time.Time, logs []LogEntry) error {
	host := GetHostInfo()
	obs := res.Observations

	seq := 1
	for i := 0; i < len(obs); i += BatchSize {
		end := i + BatchSize
		if end > len(obs) {
			end = len(obs)
		}
		chunk := obs[i:end]

		env := BatchEnvelope{
			Seq:     seq,
			Kind:    "cert_observations",
			Host:    host,
			Records: chunk,
		}

		if err := c.sendBatchWithRetry(&env); err != nil {
			return fmt.Errorf("send batch seq %d failed: %w", seq, err)
		}
		seq++
	}

	// Determine status hint
	statusHint := "completed"
	if len(res.Errors) > 0 {
		statusHint = "partial"
	}

	completePayload := CompletePayload{
		StatusHint:    statusHint,
		StartedAt:     startedAt.UTC().Format(time.RFC3339),
		FinishedAt:    finishedAt.UTC().Format(time.RFC3339),
		Agent:         AgentInfo{Version: Version, UID: os.Getuid(), Privileged: os.Getuid() == 0},
		CaptureMethod: nil,
		CaptureNote:   nil,
		Errors:        res.Errors,
		Logs:          logs,
	}

	return c.sendCompleteWithRetry(&completePayload)
}

func (c *Client) sendBatchWithRetry(env *BatchEnvelope) error {
	url := fmt.Sprintf("%s/api/v1/ingest/runs/%s/batches", c.CallbackBase, c.RunID)
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(100*(1<<attempt)) * time.Millisecond)
		}

		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted {
			return nil
		}
		lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return lastErr
}

func (c *Client) sendCompleteWithRetry(payload *CompletePayload) error {
	url := fmt.Sprintf("%s/api/v1/ingest/runs/%s/complete", c.CallbackBase, c.RunID)
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(100*(1<<attempt)) * time.Millisecond)
		}

		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted {
			return nil
		}
		lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return lastErr
}
