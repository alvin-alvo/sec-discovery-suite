package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

type Store struct {
	db *sql.DB
	mu sync.Mutex
}

func Open(dbPath string) (*Store, error) {
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}
	}

	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open duckdb: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping duckdb: %w", err)
	}

	if err := applyMigrations(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	s := &Store{db: db}
	if err := s.initDefaultSchedules(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to init schedules: %w", err)
	}

	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Lock() {
	s.mu.Lock()
}

func (s *Store) Unlock() {
	s.mu.Unlock()
}

func (s *Store) WithTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (s *Store) initDefaultSchedules(ctx context.Context) error {
	query := `
INSERT INTO schedules (id, type, interval_seconds, enabled, params, last_run_id, next_run_at)
VALUES (?, ?, ?, ?, ?, NULL, NULL)
ON CONFLICT (id) DO NOTHING;`

	if _, err := s.db.ExecContext(ctx, query, "filesystem", "filesystem", 86400, false, "{}"); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, query, "tls", "tls", 3600, false, "{}"); err != nil {
		return err
	}
	return nil
}

// CreateRun inserts a new run record.
func (s *Store) CreateRun(ctx context.Context, r *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
INSERT INTO runs (
	run_id, type, trigger, status, params,
	requested_at, started_at, finished_at, duration_ms,
	records_total, records_unique, error_count, error_summary,
	agent_version, capture_method, capture_note, privileged, is_sample
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	var paramsStr *string
	if len(r.Params) > 0 {
		str := string(r.Params)
		paramsStr = &str
	}

	_, err := s.db.ExecContext(ctx, query,
		r.RunID, r.Type, r.Trigger, r.Status, paramsStr,
		r.RequestedAt, r.StartedAt, r.FinishedAt, r.DurationMs,
		r.RecordsTotal, r.RecordsUnique, r.ErrorCount, r.ErrorSummary,
		r.AgentVersion, r.CaptureMethod, r.CaptureNote, r.Privileged, r.IsSample,
	)
	return err
}

// UpdateRunStatus updates the state machine status and timing/error fields of a run.
func (s *Store) UpdateRunStatus(ctx context.Context, runID, status string, startedAt, finishedAt *time.Time, durationMs *int64, errorSummary *string, errorCount *int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
UPDATE runs SET
	status = ?,
	started_at = COALESCE(?, started_at),
	finished_at = COALESCE(?, finished_at),
	duration_ms = COALESCE(?, duration_ms),
	error_summary = COALESCE(?, error_summary),
	error_count = COALESCE(?, error_count)
WHERE run_id = ?;`

	_, err := s.db.ExecContext(ctx, query,
		status, startedAt, finishedAt, durationMs, errorSummary, errorCount, runID,
	)
	return err
}

// GetRun returns a run by ID.
func (s *Store) GetRun(ctx context.Context, runID string) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
SELECT run_id, type, trigger, status, params,
       requested_at, started_at, finished_at, duration_ms,
       records_total, records_unique, error_count, error_summary,
       agent_version, capture_method, capture_note, privileged, is_sample
FROM runs WHERE run_id = ?;`

	row := s.db.QueryRowContext(ctx, query, runID)
	r, err := scanRun(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

// ListRuns returns runs with optional filtering and cursor pagination.
func (s *Store) ListRuns(ctx context.Context, runType, status string, cursor string, limit int) ([]Run, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var sb strings.Builder
	sb.WriteString(`
SELECT run_id, type, trigger, status, params,
       requested_at, started_at, finished_at, duration_ms,
       records_total, records_unique, error_count, error_summary,
       agent_version, capture_method, capture_note, privileged, is_sample
FROM runs WHERE 1=1`)

	var args []any
	if runType != "" {
		sb.WriteString(" AND type = ?")
		args = append(args, runType)
	}
	if status != "" {
		sb.WriteString(" AND status = ?")
		args = append(args, status)
	}
	if cursor != "" {
		sb.WriteString(" AND run_id < ?")
		args = append(args, cursor)
	}

	sb.WriteString(" ORDER BY run_id DESC LIMIT ?")
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, "", err
		}
		runs = append(runs, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if len(runs) > limit {
		nextCursor = runs[limit-1].RunID
		runs = runs[:limit]
	}

	return runs, nextCursor, nil
}

// HasActiveRunOfType returns true if a run with the given type is currently queued or running.
func (s *Store) HasActiveRunOfType(ctx context.Context, runType string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM runs WHERE type = ? AND status IN ('queued', 'running')", runType).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetRunLogs returns all log entries for a run ordered by sequence.
func (s *Store) GetRunLogs(ctx context.Context, runID string) ([]RunLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := "SELECT run_id, seq, ts, level, msg FROM run_logs WHERE run_id = ? ORDER BY seq ASC;"
	rows, err := s.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []RunLog
	for rows.Next() {
		var l RunLog
		if err := rows.Scan(&l.RunID, &l.Seq, &l.Ts, &l.Level, &l.Msg); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// InsertRunLogs inserts multiple log entries.
func (s *Store) InsertRunLogs(ctx context.Context, runID string, logs []RunLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stmt, err := s.db.PrepareContext(ctx, "INSERT INTO run_logs (run_id, seq, ts, level, msg) VALUES (?, ?, ?, ?, ?) ON CONFLICT (run_id, seq) DO NOTHING;")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, l := range logs {
		if _, err := stmt.ExecContext(ctx, runID, l.Seq, l.Ts, l.Level, l.Msg); err != nil {
			return err
		}
	}
	return nil
}

// GetFSSummary computes summary statistics for a filesystem run.
func (s *Store) GetFSSummary(ctx context.Context, runID string) (*FSSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	summary := &FSSummary{
		Algorithms: []NamedCount{},
		KeySizes:   []NamedCount{},
		SigAlgos:   []NamedCount{},
		TopIssuers: []NamedCount{},
		Locations:  []NamedCount{},
	}

	// 1. Found ok and unique
	err := s.db.QueryRowContext(ctx, `
SELECT
	COALESCE(COUNT(CASE WHEN parse_status = 'ok' THEN 1 END), 0),
	COALESCE(COUNT(DISTINCT CASE WHEN parse_status = 'ok' AND fingerprint_sha256 IS NOT NULL THEN fingerprint_sha256 END), 0),
	COALESCE(COUNT(CASE WHEN validity_status = 'valid' THEN 1 END), 0),
	COALESCE(COUNT(CASE WHEN validity_status = 'expiring' THEN 1 END), 0),
	COALESCE(COUNT(CASE WHEN validity_status = 'expired' THEN 1 END), 0),
	COALESCE(COUNT(CASE WHEN parse_status = 'parse_failed' THEN 1 END), 0),
	COALESCE(COUNT(CASE WHEN parse_status = 'access_denied' THEN 1 END), 0)
FROM cert_observations WHERE run_id = ?;`, runID).Scan(
		&summary.Found,
		&summary.Unique,
		&summary.Valid,
		&summary.Expiring,
		&summary.Expired,
		&summary.ParseFailed,
		&summary.AccessDenied,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query cert_observations counts: %w", err)
	}

	summary.Duplicates = summary.Found - summary.Unique
	if summary.Duplicates < 0 {
		summary.Duplicates = 0
	}

	// 2. Algorithms
	algRows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(c.pubkey_algo, 'unknown'), COUNT(*)
FROM cert_observations o
JOIN certificates c ON o.fingerprint_sha256 = c.fingerprint_sha256
WHERE o.run_id = ?
GROUP BY 1 ORDER BY 2 DESC;`, runID)
	if err == nil {
		for algRows.Next() {
			var nc NamedCount
			if err := algRows.Scan(&nc.Name, &nc.Count); err == nil {
				summary.Algorithms = append(summary.Algorithms, nc)
			}
		}
		algRows.Close()
	}

	// 3. Key sizes
	sizeRows, err := s.db.QueryContext(ctx, `
SELECT CAST(c.pubkey_bits AS TEXT), COUNT(*)
FROM cert_observations o
JOIN certificates c ON o.fingerprint_sha256 = c.fingerprint_sha256
WHERE o.run_id = ? AND c.pubkey_bits IS NOT NULL
GROUP BY 1 ORDER BY 2 DESC;`, runID)
	if err == nil {
		for sizeRows.Next() {
			var nc NamedCount
			if err := sizeRows.Scan(&nc.Name, &nc.Count); err == nil {
				summary.KeySizes = append(summary.KeySizes, nc)
			}
		}
		sizeRows.Close()
	}

	// 4. Sig algos
	sigRows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(c.sig_algo, 'unknown'), COUNT(*)
FROM cert_observations o
JOIN certificates c ON o.fingerprint_sha256 = c.fingerprint_sha256
WHERE o.run_id = ?
GROUP BY 1 ORDER BY 2 DESC;`, runID)
	if err == nil {
		for sigRows.Next() {
			var nc NamedCount
			if err := sigRows.Scan(&nc.Name, &nc.Count); err == nil {
				summary.SigAlgos = append(summary.SigAlgos, nc)
			}
		}
		sigRows.Close()
	}

	// 5. Top issuers
	issRows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(c.issuer_cn, c.issuer_dn, 'unknown'), COUNT(*)
FROM cert_observations o
JOIN certificates c ON o.fingerprint_sha256 = c.fingerprint_sha256
WHERE o.run_id = ?
GROUP BY 1 ORDER BY 2 DESC LIMIT 10;`, runID)
	if err == nil {
		for issRows.Next() {
			var nc NamedCount
			if err := issRows.Scan(&nc.Name, &nc.Count); err == nil {
				summary.TopIssuers = append(summary.TopIssuers, nc)
			}
		}
		issRows.Close()
	}

	// 6. Locations
	locRows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(o.scan_root, 'other'), COUNT(*)
FROM cert_observations o
WHERE o.run_id = ?
GROUP BY 1 ORDER BY 2 DESC LIMIT 10;`, runID)
	if err == nil {
		for locRows.Next() {
			var nc NamedCount
			if err := locRows.Scan(&nc.Name, &nc.Count); err == nil {
				summary.Locations = append(summary.Locations, nc)
			}
		}
		locRows.Close()
	}

	return summary, nil
}

// ListFSCertificates returns certificate observations with joined certificate details.
func (s *Store) ListFSCertificates(ctx context.Context, runID string, q, status, issuer, algo, format, parseStatus, sort string, limit, offset int) ([]CertObservationItem, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	whereClause := " WHERE 1=1"
	var args []any

	if runID != "" {
		whereClause += " AND o.run_id = ?"
		args = append(args, runID)
	}
	if status != "" {
		whereClause += " AND o.validity_status = ?"
		args = append(args, status)
	}
	if issuer != "" {
		whereClause += " AND (c.issuer_cn ILIKE ? OR c.issuer_dn ILIKE ?)"
		pattern := "%" + issuer + "%"
		args = append(args, pattern, pattern)
	}
	if algo != "" {
		whereClause += " AND c.pubkey_algo = ?"
		args = append(args, algo)
	}
	if format != "" {
		whereClause += " AND o.format = ?"
		args = append(args, format)
	}
	if parseStatus != "" {
		whereClause += " AND o.parse_status = ?"
		args = append(args, parseStatus)
	}
	if q != "" {
		whereClause += " AND (o.path ILIKE ? OR c.subject_cn ILIKE ? OR c.issuer_cn ILIKE ? OR o.fingerprint_sha256 ILIKE ?)"
		pattern := "%" + q + "%"
		args = append(args, pattern, pattern, pattern, pattern)
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM cert_observations o LEFT JOIN certificates c ON o.fingerprint_sha256 = c.fingerprint_sha256" + whereClause
	var total int64
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count certificates: %w", err)
	}

	// Order by
	orderBy := " ORDER BY o.path ASC, o.chain_position ASC"
	switch sort {
	case "path_asc":
		orderBy = " ORDER BY o.path ASC"
	case "path_desc":
		orderBy = " ORDER BY o.path DESC"
	case "expiry_asc":
		orderBy = " ORDER BY o.days_to_expiry ASC NULLS LAST"
	case "expiry_desc":
		orderBy = " ORDER BY o.days_to_expiry DESC NULLS LAST"
	case "subject_asc":
		orderBy = " ORDER BY c.subject_cn ASC NULLS LAST"
	case "subject_desc":
		orderBy = " ORDER BY c.subject_cn DESC NULLS LAST"
	}

	query := `
SELECT
	o.obs_id, o.run_id, o.fingerprint_sha256, o.path, o.resolved_path, o.filename, o.format, o.scan_root,
	o.chain_position, o.chain_length, o.file_mode, o.file_size, o.is_symlink, o.contains_private_key,
	o.parse_status, o.parse_error, o.days_to_expiry, o.validity_status,
	o.owner_pid, o.owner_proc, o.owner_unit, o.owner_confidence, o.discovered_at,
	c.subject_dn, c.subject_cn, c.issuer_dn, c.issuer_cn, c.san,
	c.not_before, c.not_after, c.pubkey_algo, c.pubkey_bits, c.curve, c.sig_algo,
	c.is_ca, c.is_self_signed, c.pqc_class
FROM cert_observations o
LEFT JOIN certificates c ON o.fingerprint_sha256 = c.fingerprint_sha256` + whereClause + orderBy + " LIMIT ? OFFSET ?"

	queryArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query certificates: %w", err)
	}
	defer rows.Close()

	var items []CertObservationItem
	for rows.Next() {
		var item CertObservationItem
		var sanVal any
		if err := rows.Scan(
			&item.ObsID, &item.RunID, &item.FingerprintSHA256, &item.Path, &item.ResolvedPath, &item.Filename, &item.Format, &item.ScanRoot,
			&item.ChainPosition, &item.ChainLength, &item.FileMode, &item.FileSize, &item.IsSymlink, &item.ContainsPrivateKey,
			&item.ParseStatus, &item.ParseError, &item.DaysToExpiry, &item.ValidityStatus,
			&item.OwnerPID, &item.OwnerProc, &item.OwnerUnit, &item.OwnerConfidence, &item.DiscoveredAt,
			&item.SubjectDN, &item.SubjectCN, &item.IssuerDN, &item.IssuerCN, &sanVal,
			&item.NotBefore, &item.NotAfter, &item.PubkeyAlgo, &item.PubkeyBits, &item.Curve, &item.SigAlgo,
			&item.IsCA, &item.IsSelfSigned, &item.PQCClass,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan cert observation: %w", err)
		}
		item.SAN, _ = scanJSON(sanVal)
		items = append(items, item)
	}

	return items, total, rows.Err()
}

// GetFSFacets returns distinct filter values for the run or global.
func (s *Store) GetFSFacets(ctx context.Context, runID string) (*FSFacets, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	facets := &FSFacets{
		ValidityStatus: []string{},
		Issuers:        []string{},
		Algorithms:     []string{},
		Formats:        []string{},
		ParseStatus:    []string{},
	}

	whereClause := ""
	var args []any
	if runID != "" {
		whereClause = " WHERE o.run_id = ?"
		args = append(args, runID)
	}

	// Validity status
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT o.validity_status FROM cert_observations o"+whereClause+" AND o.validity_status IS NOT NULL ORDER BY 1", args...)
	if err == nil {
		for rows.Next() {
			var val string
			if err := rows.Scan(&val); err == nil && val != "" {
				facets.ValidityStatus = append(facets.ValidityStatus, val)
			}
		}
		rows.Close()
	}

	// Issuers
	issQuery := "SELECT DISTINCT COALESCE(c.issuer_cn, c.issuer_dn) FROM cert_observations o JOIN certificates c ON o.fingerprint_sha256 = c.fingerprint_sha256" + whereClause + " ORDER BY 1 LIMIT 50"
	rows, err = s.db.QueryContext(ctx, issQuery, args...)
	if err == nil {
		for rows.Next() {
			var val string
			if err := rows.Scan(&val); err == nil && val != "" {
				facets.Issuers = append(facets.Issuers, val)
			}
		}
		rows.Close()
	}

	// Algorithms
	algQuery := "SELECT DISTINCT c.pubkey_algo FROM cert_observations o JOIN certificates c ON o.fingerprint_sha256 = c.fingerprint_sha256" + whereClause + " AND c.pubkey_algo IS NOT NULL ORDER BY 1"
	rows, err = s.db.QueryContext(ctx, algQuery, args...)
	if err == nil {
		for rows.Next() {
			var val string
			if err := rows.Scan(&val); err == nil && val != "" {
				facets.Algorithms = append(facets.Algorithms, val)
			}
		}
		rows.Close()
	}

	// Formats
	rows, err = s.db.QueryContext(ctx, "SELECT DISTINCT o.format FROM cert_observations o"+whereClause+" AND o.format IS NOT NULL ORDER BY 1", args...)
	if err == nil {
		for rows.Next() {
			var val string
			if err := rows.Scan(&val); err == nil && val != "" {
				facets.Formats = append(facets.Formats, val)
			}
		}
		rows.Close()
	}

	// Parse Status
	rows, err = s.db.QueryContext(ctx, "SELECT DISTINCT o.parse_status FROM cert_observations o"+whereClause+" AND o.parse_status IS NOT NULL ORDER BY 1", args...)
	if err == nil {
		for rows.Next() {
			var val string
			if err := rows.Scan(&val); err == nil && val != "" {
				facets.ParseStatus = append(facets.ParseStatus, val)
			}
		}
		rows.Close()
	}

	return facets, nil
}

// GetRunGraph returns the relationship graph for a specific run.
func (s *Store) GetRunGraph(ctx context.Context, runID string) (*GraphResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	resp := &GraphResponse{
		Nodes: []GraphNode{},
		Edges: []GraphEdge{},
	}

	edgeQuery := `
SELECT e.edge_id, e.src, e.dst, e.type, s.weight, e.first_seen, e.last_seen, e.confidence
FROM edge_run_stats s
JOIN edges e ON s.edge_id = e.edge_id
WHERE s.run_id = ?;`

	rows, err := s.db.QueryContext(ctx, edgeQuery, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query edge run stats: %w", err)
	}
	defer rows.Close()

	nodeIDs := make(map[string]struct{})
	for rows.Next() {
		var edge GraphEdge
		if err := rows.Scan(&edge.ID, &edge.Src, &edge.Dst, &edge.Type, &edge.Weight, &edge.FirstSeen, &edge.LastSeen, &edge.Confidence); err != nil {
			return nil, err
		}
		resp.Edges = append(resp.Edges, edge)
		nodeIDs[edge.Src] = struct{}{}
		nodeIDs[edge.Dst] = struct{}{}
	}

	if len(nodeIDs) == 0 {
		return resp, nil
	}

	// Query nodes
	placeholders := strings.Repeat("?,", len(nodeIDs))
	placeholders = placeholders[:len(placeholders)-1]

	var args []any
	for id := range nodeIDs {
		args = append(args, id)
	}

	nodeQuery := fmt.Sprintf("SELECT node_id, type, label, props, first_seen, last_seen FROM nodes WHERE node_id IN (%s);", placeholders)
	nRows, err := s.db.QueryContext(ctx, nodeQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer nRows.Close()

	for nRows.Next() {
		var n GraphNode
		var label *string
		var propsVal any
		if err := nRows.Scan(&n.ID, &n.Type, &label, &propsVal, &n.FirstSeen, &n.LastSeen); err != nil {
			return nil, err
		}
		if label != nil {
			n.Label = *label
		}
		n.Props, _ = scanJSON(propsVal)
		resp.Nodes = append(resp.Nodes, n)
	}

	return resp, nil
}

// GetGlobalGraph returns the lifetime graph.
func (s *Store) GetGlobalGraph(ctx context.Context) (*GraphResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	resp := &GraphResponse{
		Nodes: []GraphNode{},
		Edges: []GraphEdge{},
	}

	edgeQuery := "SELECT edge_id, src, dst, type, weight, first_seen, last_seen, confidence FROM edges;"
	rows, err := s.db.QueryContext(ctx, edgeQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query edges: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var edge GraphEdge
		if err := rows.Scan(&edge.ID, &edge.Src, &edge.Dst, &edge.Type, &edge.Weight, &edge.FirstSeen, &edge.LastSeen, &edge.Confidence); err != nil {
			return nil, err
		}
		resp.Edges = append(resp.Edges, edge)
	}

	nodeQuery := "SELECT node_id, type, label, props, first_seen, last_seen FROM nodes;"
	nRows, err := s.db.QueryContext(ctx, nodeQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer nRows.Close()

	for nRows.Next() {
		var n GraphNode
		var label *string
		var propsVal any
		if err := nRows.Scan(&n.ID, &n.Type, &label, &propsVal, &n.FirstSeen, &n.LastSeen); err != nil {
			return nil, err
		}
		if label != nil {
			n.Label = *label
		}
		n.Props, _ = scanJSON(propsVal)
		resp.Nodes = append(resp.Nodes, n)
	}

	return resp, nil
}

// GetDashboardSummary returns dashboard summary metrics.
func (s *Store) GetDashboardSummary(ctx context.Context) (*DashboardSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	summary := &DashboardSummary{}

	// Latest FS run
	row := s.db.QueryRowContext(ctx, `
SELECT run_id, type, trigger, status, params, requested_at, started_at, finished_at, duration_ms,
       records_total, records_unique, error_count, error_summary, agent_version, capture_method, capture_note, privileged, is_sample
FROM runs WHERE type = 'filesystem' ORDER BY requested_at DESC LIMIT 1;`)
	fsRun, err := scanRun(row)
	if err == nil {
		summary.LatestFSRun = fsRun
	}

	// Latest TLS run
	rowTLS := s.db.QueryRowContext(ctx, `
SELECT run_id, type, trigger, status, params, requested_at, started_at, finished_at, duration_ms,
       records_total, records_unique, error_count, error_summary, agent_version, capture_method, capture_note, privileged, is_sample
FROM runs WHERE type = 'tls' ORDER BY requested_at DESC LIMIT 1;`)
	tlsRun, err := scanRun(rowTLS)
	if err == nil {
		summary.LatestTLSRun = tlsRun
	}

	// Total certs
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM certificates;").Scan(&summary.TotalCerts)

	// Active runs
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM runs WHERE status IN ('queued', 'running');").Scan(&summary.ActiveRuns)

	// Expiring certs
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM certificates WHERE not_after <= (CURRENT_TIMESTAMP + INTERVAL '30 days') AND not_after > CURRENT_TIMESTAMP;").Scan(&summary.ExpiringCerts)

	return summary, nil
}

func scanJSON(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	switch val := v.(type) {
	case []byte:
		if len(val) == 0 {
			return nil, nil
		}
		return json.RawMessage(val), nil
	case string:
		if val == "" {
			return nil, nil
		}
		return json.RawMessage(val), nil
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(b), nil
	}
}

// GetSchedules returns all schedules.
func (s *Store) GetSchedules(ctx context.Context) ([]Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx, "SELECT id, type, interval_seconds, enabled, params, last_run_id, next_run_at FROM schedules ORDER BY id ASC;")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []Schedule
	for rows.Next() {
		var sch Schedule
		var paramsVal any
		if err := rows.Scan(&sch.ID, &sch.Type, &sch.IntervalSeconds, &sch.Enabled, &paramsVal, &sch.LastRunID, &sch.NextRunAt); err != nil {
			return nil, err
		}
		raw, err := scanJSON(paramsVal)
		if err != nil {
			return nil, err
		}
		sch.Params = raw
		schedules = append(schedules, sch)
	}
	return schedules, nil
}

// UpdateSchedule updates a schedule configuration.
func (s *Store) UpdateSchedule(ctx context.Context, sch Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var paramsStr *string
	if len(sch.Params) > 0 {
		str := string(sch.Params)
		paramsStr = &str
	}

	query := `
UPDATE schedules SET
	interval_seconds = COALESCE(?, interval_seconds),
	enabled = COALESCE(?, enabled),
	params = COALESCE(?, params),
	next_run_at = COALESCE(?, next_run_at)
WHERE id = ?;`

	_, err := s.db.ExecContext(ctx, query, sch.IntervalSeconds, sch.Enabled, paramsStr, sch.NextRunAt, sch.ID)
	return err
}

// GetDueSchedules returns all enabled schedules whose next_run_at is <= now or NULL.
func (s *Store) GetDueSchedules(ctx context.Context, now time.Time) ([]Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx, "SELECT id, type, interval_seconds, enabled, params, last_run_id, next_run_at FROM schedules WHERE enabled = true AND (next_run_at IS NULL OR next_run_at <= ?) ORDER BY id ASC;", now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []Schedule
	for rows.Next() {
		var sch Schedule
		var paramsVal any
		if err := rows.Scan(&sch.ID, &sch.Type, &sch.IntervalSeconds, &sch.Enabled, &paramsVal, &sch.LastRunID, &sch.NextRunAt); err != nil {
			return nil, err
		}
		sch.Params, _ = scanJSON(paramsVal)
		schedules = append(schedules, sch)
	}
	return schedules, nil
}

// UpdateScheduleNextRun updates last_run_id and next_run_at after a trigger.
func (s *Store) UpdateScheduleNextRun(ctx context.Context, id, runID string, nextRunAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, "UPDATE schedules SET last_run_id = ?, next_run_at = ? WHERE id = ?;", runID, nextRunAt, id)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRun(s rowScanner) (*Run, error) {
	var r Run
	var paramsVal any

	err := s.Scan(
		&r.RunID, &r.Type, &r.Trigger, &r.Status, &paramsVal,
		&r.RequestedAt, &r.StartedAt, &r.FinishedAt, &r.DurationMs,
		&r.RecordsTotal, &r.RecordsUnique, &r.ErrorCount, &r.ErrorSummary,
		&r.AgentVersion, &r.CaptureMethod, &r.CaptureNote, &r.Privileged, &r.IsSample,
	)
	if err != nil {
		return nil, err
	}
	r.Params, _ = scanJSON(paramsVal)
	return &r, nil
}

// GetTLSSummary computes summary statistics for a TLS run per docs/07.
func (s *Store) GetTLSSummary(ctx context.Context, runID string) (*TLSSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	summary := &TLSSummary{
		Versions:        []NamedCount{},
		Ciphers:         []NamedCount{},
		KexGroups:       []NamedCount{},
		Directions:      []NamedCount{},
		PeerCertSources: []NamedCount{},
		Attribution:     []NamedCount{},
		TopEndpoints:    []NamedCount{},
		CertPosture:     []NamedCount{},
		Timeline:        []TimelineBucket{},
	}

	// 1. Basic counts
	row := s.db.QueryRowContext(ctx, `
SELECT
	COALESCE(SUM(record_count), 0),
	COUNT(*),
	COALESCE(COUNT(DISTINCT (remote_ip || ':' || CAST(remote_port AS TEXT))), 0),
	COALESCE(COUNT(DISTINCT remote_port), 0),
	COALESCE(COUNT(DISTINCT peer_cert_fp), 0)
FROM tls_observations WHERE run_id = ?;`, runID)
	if err := row.Scan(&summary.Events, &summary.Connections, &summary.UniqueEndpoints, &summary.UniquePorts, &summary.CertsObserved); err != nil {
		return nil, fmt.Errorf("failed to query tls basic counts: %w", err)
	}

	// Relationships from edge_run_stats
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM edge_run_stats WHERE run_id = ?;", runID).Scan(&summary.Relationships)

	// Helper to load NamedCount lists
	loadNamedCounts := func(query string, target *[]NamedCount) {
		rows, err := s.db.QueryContext(ctx, query, runID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var nc NamedCount
				if err := rows.Scan(&nc.Name, &nc.Count); err == nil {
					*target = append(*target, nc)
				}
			}
		}
	}

	loadNamedCounts("SELECT COALESCE(tls_version, 'unknown'), COUNT(*) FROM tls_observations WHERE run_id = ? GROUP BY 1 ORDER BY 2 DESC;", &summary.Versions)
	loadNamedCounts("SELECT COALESCE(cipher_suite, 'unknown'), COUNT(*) FROM tls_observations WHERE run_id = ? GROUP BY 1 ORDER BY 2 DESC;", &summary.Ciphers)
	loadNamedCounts("SELECT COALESCE(kex_group, 'unknown'), COUNT(*) FROM tls_observations WHERE run_id = ? GROUP BY 1 ORDER BY 2 DESC;", &summary.KexGroups)
	loadNamedCounts("SELECT COALESCE(direction, 'unknown'), COUNT(*) FROM tls_observations WHERE run_id = ? GROUP BY 1 ORDER BY 2 DESC;", &summary.Directions)
	loadNamedCounts("SELECT COALESCE(peer_cert_source, 'unavailable'), COUNT(*) FROM tls_observations WHERE run_id = ? GROUP BY 1 ORDER BY 2 DESC;", &summary.PeerCertSources)
	loadNamedCounts("SELECT COALESCE(attribution, 'unavailable'), COUNT(*) FROM tls_observations WHERE run_id = ? GROUP BY 1 ORDER BY 2 DESC;", &summary.Attribution)

	// Top endpoints
	loadNamedCounts(`
SELECT
	CASE
		WHEN sni IS NOT NULL AND sni != '' THEN sni || ' (' || remote_ip || ':' || CAST(remote_port AS TEXT) || ')'
		ELSE remote_ip || ':' || CAST(remote_port AS TEXT)
	END,
	COUNT(*)
FROM tls_observations WHERE run_id = ? GROUP BY 1 ORDER BY 2 DESC LIMIT 10;`, &summary.TopEndpoints)

	// Cert posture evaluated at query time against now()
	loadNamedCounts(`
SELECT
	CASE
		WHEN o.peer_cert_fp IS NULL THEN 'unknown'
		WHEN c.not_after < CURRENT_TIMESTAMP THEN 'expired'
		WHEN c.not_after <= CURRENT_TIMESTAMP + INTERVAL '30 days' THEN 'expiring'
		ELSE 'valid'
	END,
	COUNT(*)
FROM tls_observations o
LEFT JOIN certificates c ON o.peer_cert_fp = c.fingerprint_sha256
WHERE o.run_id = ?
GROUP BY 1 ORDER BY 2 DESC;`, &summary.CertPosture)

	// Timeline in 5-second buckets of first_ts
	timelineRows, err := s.db.QueryContext(ctx, `
SELECT
	strftime(time_bucket(INTERVAL '5 seconds', first_ts), '%Y-%m-%dT%H:%M:%SZ'),
	SUM(record_count)
FROM tls_observations
WHERE run_id = ? AND first_ts IS NOT NULL
GROUP BY 1 ORDER BY 1 ASC;`, runID)
	if err == nil {
		defer timelineRows.Close()
		for timelineRows.Next() {
			var tb TimelineBucket
			if err := timelineRows.Scan(&tb.TS, &tb.Events); err == nil {
				summary.Timeline = append(summary.Timeline, tb)
			}
		}
	}

	return summary, nil
}

// ListTLSObservations queries TLS observations with filtering and optional grouping per docs/07.
func (s *Store) ListTLSObservations(ctx context.Context, runID, group, q, version, cipher, direction, attribution string, limit, offset int) (any, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	whereClause := " WHERE o.run_id = ?"
	args := []any{runID}

	if version != "" {
		whereClause += " AND o.tls_version = ?"
		args = append(args, version)
	}
	if cipher != "" {
		whereClause += " AND o.cipher_suite = ?"
		args = append(args, cipher)
	}
	if direction != "" {
		whereClause += " AND o.direction = ?"
		args = append(args, direction)
	}
	if attribution != "" {
		whereClause += " AND o.attribution = ?"
		args = append(args, attribution)
	}
	if q != "" {
		whereClause += " AND (o.remote_ip ILIKE ? OR o.sni ILIKE ? OR o.exe ILIKE ? OR o.comm ILIKE ? OR o.peer_cert_fp ILIKE ?)"
		pattern := "%" + q + "%"
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}

	if group == "endpoint" {
		// Group by endpoint: remote_ip, remote_port, sni
		countQuery := "SELECT COUNT(DISTINCT (o.remote_ip || ':' || CAST(o.remote_port AS TEXT) || ':' || COALESCE(o.sni, ''))) FROM tls_observations o" + whereClause
		var total int64
		if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("failed to count endpoint groups: %w", err)
		}

		query := `
SELECT
	o.remote_ip, o.remote_port, o.sni,
	COUNT(*) AS connections,
	SUM(o.record_count) AS events,
	list(DISTINCT o.tls_version) FILTER (WHERE o.tls_version IS NOT NULL),
	list(DISTINCT o.cipher_suite) FILTER (WHERE o.cipher_suite IS NOT NULL),
	list(DISTINCT COALESCE(o.comm, o.exe)) FILTER (WHERE o.comm IS NOT NULL OR o.exe IS NOT NULL),
	list(DISTINCT o.direction) FILTER (WHERE o.direction IS NOT NULL)
FROM tls_observations o` + whereClause + `
GROUP BY o.remote_ip, o.remote_port, o.sni
ORDER BY events DESC LIMIT ? OFFSET ?;`

		queryArgs := append(append([]any{}, args...), limit, offset)
		rows, err := s.db.QueryContext(ctx, query, queryArgs...)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to query grouped endpoints: %w", err)
		}
		defer rows.Close()

		var items []TLSEndpointGroupItem
		for rows.Next() {
			var it TLSEndpointGroupItem
			var versionsVal, ciphersVal, procsVal, dirsVal any
			if err := rows.Scan(
				&it.RemoteIP, &it.RemotePort, &it.SNI,
				&it.Connections, &it.Events,
				&versionsVal, &ciphersVal, &procsVal, &dirsVal,
			); err != nil {
				return nil, 0, fmt.Errorf("failed to scan endpoint group: %w", err)
			}
			it.Versions = toStringSlice(versionsVal)
			it.Ciphers = toStringSlice(ciphersVal)
			it.Processes = toStringSlice(procsVal)
			it.Directions = toStringSlice(dirsVal)
			items = append(items, it)
		}
		return items, total, rows.Err()
	}

	// group=none (flat observations)
	countQuery := "SELECT COUNT(*) FROM tls_observations o LEFT JOIN certificates c ON o.peer_cert_fp = c.fingerprint_sha256" + whereClause
	var total int64
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count tls observations: %w", err)
	}

	query := `
SELECT
	o.obs_id, o.run_id, o.conn_key, o.first_ts, o.last_ts, o.direction, o.transport,
	o.local_ip, o.local_port, o.remote_ip, o.remote_port, o.sni, o.tls_version,
	o.cipher_suite, o.kex_group, o.pqc_class, o.outcome, o.peer_cert_fp, o.peer_cert_source,
	o.pid, o.exe, o.comm, o.unit, o.attribution, o.record_count, o.handshake_count,
	o.appdata_count, o.alert_count, o.handshake_seq,
	c.subject_cn, c.issuer_cn, c.not_after, c.pubkey_algo, c.pubkey_bits,
	CASE
		WHEN o.peer_cert_fp IS NULL THEN NULL
		WHEN c.not_after < CURRENT_TIMESTAMP THEN 'expired'
		WHEN c.not_after <= CURRENT_TIMESTAMP + INTERVAL '30 days' THEN 'expiring'
		ELSE 'valid'
	END AS peer_validity
FROM tls_observations o
LEFT JOIN certificates c ON o.peer_cert_fp = c.fingerprint_sha256` + whereClause + `
ORDER BY o.first_ts DESC NULLS LAST LIMIT ? OFFSET ?;`

	queryArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query tls observations: %w", err)
	}
	defer rows.Close()

	var items []TLSObservationItem
	for rows.Next() {
		var it TLSObservationItem
		if err := rows.Scan(
			&it.ObsID, &it.RunID, &it.ConnKey, &it.FirstTS, &it.LastTS, &it.Direction, &it.Transport,
			&it.LocalIP, &it.LocalPort, &it.RemoteIP, &it.RemotePort, &it.SNI, &it.TLSVersion,
			&it.CipherSuite, &it.KexGroup, &it.PQCClass, &it.Outcome, &it.PeerCertFP, &it.PeerCertSource,
			&it.PID, &it.Exe, &it.Comm, &it.Unit, &it.Attribution, &it.RecordCount, &it.HandshakeCount,
			&it.AppdataCount, &it.AlertCount, &it.HandshakeSeq,
			&it.PeerSubjectCN, &it.PeerIssuerCN, &it.PeerNotAfter, &it.PeerPubkeyAlgo, &it.PeerPubkeyBits,
			&it.PeerValidity,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan tls observation: %w", err)
		}
		items = append(items, it)
	}

	return items, total, rows.Err()
}

