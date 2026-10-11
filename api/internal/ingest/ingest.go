package ingest

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"sds/api/internal/store"
)

var (
	ErrRunNotFound        = errors.New("run not found")
	ErrWrongRunType       = errors.New("wrong run type for batch kind")
	ErrInvalidRequest     = errors.New("invalid request")
	ErrInvalidFingerprint = errors.New("malformed fingerprint_sha256")
	ErrInvalidPort        = errors.New("port out of range")
	ErrTooManyRecords     = errors.New("too many records in batch (max 5000)")
)

func sha1Hex(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func isLowerHex(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

type certDelta struct {
	cert      CertificateData
	seenCount int64
	firstSeen time.Time
	lastSeen  time.Time
}

type nodeDelta struct {
	id        string
	nodeType  string
	label     string
	props     json.RawMessage
	firstSeen time.Time
	lastSeen  time.Time
}

type edgeDelta struct {
	edgeID     string
	src        string
	dst        string
	edgeType   string
	weight     int64
	firstSeen  time.Time
	lastSeen   time.Time
	confidence string
}

// ProcessBatch ingests a batch envelope atomically per docs/06.
func ProcessBatch(ctx context.Context, s *store.Store, runID string, env BatchEnvelope) (bool, error) {
	if env.Seq < 1 {
		return false, fmt.Errorf("%w: seq must be >= 1", ErrInvalidRequest)
	}

	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return false, err
	}
	if run == nil {
		return false, ErrRunNotFound
	}

	if run.Type == "filesystem" && env.Kind != "cert_observations" {
		return false, fmt.Errorf("%w: expected cert_observations for filesystem run", ErrWrongRunType)
	}
	if run.Type == "tls" && env.Kind != "tls_observations" {
		return false, fmt.Errorf("%w: expected tls_observations for tls run", ErrWrongRunType)
	}

	var alreadyProcessed bool

	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		// 1. Check idempotency: ingest_batches(run_id, seq)
		var dummy int
		err := tx.QueryRowContext(ctx, "SELECT 1 FROM ingest_batches WHERE run_id = ? AND seq = ?", runID, env.Seq).Scan(&dummy)
		if err == nil {
			alreadyProcessed = true
			return nil
		}

		now := time.Now().UTC()

		switch env.Kind {
		case "cert_observations":
			var records []CertObservationRecord
			if err := json.Unmarshal(env.Records, &records); err != nil {
				return fmt.Errorf("%w: failed to parse records: %v", ErrInvalidRequest, err)
			}
			if len(records) > 5000 {
				return ErrTooManyRecords
			}

			// Validate fingerprints
			for _, rec := range records {
				if rec.Certificate != nil {
					fp := rec.Certificate.FingerprintSHA256
					if len(fp) != 64 || !isLowerHex(fp) {
						return fmt.Errorf("%w: %s", ErrInvalidFingerprint, fp)
					}
				}
			}

			// In-Go aggregation
			certMap := make(map[string]*certDelta)
			nodeMap := make(map[string]*nodeDelta)
			edgeMap := make(map[string]*edgeDelta)

			hostMachineHash := env.Host.MachineIDHash
			if hostMachineHash == "" {
				hostMachineHash = "unknown"
			}
			hostNodeID := "host:" + hostMachineHash
			hostLabel := env.Host.Hostname
			if hostLabel == "" {
				hostLabel = hostMachineHash
			}

			nodeMap[hostNodeID] = &nodeDelta{
				id:        hostNodeID,
				nodeType:  "host",
				label:     hostLabel,
				firstSeen: now,
				lastSeen:  now,
			}

			for _, rec := range records {
				discAt := now
				if rec.DiscoveredAt != nil {
					discAt = *rec.DiscoveredAt
				}

				if rec.Certificate != nil {
					fp := rec.Certificate.FingerprintSHA256
					if existing, ok := certMap[fp]; ok {
						existing.seenCount++
						if discAt.After(existing.lastSeen) {
							existing.lastSeen = discAt
						}
						if discAt.Before(existing.firstSeen) {
							existing.firstSeen = discAt
						}
					} else {
						certMap[fp] = &certDelta{
							cert:      *rec.Certificate,
							seenCount: 1,
							firstSeen: discAt,
							lastSeen:  discAt,
						}
					}

					certNodeID := "cert:" + fp
					certLabel := fp[:12]
					if rec.Certificate.SubjectCN != nil && *rec.Certificate.SubjectCN != "" {
						certLabel = *rec.Certificate.SubjectCN
					}
					nodeMap[certNodeID] = &nodeDelta{
						id:        certNodeID,
						nodeType:  "certificate",
						label:     certLabel,
						firstSeen: discAt,
						lastSeen:  discAt,
					}

					// host -STORES-> cert
					storesEdgeID := sha1Hex(hostNodeID + "|STORES|" + certNodeID)
					if existing, ok := edgeMap[storesEdgeID]; ok {
						existing.weight++
						if discAt.After(existing.lastSeen) {
							existing.lastSeen = discAt
						}
					} else {
						edgeMap[storesEdgeID] = &edgeDelta{
							edgeID:     storesEdgeID,
							src:        hostNodeID,
							dst:        certNodeID,
							edgeType:   "STORES",
							weight:     1,
							firstSeen:  discAt,
							lastSeen:   discAt,
							confidence: "high",
						}
					}

					// Process owner
					if rec.Owner != nil && rec.Owner.Proc != "" {
						procNodeID := "proc:" + sha1Hex(rec.Owner.Proc)[:12]
						procLabel := filepath.Base(rec.Owner.Proc)
						propsJSON, _ := json.Marshal(map[string]any{
							"pid":  rec.Owner.PID,
							"proc": rec.Owner.Proc,
							"unit": rec.Owner.Unit,
						})
						nodeMap[procNodeID] = &nodeDelta{
							id:        procNodeID,
							nodeType:  "process",
							label:     procLabel,
							props:     propsJSON,
							firstSeen: discAt,
							lastSeen:  discAt,
						}

						// host -RUNS-> proc
						runsEdgeID := sha1Hex(hostNodeID + "|RUNS|" + procNodeID)
						if existing, ok := edgeMap[runsEdgeID]; ok {
							existing.weight++
							if discAt.After(existing.lastSeen) {
								existing.lastSeen = discAt
							}
						} else {
							edgeMap[runsEdgeID] = &edgeDelta{
								edgeID:     runsEdgeID,
								src:        hostNodeID,
								dst:        procNodeID,
								edgeType:   "RUNS",
								weight:     1,
								firstSeen:  discAt,
								lastSeen:   discAt,
								confidence: "high",
							}
						}

						// proc -USES-> cert
						conf := rec.Owner.Confidence
						if conf == "" {
							conf = "medium"
						}
						usesEdgeID := sha1Hex(procNodeID + "|USES|" + certNodeID)
						if existing, ok := edgeMap[usesEdgeID]; ok {
							existing.weight++
							if discAt.After(existing.lastSeen) {
								existing.lastSeen = discAt
							}
						} else {
							edgeMap[usesEdgeID] = &edgeDelta{
								edgeID:     usesEdgeID,
								src:        procNodeID,
								dst:        certNodeID,
								edgeType:   "USES",
								weight:     1,
								firstSeen:  discAt,
								lastSeen:   discAt,
								confidence: conf,
							}
						}
					}
				}
			}

			// 2. Upsert aggregated certificates
			certStmt, err := tx.PrepareContext(ctx, `
INSERT INTO certificates (
	fingerprint_sha256, serial, subject_dn, subject_cn, issuer_dn, issuer_cn,
	san, not_before, not_after, pubkey_algo, pubkey_bits, curve, sig_algo,
	is_ca, is_self_signed, pqc_class, first_seen, last_seen, seen_count
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (fingerprint_sha256) DO UPDATE SET
	last_seen = excluded.last_seen,
	seen_count = certificates.seen_count + excluded.seen_count;`)
			if err != nil {
				return fmt.Errorf("failed to prepare cert stmt: %w", err)
			}
			defer certStmt.Close()

			for fp, cd := range certMap {
				var sanJSON *string
				if len(cd.cert.SAN) > 0 {
					b, _ := json.Marshal(cd.cert.SAN)
					s := string(b)
					sanJSON = &s
				}
				if _, err := certStmt.ExecContext(ctx,
					fp, cd.cert.Serial, cd.cert.SubjectDN, cd.cert.SubjectCN, cd.cert.IssuerDN, cd.cert.IssuerCN,
					sanJSON, cd.cert.NotBefore, cd.cert.NotAfter, cd.cert.PubkeyAlgo, cd.cert.PubkeyBits, cd.cert.Curve, cd.cert.SigAlgo,
					cd.cert.IsCA, cd.cert.IsSelfSigned, cd.cert.PQCClass, cd.firstSeen, cd.lastSeen, cd.seenCount,
				); err != nil {
					return fmt.Errorf("failed to upsert cert %s: %w", fp, err)
				}
			}

			// 3. Upsert nodes
			nodeStmt, err := tx.PrepareContext(ctx, `
INSERT INTO nodes (node_id, type, label, props, first_seen, last_seen)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (node_id) DO UPDATE SET
	label = COALESCE(excluded.label, nodes.label),
	props = COALESCE(excluded.props, nodes.props),
	last_seen = excluded.last_seen;`)
			if err != nil {
				return fmt.Errorf("failed to prepare node stmt: %w", err)
			}
			defer nodeStmt.Close()

			for _, nd := range nodeMap {
				var propsStr *string
				if len(nd.props) > 0 {
					s := string(nd.props)
					propsStr = &s
				}
				if _, err := nodeStmt.ExecContext(ctx, nd.id, nd.nodeType, nd.label, propsStr, nd.firstSeen, nd.lastSeen); err != nil {
					return fmt.Errorf("failed to upsert node %s: %w", nd.id, err)
				}
			}

			// 4. Upsert edges and edge_run_stats
			edgeStmt, err := tx.PrepareContext(ctx, `
INSERT INTO edges (edge_id, src, dst, type, weight, first_seen, last_seen, last_run_id, confidence)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (edge_id) DO UPDATE SET
	weight = edges.weight + excluded.weight,
	last_seen = excluded.last_seen,
	last_run_id = excluded.last_run_id;`)
			if err != nil {
				return fmt.Errorf("failed to prepare edge stmt: %w", err)
			}
			defer edgeStmt.Close()

			statStmt, err := tx.PrepareContext(ctx, `
INSERT INTO edge_run_stats (edge_id, run_id, weight)
VALUES (?, ?, ?)
ON CONFLICT (edge_id, run_id) DO UPDATE SET
	weight = edge_run_stats.weight + excluded.weight;`)
			if err != nil {
				return fmt.Errorf("failed to prepare edge stat stmt: %w", err)
			}
			defer statStmt.Close()

			for _, ed := range edgeMap {
				if _, err := edgeStmt.ExecContext(ctx, ed.edgeID, ed.src, ed.dst, ed.edgeType, ed.weight, ed.firstSeen, ed.lastSeen, runID, ed.confidence); err != nil {
					return fmt.Errorf("failed to upsert edge %s: %w", ed.edgeID, err)
				}
				if _, err := statStmt.ExecContext(ctx, ed.edgeID, runID, ed.weight); err != nil {
					return fmt.Errorf("failed to upsert edge run stat %s: %w", ed.edgeID, err)
				}
			}

			// 5. Insert cert_observations
			obsStmt, err := tx.PrepareContext(ctx, `
INSERT INTO cert_observations (
	run_id, fingerprint_sha256, path, resolved_path, filename, format, scan_root,
	chain_position, chain_length, file_mode, file_size, is_symlink, contains_private_key,
	parse_status, parse_error, days_to_expiry, validity_status,
	owner_pid, owner_proc, owner_unit, owner_confidence, discovered_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (run_id, path, chain_position) DO NOTHING;`)
			if err != nil {
				return fmt.Errorf("failed to prepare obs stmt: %w", err)
			}
			defer obsStmt.Close()

			for _, rec := range records {
				var fp *string
				if rec.Certificate != nil {
					fp = &rec.Certificate.FingerprintSHA256
				}
				var pid *int
				var proc, unit, conf *string
				if rec.Owner != nil {
					pid = rec.Owner.PID
					if rec.Owner.Proc != "" {
						proc = &rec.Owner.Proc
					}
					if rec.Owner.Unit != "" {
						unit = &rec.Owner.Unit
					}
					if rec.Owner.Confidence != "" {
						conf = &rec.Owner.Confidence
					}
				}
				discAt := now
				if rec.DiscoveredAt != nil {
					discAt = *rec.DiscoveredAt
				}

				if _, err := obsStmt.ExecContext(ctx,
					runID, fp, rec.Path, rec.ResolvedPath, rec.Filename, rec.Format, rec.ScanRoot,
					rec.ChainPosition, rec.ChainLength, rec.FileMode, rec.FileSize, rec.IsSymlink, rec.ContainsPrivateKey,
					rec.ParseStatus, rec.ParseError, rec.DaysToExpiry, rec.ValidityStatus,
					pid, proc, unit, conf, discAt,
				); err != nil {
					return fmt.Errorf("failed to insert cert obs (%s, %d): %w", rec.Path, rec.ChainPosition, err)
				}
			}

		case "tls_observations":
			var records []TLSObservationRecord
			if err := json.Unmarshal(env.Records, &records); err != nil {
				return fmt.Errorf("%w: failed to parse records: %v", ErrInvalidRequest, err)
			}
			if len(records) > 5000 {
				return ErrTooManyRecords
			}

			// Validate ports, fingerprints, and enums
			for _, rec := range records {
				if rec.LocalPort < 0 || rec.LocalPort > 65535 || rec.RemotePort < 0 || rec.RemotePort > 65535 {
					return ErrInvalidPort
				}
				if rec.Direction != nil && *rec.Direction != "inbound" && *rec.Direction != "outbound" {
					return fmt.Errorf("%w: invalid direction %s", ErrInvalidRequest, *rec.Direction)
				}
				if rec.Outcome != nil && *rec.Outcome != "established" && *rec.Outcome != "failed" && *rec.Outcome != "alert" && *rec.Outcome != "incomplete" {
					return fmt.Errorf("%w: invalid outcome %s", ErrInvalidRequest, *rec.Outcome)
				}
				if rec.PeerCertificate != nil {
					fp := rec.PeerCertificate.FingerprintSHA256
					if len(fp) != 64 || !isLowerHex(fp) {
						return fmt.Errorf("%w: %s", ErrInvalidFingerprint, fp)
					}
				}
			}

			// Aggregate peer certificates in Go
			certMap := make(map[string]*certDelta)
			nodeMap := make(map[string]*nodeDelta)
			edgeMap := make(map[string]*edgeDelta)

			hostMachineHash := env.Host.MachineIDHash
			if hostMachineHash == "" {
				hostMachineHash = "unknown"
			}
			hostNodeID := "host:" + hostMachineHash
			hostLabel := env.Host.Hostname
			if hostLabel == "" {
				hostLabel = hostMachineHash
			}
			nodeMap[hostNodeID] = &nodeDelta{
				id:        hostNodeID,
				nodeType:  "host",
				label:     hostLabel,
				firstSeen: now,
				lastSeen:  now,
			}

			for _, rec := range records {
				firstTS := now
				if rec.FirstTS != nil {
					firstTS = *rec.FirstTS
				}
				lastTS := now
				if rec.LastTS != nil {
					lastTS = *rec.LastTS
				}

				var certNodeID string
				if rec.PeerCertificate != nil {
					fp := rec.PeerCertificate.FingerprintSHA256
					if existing, ok := certMap[fp]; ok {
						existing.seenCount++
						if lastTS.After(existing.lastSeen) {
							existing.lastSeen = lastTS
						}
					} else {
						certMap[fp] = &certDelta{
							cert:      *rec.PeerCertificate,
							seenCount: 1,
							firstSeen: firstTS,
							lastSeen:  lastTS,
						}
					}
					certNodeID = "cert:" + fp
					certLabel := fp[:12]
					if rec.PeerCertificate.SubjectCN != nil && *rec.PeerCertificate.SubjectCN != "" {
						certLabel = *rec.PeerCertificate.SubjectCN
					}
					nodeMap[certNodeID] = &nodeDelta{
						id:        certNodeID,
						nodeType:  "certificate",
						label:     certLabel,
						firstSeen: firstTS,
						lastSeen:  lastTS,
					}
				}

				// Process node
				var procNodeID string
				if rec.Process != nil && rec.Process.Exe != "" {
					procNodeID = "proc:" + sha1Hex(rec.Process.Exe)[:12]
					procLabel := filepath.Base(rec.Process.Exe)
					propsJSON, _ := json.Marshal(map[string]any{
						"exe":  rec.Process.Exe,
						"comm": rec.Process.Comm,
						"unit": rec.Process.Unit,
					})
					nodeMap[procNodeID] = &nodeDelta{
						id:        procNodeID,
						nodeType:  "process",
						label:     procLabel,
						props:     propsJSON,
						firstSeen: firstTS,
						lastSeen:  lastTS,
					}
					// host -RUNS-> process
					hRunsP := sha1Hex(hostNodeID + "|RUNS|" + procNodeID)
					if existing, ok := edgeMap[hRunsP]; ok {
						existing.weight++
						if lastTS.After(existing.lastSeen) {
							existing.lastSeen = lastTS
						}
					} else {
						edgeMap[hRunsP] = &edgeDelta{
							edgeID:     hRunsP,
							src:        hostNodeID,
							dst:        procNodeID,
							edgeType:   "RUNS",
							weight:     1,
							firstSeen:  firstTS,
							lastSeen:   lastTS,
							confidence: "high",
						}
					}
				}

				// Endpoints
				remoteIP := "unknown"
				if rec.RemoteIP != nil {
					remoteIP = *rec.RemoteIP
				}
				remoteEP := fmt.Sprintf("ep:%s:%d", remoteIP, rec.RemotePort)
				remoteLabel := fmt.Sprintf("%s:%d", remoteIP, rec.RemotePort)
				if rec.SNI != nil && *rec.SNI != "" {
					remoteLabel = *rec.SNI
				}
				remoteProps, _ := json.Marshal(map[string]any{
					"ip":   remoteIP,
					"port": rec.RemotePort,
					"sni":  rec.SNI,
				})
				nodeMap[remoteEP] = &nodeDelta{
					id:        remoteEP,
					nodeType:  "endpoint",
					label:     remoteLabel,
					props:     remoteProps,
					firstSeen: firstTS,
					lastSeen:  lastTS,
				}

				localIP := "unknown"
				if rec.LocalIP != nil {
					localIP = *rec.LocalIP
				}
				localEP := fmt.Sprintf("ep:%s:%d", localIP, rec.LocalPort)
				localProps, _ := json.Marshal(map[string]any{
					"ip":   localIP,
					"port": rec.LocalPort,
					"sni":  nil,
				})
				nodeMap[localEP] = &nodeDelta{
					id:        localEP,
					nodeType:  "endpoint",
					label:     fmt.Sprintf("%s:%d", localIP, rec.LocalPort),
					props:     localProps,
					firstSeen: firstTS,
					lastSeen:  lastTS,
				}

				direction := "outbound"
				if rec.Direction != nil {
					direction = *rec.Direction
				}

				// Connection edge
				if procNodeID != "" {
					if direction == "outbound" {
						// proc -CONNECTS_TO-> ep:<remote>
						connEdgeID := sha1Hex(procNodeID + "|CONNECTS_TO|" + remoteEP)
						if existing, ok := edgeMap[connEdgeID]; ok {
							existing.weight++
							if lastTS.After(existing.lastSeen) {
								existing.lastSeen = lastTS
							}
						} else {
							edgeMap[connEdgeID] = &edgeDelta{
								edgeID:     connEdgeID,
								src:        procNodeID,
								dst:        remoteEP,
								edgeType:   "CONNECTS_TO",
								weight:     1,
								firstSeen:  firstTS,
								lastSeen:   lastTS,
								confidence: "high",
							}
						}
					} else {
						// ep:<remote> -CONNECTS_TO-> proc
						connEdgeID := sha1Hex(remoteEP + "|CONNECTS_TO|" + procNodeID)
						if existing, ok := edgeMap[connEdgeID]; ok {
							existing.weight++
							if lastTS.After(existing.lastSeen) {
								existing.lastSeen = lastTS
							}
						} else {
							edgeMap[connEdgeID] = &edgeDelta{
								edgeID:     connEdgeID,
								src:        remoteEP,
								dst:        procNodeID,
								edgeType:   "CONNECTS_TO",
								weight:     1,
								firstSeen:  firstTS,
								lastSeen:   lastTS,
								confidence: "high",
							}
						}
					}
				}

				// Presenter endpoint
				if certNodeID != "" {
					presenterEP := remoteEP
					if direction == "inbound" {
						presenterEP = localEP
					}
					presEdgeID := sha1Hex(presenterEP + "|PRESENTS|" + certNodeID)
					if existing, ok := edgeMap[presEdgeID]; ok {
						existing.weight++
						if lastTS.After(existing.lastSeen) {
							existing.lastSeen = lastTS
						}
					} else {
						edgeMap[presEdgeID] = &edgeDelta{
							edgeID:     presEdgeID,
							src:        presenterEP,
							dst:        certNodeID,
							edgeType:   "PRESENTS",
							weight:     1,
							firstSeen:  firstTS,
							lastSeen:   lastTS,
							confidence: "high",
						}
					}

					// inbound with peer cert also adds process -USES-> cert
					if direction == "inbound" && procNodeID != "" {
						usesEdgeID := sha1Hex(procNodeID + "|USES|" + certNodeID)
						if existing, ok := edgeMap[usesEdgeID]; ok {
							existing.weight++
							if lastTS.After(existing.lastSeen) {
								existing.lastSeen = lastTS
							}
						} else {
							edgeMap[usesEdgeID] = &edgeDelta{
								edgeID:     usesEdgeID,
								src:        procNodeID,
								dst:        certNodeID,
								edgeType:   "USES",
								weight:     1,
								firstSeen:  firstTS,
								lastSeen:   lastTS,
								confidence: "high",
							}
						}
					}
				}
			}

			// Upsert certificates if any
			if len(certMap) > 0 {
				certStmt, err := tx.PrepareContext(ctx, `
INSERT INTO certificates (
	fingerprint_sha256, serial, subject_dn, subject_cn, issuer_dn, issuer_cn,
	san, not_before, not_after, pubkey_algo, pubkey_bits, curve, sig_algo,
	is_ca, is_self_signed, pqc_class, first_seen, last_seen, seen_count
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (fingerprint_sha256) DO UPDATE SET
	last_seen = excluded.last_seen,
	seen_count = certificates.seen_count + excluded.seen_count;`)
				if err != nil {
					return fmt.Errorf("failed to prepare tls cert stmt: %w", err)
				}
				defer certStmt.Close()

				for fp, cd := range certMap {
					var sanJSON *string
					if len(cd.cert.SAN) > 0 {
						b, _ := json.Marshal(cd.cert.SAN)
						s := string(b)
						sanJSON = &s
					}
					if _, err := certStmt.ExecContext(ctx,
						fp, cd.cert.Serial, cd.cert.SubjectDN, cd.cert.SubjectCN, cd.cert.IssuerDN, cd.cert.IssuerCN,
						sanJSON, cd.cert.NotBefore, cd.cert.NotAfter, cd.cert.PubkeyAlgo, cd.cert.PubkeyBits, cd.cert.Curve, cd.cert.SigAlgo,
						cd.cert.IsCA, cd.cert.IsSelfSigned, cd.cert.PQCClass, cd.firstSeen, cd.lastSeen, cd.seenCount,
					); err != nil {
						return fmt.Errorf("failed to upsert tls cert %s: %w", fp, err)
					}
				}
			}

			// Upsert nodes
			nodeStmt, err := tx.PrepareContext(ctx, `
INSERT INTO nodes (node_id, type, label, props, first_seen, last_seen)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (node_id) DO UPDATE SET
	label = COALESCE(excluded.label, nodes.label),
	props = COALESCE(excluded.props, nodes.props),
	last_seen = excluded.last_seen;`)
			if err != nil {
				return fmt.Errorf("failed to prepare node stmt: %w", err)
			}
			defer nodeStmt.Close()

			for _, nd := range nodeMap {
				var propsStr *string
				if len(nd.props) > 0 {
					s := string(nd.props)
					propsStr = &s
				}
				if _, err := nodeStmt.ExecContext(ctx, nd.id, nd.nodeType, nd.label, propsStr, nd.firstSeen, nd.lastSeen); err != nil {
					return fmt.Errorf("failed to upsert node %s: %w", nd.id, err)
				}
			}

			// Upsert edges and stats
			edgeStmt, err := tx.PrepareContext(ctx, `
INSERT INTO edges (edge_id, src, dst, type, weight, first_seen, last_seen, last_run_id, confidence)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (edge_id) DO UPDATE SET
	weight = edges.weight + excluded.weight,
	last_seen = excluded.last_seen,
	last_run_id = excluded.last_run_id;`)
			if err != nil {
				return fmt.Errorf("failed to prepare edge stmt: %w", err)
			}
			defer edgeStmt.Close()

			statStmt, err := tx.PrepareContext(ctx, `
INSERT INTO edge_run_stats (edge_id, run_id, weight)
VALUES (?, ?, ?)
ON CONFLICT (edge_id, run_id) DO UPDATE SET
	weight = edge_run_stats.weight + excluded.weight;`)
			if err != nil {
				return fmt.Errorf("failed to prepare edge stat stmt: %w", err)
			}
			defer statStmt.Close()

			for _, ed := range edgeMap {
				if _, err := edgeStmt.ExecContext(ctx, ed.edgeID, ed.src, ed.dst, ed.edgeType, ed.weight, ed.firstSeen, ed.lastSeen, runID, ed.confidence); err != nil {
					return fmt.Errorf("failed to upsert edge %s: %w", ed.edgeID, err)
				}
				if _, err := statStmt.ExecContext(ctx, ed.edgeID, runID, ed.weight); err != nil {
					return fmt.Errorf("failed to upsert edge stat %s: %w", ed.edgeID, err)
				}
			}

			// Insert tls_observations
			tlsStmt, err := tx.PrepareContext(ctx, `
INSERT INTO tls_observations (
	run_id, conn_key, first_ts, last_ts, direction, transport,
	local_ip, local_port, remote_ip, remote_port, sni, tls_version,
	cipher_suite, kex_group, pqc_class, outcome, peer_cert_fp, peer_cert_source,
	pid, exe, comm, unit, attribution, record_count, handshake_count,
	appdata_count, alert_count, handshake_seq
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (run_id, conn_key) DO NOTHING;`)
			if err != nil {
				return fmt.Errorf("failed to prepare tls obs stmt: %w", err)
			}
			defer tlsStmt.Close()

			for _, rec := range records {
				var peerFP *string
				if rec.PeerCertificate != nil {
					peerFP = &rec.PeerCertificate.FingerprintSHA256
				}
				var pid *int
				var exe, comm, unit, attr *string
				if rec.Process != nil {
					pid = rec.Process.PID
					if rec.Process.Exe != "" {
						exe = &rec.Process.Exe
					}
					if rec.Process.Comm != "" {
						comm = &rec.Process.Comm
					}
					unit = rec.Process.Unit
					if rec.Process.Attribution != "" {
						attr = &rec.Process.Attribution
					}
				}

				if _, err := tlsStmt.ExecContext(ctx,
					runID, rec.ConnKey, rec.FirstTS, rec.LastTS, rec.Direction, rec.Transport,
					rec.LocalIP, rec.LocalPort, rec.RemoteIP, rec.RemotePort, rec.SNI, rec.TLSVersion,
					rec.CipherSuite, rec.KexGroup, rec.PQCClass, rec.Outcome, peerFP, rec.PeerCertSource,
					pid, exe, comm, unit, attr, rec.RecordCount, rec.HandshakeCount,
					rec.AppdataCount, rec.AlertCount, rec.HandshakeSeq,
				); err != nil {
					return fmt.Errorf("failed to insert tls obs %s: %w", rec.ConnKey, err)
				}
			}
		}

		// 6. Record batch in ingest_batches
		if _, err := tx.ExecContext(ctx, "INSERT INTO ingest_batches (run_id, seq, received_at) VALUES (?, ?, ?);", runID, env.Seq, now); err != nil {
			return fmt.Errorf("failed to record ingest batch: %w", err)
		}

		return nil
	})

	if err != nil {
		return false, err
	}
	return alreadyProcessed, nil
}

