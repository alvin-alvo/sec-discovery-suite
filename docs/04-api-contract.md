# 04 API Contract

Backend `:8080`, prefix `/api/v1`, JSON. UI is served at `/`. Agent->backend ingest
endpoints require `Authorization: Bearer $SDS_TOKEN`.

## Errors
`{"error":{"code":"invalid_request","message":"..."}}` with 400/404/409/422/500.

## Endpoints
| Method & path | Purpose |
|---|---|
| `GET /health` | Backend health |
| `GET /agent/status` | Agent heartbeat: version, uid, privileged, capture tiers available, binary size |
| `POST /runs` | Body `{type:"filesystem"|"tls", params:{roots?, duration_sec?}}` => `202 {run_id, status:"queued"}` |
| `GET /runs?type=&status=&limit=&cursor=` | Discovery Runs table |
| `GET /runs/{id}` | Run detail |
| `GET /runs/{id}/logs` | Per-run logs |
| `GET /runs/{id}/fs/summary` | found, unique, duplicates, valid, expiring, expired, parse_failed, algorithms, key sizes, sig algos, issuers, locations |
| `GET /runs/{id}/fs/certificates?q=&status=&issuer=&algo=&format=&parse_status=&sort=&limit=&offset=` | Inventory table |
| `GET /runs/{id}/fs/facets` | Distinct filter values |
| `GET /runs/{id}/tls/summary` | events, unique endpoints/ports, versions, ciphers, certs observed, relationships, time buckets |
| `GET /runs/{id}/tls/observations?group=none|endpoint&...` | Evidence table |
| `GET /runs/{id}/tls/evidence` | Arithmetic behind each total |
| `GET /runs/{id}/graph` | Graph for one run |
| `GET /graph?scope=global` | Lifetime graph |
| `GET /dashboard/summary` | Latest run per type + trends |
| `GET /schedules`, `PUT /schedules` | Periodic scan config |
| `POST /ingest/runs/{id}/batches` | Agent => backend batch |
| `POST /ingest/runs/{id}/complete` | Agent => backend final status |
| `POST /sample/load` | Load flagged sample data (`is_sample=true`) |

## Agent command API (127.0.0.1:7471, bearer token)
- `GET /v1/health` => `{version, uid, privileged, tiers:[...], binary_bytes}`
- `POST /v1/scan/fs`  `{run_id, roots?, callback_url}` => `202`
- `POST /v1/scan/tls` `{run_id, duration_sec, callback_url}` => `202`

## Core flow
1. UI `POST /runs`. Backend inserts `queued` row, POSTs the command to the agent,
   sets `running` once the agent acknowledges (agent unreachable => `failed` with message).
2. Agent streams batches to `/ingest/runs/{id}/batches`, then `/complete`.
3. UI polls `GET /runs/{id}` every 1 s while status is queued/running.
4. Scheduler goroutine ticks over `schedules`, creates runs with `trigger=scheduled`;
   skips a tick if a run of the same type is still active.
5. `/complete` computes final status: completed | partial | failed (rules in docs/01).

## Shapes
Run:
```json
{"run_id":"01J...","type":"tls","trigger":"manual","status":"partial",
 "requested_at":"...","started_at":"...","finished_at":"...","duration_ms":31200,
 "records_total":412,"records_unique":9,"error_count":1,
 "error_summary":"raw socket denied; fell back to procfs+probe",
 "capture_method":"procfs+probe","capture_note":"operation not permitted","is_sample":false}
```
Ingest batch and Complete shapes: see docs/06-wire-format.md (supercedes batch shapes here).

Graph:
```json
{"nodes":[{"id":"proc:ab12","type":"process","label":"nginx","props":{}}],
 "edges":[{"id":"e1","src":"proc:ab12","dst":"ep:10.0.0.5:443","type":"CONNECTS_TO","weight":14}]}
```
Validation: reject unknown run_id (404), wrong run type for batch kind (422), malformed
fingerprints (must be 64 hex), out-of-range ports. Ingest is atomic per batch.
