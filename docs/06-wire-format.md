# 06 Wire Format (agent <-> backend). SUPERSEDES the batch shapes in docs/04.

Conventions: timestamps RFC3339 UTC; fingerprints = 64 lowercase hex; ports are ints;
unknown values are JSON null (never empty strings). Agent auth: `Authorization: Bearer $SDS_TOKEN`.

## Agent command (backend -> agent)
`POST /v1/scan/fs`  {"run_id","roots":[...]|null,"callback_base":"http://127.0.0.1:8080"}
`POST /v1/scan/tls` {"run_id","duration_sec":30,"callback_base":"..."}
Agent replies 202 immediately, then posts to
`{callback_base}/api/v1/ingest/runs/{run_id}/batches` and `.../complete`.

## Batch envelope (agent -> backend)
{"seq":1,"kind":"cert_observations"|"tls_observations",
 "host":{"machine_id_hash":"<sha256 of /etc/machine-id, hex>","hostname":"vm1"},
 "records":[...]}   // max 5000 records / 10 MB per batch; seq starts at 1; retry reuses seq

## cert_observations record
{"path":"/etc/ssl/certs/a.pem","resolved_path":"/usr/share/ca-certificates/x/a.crt",
 "filename":"a.pem","format":"PEM|DER|PKCS12","scan_root":"/etc/ssl",
 "chain_position":0,"chain_length":1,"file_mode":"0644","file_size":1234,
 "is_symlink":true,"contains_private_key":false,
 "parse_status":"ok|parse_failed|access_denied|password_protected","parse_error":null,
 "days_to_expiry":123,"validity_status":"valid|expiring|expired",   // computed by AGENT at scan time; expiring = <=30 days
 "owner":{"pid":1234,"proc":"/usr/sbin/nginx","unit":"nginx.service","confidence":"high|medium|low"} | null,
 "discovered_at":"...",
 "certificate":{"fingerprint_sha256":"..","serial":"..","subject_dn":"..","subject_cn":"..",
   "issuer_dn":"..","issuer_cn":"..","san":["dns:a.com","ip:1.2.3.4"],
   "not_before":"..","not_after":"..","pubkey_algo":"RSA|ECDSA|Ed25519|unknown",
   "pubkey_bits":2048,"curve":null,"sig_algo":"SHA256-RSA","is_ca":false,
   "is_self_signed":true,"pqc_class":"classical|hybrid|pqc|unknown"} | null   // null unless parse_status="ok"
}
Unparseable/denied files: chain_position=-1, certificate=null, days_to_expiry=null.

## tls_observations record
{"conn_key":"<sha1 hex of 5-tuple+first_ts>","first_ts":"..","last_ts":"..",
 "direction":"inbound|outbound","transport":"tcp",
 "local_ip":"..","local_port":54321,"remote_ip":"..","remote_port":443,
 "sni":null,"tls_version":"TLS1.2|TLS1.3|null","cipher_suite":"TLS_AES_128_GCM_SHA256|null",
 "kex_group":"x25519|null","pqc_class":"classical|hybrid|unknown",
 "outcome":"established|failed|alert|incomplete",
 "peer_cert_source":"observed|probed|unavailable",
 "process":{"pid":1,"exe":"/usr/bin/curl","comm":"curl","unit":null,
            "attribution":"ebpf|proc_fd|unavailable"} | null,
 "record_count":12,"handshake_count":4,"appdata_count":7,"alert_count":0,
 "handshake_seq":"CH,SH,CERT,FIN",
 "peer_certificate":{...same object as `certificate` above...} | null}

## Complete
POST .../complete
{"status_hint":"completed|partial|failed","started_at":"..","finished_at":"..",
 "agent":{"version":"..","uid":0,"privileged":true},
 "capture_method":"af_packet|ebpf+af_packet|procfs+probe|null","capture_note":null,
 "errors":[{"code":"access_denied|walk_error|sniffer_fallback|attribution_unavailable",
            "message":"..","path":"/root"|null}],
 "logs":[{"ts":"..","level":"info|warn|error","msg":".."}]}

## Server-side rules
- Final status: status_hint=failed => failed; else any error with a code above => partial; else completed.
  (parse_failed files are findings, NOT errors.)
- records_total/records_unique and duration are RECOMPUTED by the server from DB/timestamps.
- The SERVER derives nodes/edges from records (agents never send graph data):
  FS:  host -STORES-> cert (1 per observation); process -USES-> cert when owner present.
  TLS: host -RUNS-> process; outbound: process -CONNECTS_TO-> ep:<remote>; inbound: ep:<remote> -CONNECTS_TO-> process;
       presenter endpoint = remote ep (outbound) or LOCAL ep (inbound): presenter -PRESENTS-> cert when peer cert known;
       inbound with peer cert also adds process -USES-> cert. Edge weight = 1 per connection (aggregate in Go before upsert).
- Validation: 404 unknown run; 422 batch kind != run type; 400 malformed fingerprint/port; atomic per batch;
  repeated (run_id, seq) returns 200 and changes nothing.
