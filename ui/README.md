# Cryptographic Discovery Suite — Frontend UI

The web frontend for `sec-discovery-suite` is an enterprise dark-mode Single Page Application (SPA) built with modern React 19, TypeScript, and Vite. It is embedded directly into the Go `sds-server` binary using `embed.FS`.

---

## Key Tech Stack

- **Framework:** React 19 + TypeScript
- **Build Tool:** Vite with `@tailwindcss/vite`
- **Routing:** React Router v7
- **Data Fetching:** TanStack Query (React Query) with polling while scans are active
- **Tables & Filtering:** TanStack Table
- **Charts & Visualizations:** Recharts (Donut, Stacked Bar, Area, and Timeline)
- **Relationship Graph:** React Flow (`@xyflow/react`) + Dagre layout (`@dagrejs/dagre`)
- **Icons:** Lucide React

---

## Views & Capabilities

1. **Discovery Runs (`/runs`):**
   - Trigger manual Filesystem and TLS metadata scans.
   - Configure periodic automated scan schedules (from 30s demo intervals to 24 hours).
   - Overview charts of scan durations and run statuses.
   - Agent heartbeat status indicator (`online`, `privileged`, `tiers`, `size`).

2. **Filesystem Scan Report (`/runs/:id/fs`):**
   - Metric cards: Found, Unique, Duplicates, Valid, Expiring (&le;30d), Expired, Parse Failed, Access Denied.
   - Recharts charts: Validity Posture donut, Key Sizes & Algorithms, and Top Issuers.
   - Searchable, faceted inventory table with copyable fingerprints and row inspection drawer.
   - Flags for `PRIVATE KEY`, `SYMLINK`, and `WORLD-READ`.

3. **TLS Metadata Scan Report (`/runs/:id/tls`):**
   - Metric cards: Total Events (record headers), Connections, Unique Endpoints, Certs Observed, Mapped Relationships.
   - Capture method chip (e.g. `af_packet`).
   - "How is this total computed?" mathematical proof expander backed by `/tls/evidence`.
   - Endpoint-grouped flows with expandable connection rows, cipher suites, PQC badges, and process attribution.
   - Integrated Cryptographic Relationship Map.

4. **Cryptographic Relationship Map:**
   - Visualizes `Host` &rarr; `Process` &rarr; `Endpoint` &rarr; `Certificate` relationships.
   - Toggle between **This Run** and **Lifetime** global graph views.
   - Logarithmic edge thickness indicating observation frequency.
   - Expired or classical quantum-vulnerable certificates highlighted with risk rings.
   - Click-to-inspect node detail side drawer.

---

## Local Development

```bash
# From repository root:
make ui-dev

# Or directly in ui directory:
cd ui
npm install
npm run dev
```

During development, Vite proxies all `/api/*` requests to `http://127.0.0.1:8080`.

---

## Production Build

```bash
npm run build
# Compiles assets into ../api/internal/web/dist
```

Running `make server` automatically builds the UI and embeds the distribution bundle into `bin/sds-server`.

---

## License & Copyright

Copyright (C) 2026 Alvin Alvo ([https://github.com/alvin-alvo/sec-discovery-suite](https://github.com/alvin-alvo/sec-discovery-suite)). All rights reserved.

Licensed under GNU General Public License v3.0 (GPLv3).

