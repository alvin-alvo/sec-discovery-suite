# 03 Database Schema (DuckDB)

File: `data/sds.duckdb`. Migrations in `api/migrations/` embedded and applied at startup
(track in `schema_migrations`).

## Rules (DuckDB-specific)
1. **No FOREIGN KEYs on tables that are upserted.** DuckDB implements updates as
   delete+insert, so FKs cause spurious constraint failures. Enforce integrity in the
   ingest layer.
2. **One statement must not touch the same conflict key twice** (ON CONFLICT DO UPDATE
   cannot update the same row twice in one command). Aggregate duplicates in Go
   (e.g. sum edge weights per edge_id) BEFORE executing a batch's upserts.
3. Use single writer connection for ingest; wrap each batch in one transaction.
4. IDs: runs use ULIDs; edges/nodes use deterministic keys (below).

## 001_init.sql
```sql
CREATE SEQUENCE IF NOT EXISTS obs_seq;

CREATE TABLE runs (
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

CREATE TABLE run_logs (
  run_id TEXT, seq INT, ts TIMESTAMPTZ, level TEXT, msg TEXT,
  PRIMARY KEY (run_id, seq)
);

CREATE TABLE ingest_batches (          -- idempotent agent retries
  run_id TEXT, seq INT, received_at TIMESTAMPTZ,
  PRIMARY KEY (run_id, seq)
);

-- Global identity: one row per distinct certificate (files AND network)
CREATE TABLE certificates (
  fingerprint_sha256 TEXT PRIMARY KEY,
  serial TEXT, subject_dn TEXT, subject_cn TEXT, issuer_dn TEXT, issuer_cn TEXT,
  san JSON, not_before TIMESTAMPTZ, not_after TIMESTAMPTZ,
  pubkey_algo TEXT, pubkey_bits INT, curve TEXT, sig_algo TEXT,
  is_ca BOOLEAN, is_self_signed BOOLEAN,
  pqc_class TEXT,                       -- classical | hybrid | pqc | unknown
  first_seen TIMESTAMPTZ, last_seen TIMESTAMPTZ, seen_count BIGINT DEFAULT 1
);

-- One row per certificate slot per file per run (a bundle file => many rows)
CREATE TABLE cert_observations (
  obs_id BIGINT PRIMARY KEY DEFAULT nextval('obs_seq'),
  run_id TEXT NOT NULL,
  fingerprint_sha256 TEXT,              -- NULL when parse failed
  path TEXT NOT NULL, resolved_path TEXT, filename TEXT, format TEXT, scan_root TEXT,
  chain_position INT NOT NULL,          -- 0 = first/leaf; -1 = unparseable file
  chain_length INT, file_mode TEXT, file_size BIGINT,
  is_symlink BOOLEAN, contains_private_key BOOLEAN,
  parse_status TEXT NOT NULL,           -- ok | parse_failed | access_denied | password_protected
  parse_error TEXT,
  days_to_expiry INT, validity_status TEXT,   -- snapshot AT SCAN TIME
  owner_pid INT, owner_proc TEXT, owner_unit TEXT, owner_confidence TEXT,
  discovered_at TIMESTAMPTZ,
  UNIQUE (run_id, path, chain_position)
);
CREATE INDEX idx_certobs_run ON cert_observations(run_id);
CREATE INDEX idx_certobs_fp  ON cert_observations(fingerprint_sha256);

-- One row per TCP connection per run. THIS TABLE IS THE EVIDENCE for report totals.
CREATE TABLE tls_observations (
  obs_id BIGINT PRIMARY KEY DEFAULT nextval('obs_seq'),
  run_id TEXT NOT NULL, conn_key TEXT NOT NULL,   -- hash(5-tuple + first_ts)
  first_ts TIMESTAMPTZ, last_ts TIMESTAMPTZ,
  direction TEXT, transport TEXT DEFAULT 'tcp',
  local_ip TEXT, local_port INT, remote_ip TEXT, remote_port INT,
  sni TEXT, tls_version TEXT, cipher_suite TEXT, kex_group TEXT, pqc_class TEXT,
  outcome TEXT,                                   -- established | failed | alert | incomplete
  peer_cert_fp TEXT, peer_cert_source TEXT,       -- observed | probed | unavailable
  pid INT, exe TEXT, comm TEXT, unit TEXT, attribution TEXT,  -- ebpf | proc_fd | unavailable
  record_count INT, handshake_count INT, appdata_count INT, alert_count INT,
  handshake_seq TEXT,                             -- e.g. 'CH,SH,CERT,FIN'
  UNIQUE (run_id, conn_key)
);
CREATE INDEX idx_tlsobs_run ON tls_observations(run_id);

-- Relationship graph
CREATE TABLE nodes (
  node_id TEXT PRIMARY KEY,
  type TEXT NOT NULL,                  -- host | process | certificate | endpoint
  label TEXT, props JSON, first_seen TIMESTAMPTZ, last_seen TIMESTAMPTZ
);
CREATE TABLE edges (
  edge_id TEXT PRIMARY KEY,            -- hash(src|type|dst): THE dedup key
  src TEXT NOT NULL, dst TEXT NOT NULL,
  type TEXT NOT NULL,                  -- RUNS | CONNECTS_TO | PRESENTS | USES | STORES
  weight BIGINT DEFAULT 0,             -- lifetime observations
  first_seen TIMESTAMPTZ, last_seen TIMESTAMPTZ, last_run_id TEXT, confidence TEXT
);
CREATE TABLE edge_run_stats (          -- makes each run's graph reproducible
  edge_id TEXT, run_id TEXT, weight BIGINT,
  PRIMARY KEY (edge_id, run_id)
);

CREATE TABLE schedules (
  id TEXT PRIMARY KEY, type TEXT, interval_seconds INT, enabled BOOLEAN,
  params JSON, last_run_id TEXT, next_run_at TIMESTAMPTZ
);
```

