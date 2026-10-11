# 05 Frontend / UI

## Setup (run from repo root once Phase 0 is done)
```bash
npm create vite@latest ui -- --template react-ts
cd ui && npm install
npm i react-router-dom @tanstack/react-query @tanstack/react-table recharts \
      @xyflow/react @dagrejs/dagre
npm i -D tailwindcss @tailwindcss/vite
```
- Vite: dev proxy `/api` => `http://localhost:8080`; `build.outDir` =
  `../api/internal/web/dist`, `emptyOutDir: true`.
- Routes (React Router, NO dynamic server routes): `/runs`, `/runs/:id/fs`, `/runs/:id/tls`.
- Data: TanStack Query; poll `GET /runs/{id}` with `refetchInterval: 1000` while
  status is queued/running. Tables: TanStack Table.
- shadcn/ui only if setup takes <= 10 minutes; otherwise plain Tailwind components.

## Look and feel
Dark-first, clean enterprise security-product style. ALL colors/spacing live in one
theme file as CSS variables so rebranding is a one-file change. No company logos or
names. Use consistent crypto-posture vocabulary and badges:
**Classical (quantum-vulnerable) / Hybrid / PQC / Unknown**. A persistent
**Sample data** banner whenever any displayed run has `is_sample=true`.

## Shared state handling (graded: 15%)
One `<AsyncState>` wrapper used by EVERY data view, rendering: loading skeleton, empty
state with a helpful message, error with Retry, and a PARTIAL banner that lists exactly
what coverage was missed (run.error_summary, capture_note). Status pills for
queued/running/completed/partial/failed used consistently everywhere.

## View 1: Discovery Runs (`/runs`)
- Buttons: **Run filesystem scan**, **Run TLS metadata scan**; new run appears
  immediately as `running` and updates by polling.
- Table: run id, type, trigger (Manual/Scheduled), start, end, status, duration, record
  counts, error summary. Row click opens `/runs/:id/fs` or `/runs/:id/tls` by type.
- Recharts: stacked bar of runs by type and status; line of duration or findings over time.
- Schedule panel: enable toggle + interval per scan type (`/schedules`).
- Agent status chip from `/agent/status` (online/offline, privileged, capture tiers).

## View 2: File System Scan Report (`/runs/:id/fs`)
- Metric cards: found, unique, duplicates, valid, expiring, expired, parse-failed.
- Recharts: validity donut; key algorithm and size bars; signature algorithms; top issuers
  (horizontal bar); certificates by location.
- Table with search + filters (status, issuer, algorithm, format, parse status). Columns:
  path, subject, issuer, validity, algorithm, key size, fingerprint (truncated, copyable),
  parsing status. Row drawer: SAN, chain position, owner process + confidence, file mode.
- Chips: `contains_private_key`, `world-readable`, `symlink`.

## View 3: TLS Metadata Scan Report (`/runs/:id/tls`)
- Metric cards: events, unique remote endpoints and ports, certs observed, relationships.
- **Capture method chip**, e.g. "AF_PACKET (eBPF unavailable: operation not permitted)".
- Recharts: TLS versions; cipher suites; top endpoints; activity over time (5 s buckets,
  area chart); certificate posture.
- "How is this total computed?" expander backed by `/tls/evidence`.
- Grouped table (by endpoint) with expandable per-connection rows: endpoint, port,
  direction, process/PID or "unavailable", protocol, cipher, peer cert (+source label
  observed/probed), event count.

## Relationship map (React Flow)
- Data from `/runs/:id/graph` with a "This run | Lifetime" toggle.
- Custom node types with distinct icon/color: Host, Process, Endpoint, Certificate.
- Auto-layout with dagre, left-to-right columns: Host -> Process -> Endpoint -> Certificate.
  Compute layout once per data change; keep nodes non-draggable-to-overlap defaults off.
- Edge width = `log(weight)`; edges seen in the current run animated; edge label = type.
- Risk styling: expired or classical-only certificates get an amber/red ring.
- Click node => side drawer listing its evidence rows (observations, fingerprints, PIDs).
- Clutter control: collapse more than ~50 endpoints into a "+N more" cluster node.
- Build the graph from a lookup keyed by node id; never rely on array order.
