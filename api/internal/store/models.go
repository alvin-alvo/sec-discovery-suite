package store

import (
	"encoding/json"
	"time"
)

type Run struct {
	RunID         string          `json:"run_id"`
	Type          string          `json:"type"`
	Trigger       string          `json:"trigger"`
	Status        string          `json:"status"`
	Params        json.RawMessage `json:"params,omitempty"`
	RequestedAt   time.Time       `json:"requested_at"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	FinishedAt    *time.Time      `json:"finished_at,omitempty"`
	DurationMs    *int64          `json:"duration_ms,omitempty"`
	RecordsTotal  int64           `json:"records_total"`
	RecordsUnique int64           `json:"records_unique"`
	ErrorCount    int             `json:"error_count"`
	ErrorSummary  *string         `json:"error_summary,omitempty"`
	AgentVersion  *string         `json:"agent_version,omitempty"`
	CaptureMethod *string         `json:"capture_method,omitempty"`
	CaptureNote   *string         `json:"capture_note,omitempty"`
	Privileged    *bool           `json:"privileged,omitempty"`
	IsSample      bool            `json:"is_sample"`
}

type RunLog struct {
	RunID string    `json:"run_id"`
	Seq   int       `json:"seq"`
	Ts    time.Time `json:"ts"`
	Level string    `json:"level"`
	Msg   string    `json:"msg"`
}

type Schedule struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	IntervalSeconds int             `json:"interval_seconds"`
	Enabled         bool            `json:"enabled"`
	Params          json.RawMessage `json:"params,omitempty"`
	LastRunID       *string         `json:"last_run_id,omitempty"`
	NextRunAt       *time.Time      `json:"next_run_at,omitempty"`
}

type Certificate struct {
	FingerprintSHA256 string          `json:"fingerprint_sha256"`
	Serial            *string         `json:"serial,omitempty"`
	SubjectDN         *string         `json:"subject_dn,omitempty"`
	SubjectCN         *string         `json:"subject_cn,omitempty"`
	IssuerDN          *string         `json:"issuer_dn,omitempty"`
	IssuerCN          *string         `json:"issuer_cn,omitempty"`
	SAN               json.RawMessage `json:"san,omitempty"`
	NotBefore         *time.Time      `json:"not_before,omitempty"`
	NotAfter          *time.Time      `json:"not_after,omitempty"`
	PubkeyAlgo        *string         `json:"pubkey_algo,omitempty"`
	PubkeyBits        *int            `json:"pubkey_bits,omitempty"`
	Curve             *string         `json:"curve,omitempty"`
	SigAlgo           *string         `json:"sig_algo,omitempty"`
	IsCA              *bool           `json:"is_ca,omitempty"`
	IsSelfSigned      *bool           `json:"is_self_signed,omitempty"`
	PQCClass          *string         `json:"pqc_class,omitempty"`
	FirstSeen         *time.Time      `json:"first_seen,omitempty"`
	LastSeen          *time.Time      `json:"last_seen,omitempty"`
	SeenCount         int64           `json:"seen_count"`
}

type CertObservationItem struct {
	ObsID              int64           `json:"obs_id"`
	RunID              string          `json:"run_id"`
	FingerprintSHA256  *string         `json:"fingerprint_sha256,omitempty"`
	Path               string          `json:"path"`
	ResolvedPath       *string         `json:"resolved_path,omitempty"`
	Filename           *string         `json:"filename,omitempty"`
	Format             *string         `json:"format,omitempty"`
	ScanRoot           *string         `json:"scan_root,omitempty"`
	ChainPosition      int             `json:"chain_position"`
	ChainLength        *int            `json:"chain_length,omitempty"`
	FileMode           *string         `json:"file_mode,omitempty"`
	FileSize           *int64          `json:"file_size,omitempty"`
	IsSymlink          *bool           `json:"is_symlink,omitempty"`
	ContainsPrivateKey *bool           `json:"contains_private_key,omitempty"`
	ParseStatus        string          `json:"parse_status"`
	ParseError         *string         `json:"parse_error,omitempty"`
	DaysToExpiry       *int            `json:"days_to_expiry,omitempty"`
	ValidityStatus     *string         `json:"validity_status,omitempty"`
	OwnerPID           *int            `json:"owner_pid,omitempty"`
	OwnerProc          *string         `json:"owner_proc,omitempty"`
	OwnerUnit          *string         `json:"owner_unit,omitempty"`
	OwnerConfidence    *string         `json:"owner_confidence,omitempty"`
	DiscoveredAt       *time.Time      `json:"discovered_at,omitempty"`
	// Joined Certificate fields
	SubjectDN    *string         `json:"subject_dn,omitempty"`
	SubjectCN    *string         `json:"subject_cn,omitempty"`
	IssuerDN     *string         `json:"issuer_dn,omitempty"`
	IssuerCN     *string         `json:"issuer_cn,omitempty"`
	SAN          json.RawMessage `json:"san,omitempty"`
	NotBefore    *time.Time      `json:"not_before,omitempty"`
	NotAfter     *time.Time      `json:"not_after,omitempty"`
	PubkeyAlgo   *string         `json:"pubkey_algo,omitempty"`
	PubkeyBits   *int            `json:"pubkey_bits,omitempty"`
	Curve        *string         `json:"curve,omitempty"`
	SigAlgo      *string         `json:"sig_algo,omitempty"`
	IsCA         *bool           `json:"is_ca,omitempty"`
	IsSelfSigned *bool           `json:"is_self_signed,omitempty"`
	PQCClass     *string         `json:"pqc_class,omitempty"`
}

type NamedCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type FSSummary struct {
	Found        int64        `json:"found"`
	Unique       int64        `json:"unique"`
	Duplicates   int64        `json:"duplicates"`
	Valid        int64        `json:"valid"`
	Expiring     int64        `json:"expiring"`
	Expired      int64        `json:"expired"`
	ParseFailed  int64        `json:"parse_failed"`
	AccessDenied int64        `json:"access_denied"`
	Algorithms   []NamedCount `json:"algorithms"`
	KeySizes     []NamedCount `json:"key_sizes"`
	SigAlgos     []NamedCount `json:"sig_algos"`
	TopIssuers   []NamedCount `json:"top_issuers"`
	Locations    []NamedCount `json:"locations"`
}

type FSFacets struct {
	ValidityStatus []string `json:"validity_status"`
	Issuers        []string `json:"issuers"`
	Algorithms     []string `json:"algorithms"`
	Formats        []string `json:"formats"`
	ParseStatus    []string `json:"parse_status"`
}

type GraphNode struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Label     string          `json:"label"`
	Props     json.RawMessage `json:"props"`
	FirstSeen *time.Time      `json:"first_seen,omitempty"`
	LastSeen  *time.Time      `json:"last_seen,omitempty"`
}

type GraphEdge struct {
	ID         string     `json:"id"`
	Src        string     `json:"src"`
	Dst        string     `json:"dst"`
	Type       string     `json:"type"`
	Weight     int64      `json:"weight"`
	FirstSeen  *time.Time `json:"first_seen,omitempty"`
	LastSeen   *time.Time `json:"last_seen,omitempty"`
	Confidence *string    `json:"confidence,omitempty"`
}

type GraphResponse struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type DashboardSummary struct {
	LatestFSRun   *Run  `json:"latest_fs_run"`
	LatestTLSRun  *Run  `json:"latest_tls_run"`
	TotalCerts    int64 `json:"total_certs"`
	ActiveRuns    int64 `json:"active_runs"`
	ExpiringCerts int64 `json:"expiring_certs"`
}

type TimelineBucket struct {
	TS     string `json:"ts"`
	Events int64  `json:"events"`
}

type TLSSummary struct {
	Events          int64            `json:"events"`
	Connections     int64            `json:"connections"`
	UniqueEndpoints int64            `json:"unique_endpoints"`
	UniquePorts     int64            `json:"unique_ports"`
	CertsObserved   int64            `json:"certs_observed"`
	Relationships   int64            `json:"relationships"`
	Versions        []NamedCount     `json:"versions"`
	Ciphers         []NamedCount     `json:"ciphers"`
	KexGroups       []NamedCount     `json:"kex_groups"`
	Directions      []NamedCount     `json:"directions"`
	PeerCertSources []NamedCount     `json:"peer_cert_sources"`
	Attribution     []NamedCount     `json:"attribution"`
	TopEndpoints    []NamedCount     `json:"top_endpoints"`
	CertPosture     []NamedCount     `json:"cert_posture"`
	Timeline        []TimelineBucket `json:"timeline"`
}

type TLSObservationItem struct {
	ObsID          int64      `json:"obs_id"`
	RunID          string     `json:"run_id"`
	ConnKey        string     `json:"conn_key"`
	FirstTS        *time.Time `json:"first_ts"`
	LastTS         *time.Time `json:"last_ts"`
	Direction      *string    `json:"direction"`
	Transport      *string    `json:"transport"`
	LocalIP        *string    `json:"local_ip"`
	LocalPort      int        `json:"local_port"`
	RemoteIP       *string    `json:"remote_ip"`
	RemotePort     int        `json:"remote_port"`
	SNI            *string    `json:"sni"`
	TLSVersion     *string    `json:"tls_version"`
	CipherSuite    *string    `json:"cipher_suite"`
	KexGroup       *string    `json:"kex_group"`
	PQCClass       *string    `json:"pqc_class"`
	Outcome        *string    `json:"outcome"`
	PeerCertFP     *string    `json:"peer_cert_fp"`
	PeerCertSource *string    `json:"peer_cert_source"`
	PID            *int       `json:"pid"`
	Exe            *string    `json:"exe"`
	Comm           *string    `json:"comm"`
	Unit           *string    `json:"unit"`
	Attribution    *string    `json:"attribution"`
	RecordCount    int        `json:"record_count"`
	HandshakeCount int        `json:"handshake_count"`
	AppdataCount   int        `json:"appdata_count"`
	AlertCount     int        `json:"alert_count"`
	HandshakeSeq   *string    `json:"handshake_seq"`
	PeerSubjectCN  *string    `json:"peer_subject_cn,omitempty"`
	PeerIssuerCN   *string    `json:"peer_issuer_cn,omitempty"`
	PeerNotAfter   *time.Time `json:"peer_not_after,omitempty"`
	PeerPubkeyAlgo *string    `json:"peer_pubkey_algo,omitempty"`
	PeerPubkeyBits *int       `json:"peer_pubkey_bits,omitempty"`
	PeerValidity   *string    `json:"peer_validity,omitempty"`
}

type TLSEndpointGroupItem struct {
	RemoteIP    string   `json:"remote_ip"`
	RemotePort  int      `json:"remote_port"`
	SNI         *string  `json:"sni"`
	Connections int64    `json:"connections"`
	Events      int64    `json:"events"`
	Versions    []string `json:"versions"`
	Ciphers     []string `json:"ciphers"`
	Processes   []string `json:"processes"`
	Directions  []string `json:"directions"`
}

type TLSConnRecordCount struct {
	ConnKey     string  `json:"conn_key"`
	Remote      string  `json:"remote"`
	SNI         *string `json:"sni"`
	RecordCount int64   `json:"record_count"`
}

type TLSCertObsItem struct {
	FingerprintSHA256 string `json:"fingerprint_sha256"`
	Source            string `json:"source"`
}

type TLSEvidence struct {
	Events struct {
		Total     int64 `json:"total"`
		Formula   string `json:"formula"`
		Breakdown struct {
			Handshake int64 `json:"handshake"`
			Appdata   int64 `json:"appdata"`
			Alert     int64 `json:"alert"`
			Other     int64 `json:"other"`
		} `json:"breakdown"`
		ByConnection []TLSConnRecordCount `json:"by_connection"`
	} `json:"events"`
	UniqueEndpoints struct {
		Total   int64    `json:"total"`
		Formula string   `json:"formula"`
		Items   []string `json:"items"`
	} `json:"unique_endpoints"`
	CertsObserved struct {
		Total int64            `json:"total"`
		Items []TLSCertObsItem `json:"items"`
	} `json:"certs_observed"`
}