// GetTLSEvidence computes the mathematical breakdown behind report totals per docs/07.
func (s *Store) GetTLSEvidence(ctx context.Context, runID string) (*TLSEvidence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ev := &TLSEvidence{}
	ev.Events.Formula = "SUM(record_count)"
	ev.Events.ByConnection = []TLSConnRecordCount{}
	ev.UniqueEndpoints.Formula = "COUNT(DISTINCT remote_ip, remote_port)"
	ev.UniqueEndpoints.Items = []string{}
	ev.CertsObserved.Items = []TLSCertObsItem{}

	// 1. Events total and breakdown
	row := s.db.QueryRowContext(ctx, `
SELECT
	COALESCE(SUM(record_count), 0),
	COALESCE(SUM(handshake_count), 0),
	COALESCE(SUM(appdata_count), 0),
	COALESCE(SUM(alert_count), 0)
FROM tls_observations WHERE run_id = ?;`, runID)
	if err := row.Scan(&ev.Events.Total, &ev.Events.Breakdown.Handshake, &ev.Events.Breakdown.Appdata, &ev.Events.Breakdown.Alert); err != nil {
		return nil, fmt.Errorf("failed to query tls events evidence: %w", err)
	}
	other := ev.Events.Total - (ev.Events.Breakdown.Handshake + ev.Events.Breakdown.Appdata + ev.Events.Breakdown.Alert)
	if other < 0 {
		other = 0
	}
	ev.Events.Breakdown.Other = other

	// 2. Events by connection (top 200)
	connRows, err := s.db.QueryContext(ctx, `
SELECT conn_key, remote_ip || ':' || CAST(remote_port AS TEXT), sni, record_count
FROM tls_observations WHERE run_id = ? ORDER BY record_count DESC LIMIT 200;`, runID)
	if err == nil {
		defer connRows.Close()
		for connRows.Next() {
			var c TLSConnRecordCount
			if err := connRows.Scan(&c.ConnKey, &c.Remote, &c.SNI, &c.RecordCount); err == nil {
				ev.Events.ByConnection = append(ev.Events.ByConnection, c)
			}
		}
	}

	// 3. Unique endpoints
	epRows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT remote_ip || ':' || CAST(remote_port AS TEXT)
FROM tls_observations WHERE run_id = ? ORDER BY 1;`, runID)
	if err == nil {
		defer epRows.Close()
		for epRows.Next() {
			var ep string
			if err := epRows.Scan(&ep); err == nil && ep != "" {
				ev.UniqueEndpoints.Items = append(ev.UniqueEndpoints.Items, ep)
			}
		}
	}
	ev.UniqueEndpoints.Total = int64(len(ev.UniqueEndpoints.Items))

	// 4. Certs observed
	certRows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT peer_cert_fp, COALESCE(peer_cert_source, 'observed')
FROM tls_observations WHERE run_id = ? AND peer_cert_fp IS NOT NULL ORDER BY 1;`, runID)
	if err == nil {
		defer certRows.Close()
		for certRows.Next() {
			var ci TLSCertObsItem
			if err := certRows.Scan(&ci.FingerprintSHA256, &ci.Source); err == nil && ci.FingerprintSHA256 != "" {
				ev.CertsObserved.Items = append(ev.CertsObserved.Items, ci)
			}
		}
	}
	ev.CertsObserved.Total = int64(len(ev.CertsObserved.Items))

	return ev, nil
}

func toStringSlice(val any) []string {
	if val == nil {
		return []string{}
	}
	switch v := val.(type) {
	case []any:
		res := make([]string, 0, len(v))
		for _, item := range v {
			if item != nil {
				res = append(res, fmt.Sprint(item))
			}
		}
		return res
	case []string:
		return v
	default:
		return []string{}
	}
}