## Deduplication logic
**Certificates (global):** fingerprint = SHA-256(DER).
```sql
INSERT INTO certificates (...) VALUES (...)
ON CONFLICT (fingerprint_sha256) DO UPDATE SET
  last_seen = excluded.last_seen,
  seen_count = certificates.seen_count + 1;
```
Report metrics per run: found = `COUNT(*)` of cert_observations with parse_status='ok';
unique = `COUNT(DISTINCT fingerprint_sha256)`; duplicates = found - unique.
Parse-failed and access-denied are counted separately.

**Observations:** `UNIQUE(run_id, path, chain_position)` => re-ingesting a batch cannot
duplicate rows. `ingest_batches(run_id, seq)` makes retries a no-op.

**Nodes** (deterministic IDs):
- `host:<machine-id hash>`
- `proc:<sha1(exe path)[:12]>`   (NEVER keyed by PID: PIDs are recycled)
- `cert:<fingerprint_sha256>`
- `ep:<ip>:<port>`

**Edges** (deterministic): `edge_id = sha1(src|type|dst)`.
```sql
INSERT INTO edges (edge_id, src, dst, type, weight, first_seen, last_seen, last_run_id, confidence)
VALUES (...)
ON CONFLICT (edge_id) DO UPDATE SET
  weight = edges.weight + excluded.weight,
  last_seen = excluded.last_seen, last_run_id = excluded.last_run_id;

INSERT INTO edge_run_stats VALUES (?, ?, ?)
ON CONFLICT (edge_id, run_id) DO UPDATE SET weight = edge_run_stats.weight + excluded.weight;
```
Repeated observations STRENGTHEN an existing edge; they never create copies.

**Edge types:**
- `host -RUNS-> process`
- `process -CONNECTS_TO-> endpoint` (inverted for inbound: `endpoint -CONNECTS_TO-> process`)
- `endpoint -PRESENTS-> certificate`
- `process -USES-> certificate` (from filesystem owner discovery or TLS server side)
- `host -STORES-> certificate` (file location)

**FS/TLS linkage:** peer certificates are ALSO upserted into `certificates`. A cert
stored at `/etc/nginx/server.crt` and presented by `:443` is ONE node with both a
`STORES` and a `PRESENTS` edge.

## Report total derivations (must be explainable via /tls/evidence)
- TLS events = `SUM(record_count)` over the run's tls_observations.
- Unique remote endpoints = `COUNT(DISTINCT remote_ip, remote_port)`.
- Certificates observed = `COUNT(DISTINCT peer_cert_fp)` where not null.
- Mapped relationships = `COUNT(*)` of edge_run_stats for the run.