// CompleteRun processes a run completion payload per docs/06.
func CompleteRun(ctx context.Context, s *store.Store, runID string, req CompleteRequest) (*store.Run, error) {
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, ErrRunNotFound
	}

	// 1. Determine final status per docs/06:
	// status_hint=failed => failed; else any error with a non-empty code => partial; else completed.
	var finalStatus string
	if req.StatusHint == "failed" {
		finalStatus = "failed"
	} else {
		hasError := false
		for _, e := range req.Errors {
			if e.Code != "" {
				hasError = true
				break
			}
		}
		if hasError {
			finalStatus = "partial"
		} else {
			finalStatus = "completed"
		}
	}

	now := time.Now().UTC()
	startedAt := req.StartedAt
	if startedAt == nil {
		if run.StartedAt != nil {
			startedAt = run.StartedAt
		} else {
			startedAt = &run.RequestedAt
		}
	}
	finishedAt := req.FinishedAt
	if finishedAt == nil {
		finishedAt = &now
	}
	durMs := finishedAt.Sub(*startedAt).Milliseconds()
	if durMs < 0 {
		durMs = 0
	}

	errCount := len(req.Errors)
	var errSummary *string
	if errCount > 0 {
		var parts []string
		for _, e := range req.Errors {
			if e.Path != nil && *e.Path != "" {
				parts = append(parts, fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, *e.Path))
			} else {
				parts = append(parts, fmt.Sprintf("%s: %s", e.Code, e.Message))
			}
		}
		summaryStr := strings.Join(parts, "; ")
		errSummary = &summaryStr
	}

	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		// 2. Recompute records_total and records_unique
		var recTotal, recUnique int64
		if run.Type == "filesystem" {
			_ = tx.QueryRowContext(ctx, "SELECT COALESCE(COUNT(*), 0), COALESCE(COUNT(DISTINCT fingerprint_sha256), 0) FROM cert_observations WHERE run_id = ? AND parse_status = 'ok' AND fingerprint_sha256 IS NOT NULL;", runID).Scan(&recTotal, &recUnique)
		} else {
			_ = tx.QueryRowContext(ctx, "SELECT COALESCE(SUM(record_count), 0), COALESCE(COUNT(DISTINCT conn_key), 0) FROM tls_observations WHERE run_id = ?;", runID).Scan(&recTotal, &recUnique)
		}

		// 3. Insert logs
		if len(req.Logs) > 0 {
			for idx, l := range req.Logs {
				logTS := now
				if l.TS != nil {
					logTS = *l.TS
				}
				if _, err := tx.ExecContext(ctx, "INSERT INTO run_logs (run_id, seq, ts, level, msg) VALUES (?, ?, ?, ?, ?) ON CONFLICT (run_id, seq) DO NOTHING;", runID, idx+1, logTS, l.Level, l.Msg); err != nil {
					return fmt.Errorf("failed to insert log: %w", err)
				}
			}
		}

		// 4. Update run row
		var agentVer *string
		if req.Agent.Version != "" {
			agentVer = &req.Agent.Version
		}
		priv := req.Agent.Privileged

		updateQuery := `
UPDATE runs SET
	status = ?,
	started_at = ?,
	finished_at = ?,
	duration_ms = ?,
	records_total = ?,
	records_unique = ?,
	error_count = ?,
	error_summary = ?,
	agent_version = ?,
	capture_method = ?,
	capture_note = ?,
	privileged = ?
WHERE run_id = ?;`

		if _, err := tx.ExecContext(ctx, updateQuery,
			finalStatus, startedAt, finishedAt, durMs,
			recTotal, recUnique, errCount, errSummary,
			agentVer, req.CaptureMethod, req.CaptureNote, priv,
			runID,
		); err != nil {
			return fmt.Errorf("failed to update run: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.GetRun(ctx, runID)
}
