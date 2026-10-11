CREATE SEQUENCE IF NOT EXISTS obs_seq;

CREATE TABLE IF NOT EXISTS runs (
  run_id TEXT PRIMARY KEY,
  type TEXT NOT NULL CHECK (type IN ('filesystem','tls')),
  trigger TEXT NOT NULL CHECK (trigger IN ('manual','scheduled')),
  status TEXT NOT NULL CHECK (status IN ('queued','running','completed','partial','failed')),
  params JSON,
  requested_at TIMESTAMPTZ NOT NULL, started_at TIMESTAMPTZ, finished_at TIMESTAMPTZ,
  duration_ms BIGINT, records_total BIGINT DEFAULT 0, records_unique BIGINT DEFAULT 0,
  error_count INT DEFAULT 0, error_summary TEXT,
  agent_version TEXT, capture_method TEXT, capture_note TEXT,
  privileged BOOLEAN, is_sample BOOLEAN DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS run_logs (
  run_id TEXT, seq INT, ts TIMESTAMPTZ, level TEXT, msg TEXT,
  PRIMARY KEY (run_id, seq)
);

CREATE TABLE IF NOT EXISTS ingest_batches (
  run_id TEXT, seq INT, received_at TIMESTAMPTZ,
  PRIMARY KEY (run_id, seq)
);

CREATE TABLE IF NOT EXISTS certificates (
  fingerprint_sha256 TEXT PRIMARY KEY,
  serial TEXT, subject_dn TEXT, subject_cn TEXT, issuer_dn TEXT, issuer_cn TEXT,
  san JSON, not_before TIMESTAMPTZ, not_after TIMESTAMPTZ,
  pubkey_algo TEXT, pubkey_bits INT, curve TEXT, sig_algo TEXT,
  is_ca BOOLEAN, is_self_signed BOOLEAN,
  pqc_class TEXT,
  first_seen TIMESTAMPTZ, last_seen TIMESTAMPTZ, seen_count BIGINT DEFAULT 1
);

CREATE TABLE IF NOT EXISTS cert_observations (
  obs_id BIGINT PRIMARY KEY DEFAULT nextval('obs_seq'),
  run_id TEXT NOT NULL,
  fingerprint_sha256 TEXT,
  path TEXT NOT NULL, resolved_path TEXT, filename TEXT, format TEXT, scan_root TEXT,
  chain_position INT NOT NULL,
  chain_length INT, file_mode TEXT, file_size BIGINT,
  is_symlink BOOLEAN, contains_private_key BOOLEAN,
  parse_status TEXT NOT NULL,
  parse_error TEXT,
  days_to_expiry INT, validity_status TEXT,
  owner_pid INT, owner_proc TEXT, owner_unit TEXT, owner_confidence TEXT,
  discovered_at TIMESTAMPTZ,
  UNIQUE (run_id, path, chain_position)
);
CREATE INDEX IF NOT EXISTS idx_certobs_run ON cert_observations(run_id);
CREATE INDEX IF NOT EXISTS idx_certobs_fp  ON cert_observations(fingerprint_sha256);

CREATE TABLE IF NOT EXISTS tls_observations (
  obs_id BIGINT PRIMARY KEY DEFAULT nextval('obs_seq'),
  run_id TEXT NOT NULL, conn_key TEXT NOT NULL,
  first_ts TIMESTAMPTZ, last_ts TIMESTAMPTZ,
  direction TEXT, transport TEXT DEFAULT 'tcp',
  local_ip TEXT, local_port INT, remote_ip TEXT, remote_port INT,
  sni TEXT, tls_version TEXT, cipher_suite TEXT, kex_group TEXT, pqc_class TEXT,
  outcome TEXT,
  peer_cert_fp TEXT, peer_cert_source TEXT,
  pid INT, exe TEXT, comm TEXT, unit TEXT, attribution TEXT,
  record_count INT, handshake_count INT, appdata_count INT, alert_count INT,
  handshake_seq TEXT,
  UNIQUE (run_id, conn_key)
);
CREATE INDEX IF NOT EXISTS idx_tlsobs_run ON tls_observations(run_id);

CREATE TABLE IF NOT EXISTS nodes (
  node_id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  label TEXT, props JSON, first_seen TIMESTAMPTZ, last_seen TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS edges (
  edge_id TEXT PRIMARY KEY,
  src TEXT NOT NULL, dst TEXT NOT NULL,
  type TEXT NOT NULL,
  weight BIGINT DEFAULT 0,
  first_seen TIMESTAMPTZ, last_seen TIMESTAMPTZ, last_run_id TEXT, confidence TEXT
);

CREATE TABLE IF NOT EXISTS edge_run_stats (
  edge_id TEXT, run_id TEXT, weight BIGINT,
  PRIMARY KEY (edge_id, run_id)
);

CREATE TABLE IF NOT EXISTS schedules (
  id TEXT PRIMARY KEY, type TEXT, interval_seconds INT, enabled BOOLEAN,
  params JSON, last_run_id TEXT, next_run_at TIMESTAMPTZ
);
