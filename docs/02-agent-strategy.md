# 02 Agent Strategy (Go)

## 1. Binary size (hard limit 5 MB; internal budget 4.5 MB)
```bash
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o bin/sds-agent ./cmd/sds-agent
upx --best --lzma bin/sds-agent      # release step; README reports BOTH sizes
```
- `scripts/check-size.sh` fails if the packed binary exceeds 4,500,000 bytes.
- Dependency allowlist: stdlib, `golang.org/x/sys/unix`, `software.sslmate.com/src/go-pkcs12`.
- Banned: gopacket, cobra, viper, zap, logrus, grpc, protobuf. Use `flag` and `log/slog`.
- `cilium/ebpf` is allowed ONLY behind build tag `ebpf`; the default build excludes it.
  If the eBPF build exceeds 5 MB after UPX, it is not the submitted artifact.
- Find bloat: `go tool nm -size -sort size bin/sds-agent | head -30`.
- UPX caveats to document: ~100 ms startup cost, some AV engines flag packed binaries.

## 2. Interfaces
CLI (same functions as the HTTP server):
```
sds-agent scan fs  [--roots a,b,c] [--json]
sds-agent scan tls [--duration 30s] [--json]
sds-agent serve    [--addr 127.0.0.1:7471]
```
Command API (bearer token `SDS_TOKEN`): `GET /v1/health`, `POST /v1/scan/fs`,
`POST /v1/scan/tls` (see docs/04). Agent streams batches to the backend ingest API.

## 3. TLS sniffer: three-tier degradation
```go
type Sniffer interface {
    Run(ctx context.Context, emit func(Event)) error
    Mode() string // "ebpf+af_packet" | "af_packet" | "procfs+probe"
}
```
Selection at scan start; every failed tier logs its reason, and the chosen tier plus
fallback reason are stored on the run (`capture_method`, `capture_note`).

1. **Tier 1 eBPF + AF_PACKET** (build tag `ebpf`, STRETCH). Requires
   `/sys/kernel/btf/vmlinux`, kernel >= 5.8, `CAP_BPF`+`CAP_PERFMON`. A CO-RE tracepoint
   on `sock:inet_sock_set_state` emits `(4-tuple, pid, comm)` via ring buffer. TLS bytes
   still come from AF_PACKET. Any load/attach error falls to tier 2.
   Optional: socket filter on the AF_PACKET socket passing only TCP payloads starting
   `16 03`, truncating frames (kernel-enforced metadata-only).
2. **Tier 2 AF_PACKET** (default). `socket(AF_PACKET, SOCK_RAW, htons(ETH_P_ALL))`,
   needs `CAP_NET_RAW`. Hand-parse Ethernet, IPv4/IPv6, TCP. No gopacket.
3. **Tier 3 procfs + probe.** If raw sockets are denied: poll `/proc/net/tcp{,6}` for
   established connections and fill TLS metadata via the active probe. Run status is
   `partial` with the reason recorded.

## 4. TLS parsing rules
- Handshake messages span TCP segments: use a bounded per-flow reassembly buffer
  (~16 KB/flow, capped flow table with TTL/LRU).
- **Version**: read the `supported_versions` extension in ServerHello (legacy field says
  1.2 for TLS 1.3). Detect HelloRetryRequest by its fixed random value.
- **Cipher suite**: plaintext in ServerHello (TLS 1.2 and 1.3).
- **SNI**: ClientHello extension 0. If absent/ECH, record unavailable.
- **Key exchange group**: ServerHello `key_share`. Hybrid PQC groups (e.g.
  X25519MLKEM768 = 0x11EC) => `pqc_class = hybrid`.
- **Peer certificate**: TLS 1.2 => parse Certificate message, SHA-256 of leaf DER,
  source `observed`. TLS 1.3 (encrypted) => ACTIVE PROBE: agent performs its own
  `tls.Dial` to the observed endpoint with the observed SNI, reads only the leaf
  certificate, discards the connection, source `probed`. Else `unavailable`.
- **Direction**: ClientHello sender is the client; if the client IP is a local interface
  address => `outbound`, else `inbound`.
- **Loopback**: frames may arrive twice on `lo`; filter on `sll_pkttype`
  (skip PACKET_OUTGOING) and VERIFY empirically.
- **Event definition**: one event = one TLS record header observed (any content type).
  Store only type and length, never application-data bytes. Per-connection counters
  (handshake, appdata, alert) explain every report total.
- **Limitations (README)**: QUIC/HTTP3 not visible, ECH hides SNI, TLS 1.3 certs need
  the probe, containers/other network namespaces out of scope.

## 5. Mapping connections to PIDs
1. Parse `/proc/net/tcp` and `/proc/net/tcp6`; match the 4-tuple to the socket **inode**.
   Gotcha: IPv4 addresses are little-endian hex words.
2. Build `inode -> pid` by walking `/proc/*/fd/*` and reading `socket:[inode]` symlinks.
   Refresh on cache miss only.
3. Enrich: `/proc/PID/exe` (readlink), `comm`, `cmdline`, `/proc/PID/cgroup` (systemd unit).
4. Record `attribution` = `ebpf` | `proc_fd` | `unavailable`. Never guess.
5. Short-lived connections that close before step 2 are `unavailable` (tier 1 closes this gap).
6. Reading other users' `/proc/*/fd` needs `CAP_SYS_PTRACE`.

## 6. Filesystem scan rules
- Default roots: `/etc/ssl`, `/usr/share/ca-certificates`, `/etc/pki`, `/etc/letsencrypt`,
  `/etc/nginx`, `/etc/apache2`, `/opt`. Roots configurable per request.
- Skip `/proc`, `/sys`, `/dev`, `/run`. Max file size ~1 MB.
- Detect by extension AND magic: PEM (`-----BEGIN`), DER (`0x30 0x82`), PFX/P12.
  Formats: PEM, DER, CRT, CER, PFX, P12.
- **Fingerprint** = SHA-256 of the certificate DER bytes.
- Evidence per cert: path, filename, format, discovery time, scan root; serial, subject,
  issuer, SAN, chain position; key algorithm+size, signature algorithm, not_before/after,
  days_to_expiry, status (valid | expiring (<=30 days) | expired); owner + confidence.
- **Private keys**: a `PRIVATE KEY` PEM block is NEVER retained or logged. Record only
  `contains_private_key=true` and the file mode. For PFX/P12 try an empty password; if
  required => `password_protected`. README must disclose that parsing an unprotected PFX
  touches key material transiently in memory (discarded, never logged/stored/sent).
- **Symlinks**: record each with `is_symlink` and `resolved_path`. Counts: found =
  observations, unique = distinct fingerprints, duplicates = found - unique.
- **Errors**: unreadable files/dirs produce `access_denied` rows and count toward the
  run's error summary (=> `partial`). Never silently skip.
- **Owner discovery** with confidence: `high` = process holds the file open
  (`/proc/*/fd`); `medium` = path appears in a process cmdline; `low` = path under a
  known service config dir (e.g. /etc/nginx). Otherwise unavailable.
- **PQC classification** (`pqc_class`): RSA/ECDSA/EdDSA => `classical`; ML-DSA/SLH-DSA
  OIDs or hybrid groups => `pqc`/`hybrid`; unrecognized algorithm => `unknown` (Go's
  x509 cannot parse PQC certs yet: record as parse_failed with a clear reason).
