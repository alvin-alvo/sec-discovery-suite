package ingest

import (
	"encoding/json"
	"time"
)

type HostInfo struct {
	MachineIDHash string `json:"machine_id_hash"`
	Hostname      string `json:"hostname"`
}

type OwnerInfo struct {
	PID        *int   `json:"pid"`
	Proc       string `json:"proc"`
	Unit       string `json:"unit"`
	Confidence string `json:"confidence"`
}

type CertificateData struct {
	FingerprintSHA256 string          `json:"fingerprint_sha256"`
	Serial            *string         `json:"serial"`
	SubjectDN         *string         `json:"subject_dn"`
	SubjectCN         *string         `json:"subject_cn"`
	IssuerDN          *string         `json:"issuer_dn"`
	IssuerCN          *string         `json:"issuer_cn"`
	SAN               []string        `json:"san"`
	NotBefore         *time.Time      `json:"not_before"`
	NotAfter          *time.Time      `json:"not_after"`
	PubkeyAlgo        *string         `json:"pubkey_algo"`
	PubkeyBits        *int            `json:"pubkey_bits"`
	Curve             *string         `json:"curve"`
	SigAlgo           *string         `json:"sig_algo"`
	IsCA              *bool           `json:"is_ca"`
	IsSelfSigned      *bool           `json:"is_self_signed"`
	PQCClass          *string         `json:"pqc_class"`
}

type CertObservationRecord struct {
	Path               string           `json:"path"`
	ResolvedPath       *string          `json:"resolved_path"`
	Filename           *string          `json:"filename"`
	Format             *string          `json:"format"`
	ScanRoot           *string          `json:"scan_root"`
	ChainPosition      int              `json:"chain_position"`
	ChainLength        *int             `json:"chain_length"`
	FileMode           *string          `json:"file_mode"`
	FileSize           *int64           `json:"file_size"`
	IsSymlink          bool             `json:"is_symlink"`
	ContainsPrivateKey bool             `json:"contains_private_key"`
	ParseStatus        string           `json:"parse_status"`
	ParseError         *string          `json:"parse_error"`
	DaysToExpiry       *int             `json:"days_to_expiry"`
	ValidityStatus     *string          `json:"validity_status"`
	Owner              *OwnerInfo       `json:"owner"`
	DiscoveredAt       *time.Time       `json:"discovered_at"`
	Certificate        *CertificateData `json:"certificate"`
}

type TLSObservationRecord struct {
	ConnKey         string           `json:"conn_key"`
	FirstTS         *time.Time       `json:"first_ts"`
	LastTS          *time.Time       `json:"last_ts"`
	Direction       *string          `json:"direction"`
	Transport       *string          `json:"transport"`
	LocalIP         *string          `json:"local_ip"`
	LocalPort       int              `json:"local_port"`
	RemoteIP        *string          `json:"remote_ip"`
	RemotePort      int              `json:"remote_port"`
	SNI             *string          `json:"sni"`
	TLSVersion      *string          `json:"tls_version"`
	CipherSuite     *string          `json:"cipher_suite"`
	KexGroup        *string          `json:"kex_group"`
	PQCClass        *string          `json:"pqc_class"`
	Outcome         *string          `json:"outcome"`
	PeerCertSource  *string          `json:"peer_cert_source"`
	Process         *TLSProcess      `json:"process"`
	RecordCount     int              `json:"record_count"`
	HandshakeCount  int              `json:"handshake_count"`
	AppdataCount    int              `json:"appdata_count"`
	AlertCount      int              `json:"alert_count"`
	HandshakeSeq    *string          `json:"handshake_seq"`
	PeerCertificate *CertificateData `json:"peer_certificate"`
}

type TLSProcess struct {
	PID         *int    `json:"pid"`
	Exe         string  `json:"exe"`
	Comm        string  `json:"comm"`
	Unit        *string `json:"unit"`
	Attribution string  `json:"attribution"`
}

type BatchEnvelope struct {
	Seq     int             `json:"seq"`
	Kind    string          `json:"kind"`
	Host    HostInfo        `json:"host"`
	Records json.RawMessage `json:"records"`
}

type AgentMeta struct {
	Version    string `json:"version"`
	UID        int    `json:"uid"`
	Privileged bool   `json:"privileged"`
}

type ScanError struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Path    *string `json:"path"`
}

type LogItem struct {
	TS    *time.Time `json:"ts"`
	Level string     `json:"level"`
	Msg   string     `json:"msg"`
}

type CompleteRequest struct {
	StatusHint    string      `json:"status_hint"`
	StartedAt     *time.Time  `json:"started_at"`
	FinishedAt    *time.Time  `json:"finished_at"`
	Agent         AgentMeta   `json:"agent"`
	CaptureMethod *string     `json:"capture_method"`
	CaptureNote   *string     `json:"capture_note"`
	Errors        []ScanError `json:"errors"`
	Logs          []LogItem   `json:"logs"`
}
