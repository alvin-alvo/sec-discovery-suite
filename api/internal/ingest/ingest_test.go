package ingest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"sds/api/internal/store"
)

func generateTestCert(t *testing.T) (*CertificateData, []byte) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	notBefore := time.Now().Add(-1 * time.Hour).UTC()
	notAfter := notBefore.Add(365 * 24 * time.Hour).UTC()

	template := x509.Certificate{
		SerialNumber: big.NewInt(123456789),
		Subject: pkix.Name{
			CommonName:   "test.example.internal",
			Organization: []string{"SDS Test"},
		},
		Issuer: pkix.Name{
			CommonName:   "test.example.internal",
			Organization: []string{"SDS Test"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              []string{"test.example.internal", "localhost"},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create cert: %v", err)
	}

	h := sha256.Sum256(der)
	fp := hex.EncodeToString(h[:])

	subjDN := "CN=test.example.internal,O=SDS Test"
	subjCN := "test.example.internal"
	serial := "123456789"
	algo := "ECDSA"
	bits := 256
	curve := "P-256"
	sigAlgo := "ECDSA-SHA256"
	isCA := false
	isSelfSigned := true
	pqc := "classical"

	certData := &CertificateData{
		FingerprintSHA256: fp,
		Serial:            &serial,
		SubjectDN:         &subjDN,
		SubjectCN:         &subjCN,
		IssuerDN:          &subjDN,
		IssuerCN:          &subjCN,
		SAN:               []string{"dns:test.example.internal", "dns:localhost"},
		NotBefore:         &notBefore,
		NotAfter:          &notAfter,
		PubkeyAlgo:        &algo,
		PubkeyBits:        &bits,
		Curve:             &curve,
		SigAlgo:           &sigAlgo,
		IsCA:              &isCA,
		IsSelfSigned:      &isSelfSigned,
		PQCClass:          &pqc,
	}

	return certData, der
}

func TestIngest_DuplicateFingerprintsAndIdempotency(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// 1. Create a filesystem run
	runID := "01JTESTFS00000000000000001"
	err = s.CreateRun(ctx, &store.Run{
		RunID:       runID,
		Type:        "filesystem",
		Trigger:     "manual",
		Status:      "running",
		RequestedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	certData, _ := generateTestCert(t)
	fp := certData.FingerprintSHA256

	// Batch 1 has DUPLICATE observations of the SAME certificate (e.g., symlink and original)
	now := time.Now().UTC()
	days := 365
	valid := "valid"
	rec1 := CertObservationRecord{
		Path:           "/etc/ssl/certs/cert.pem",
		Filename:       strPtr("cert.pem"),
		Format:         strPtr("PEM"),
		ScanRoot:       strPtr("/etc/ssl"),
		ChainPosition:  0,
		FileMode:       strPtr("0644"),
		IsSymlink:      false,
		ParseStatus:    "ok",
		DaysToExpiry:   &days,
		ValidityStatus: &valid,
		DiscoveredAt:   &now,
		Certificate:    certData,
		Owner: &OwnerInfo{
			PID:        intPtr(100),
			Proc:       "/usr/sbin/nginx",
			Confidence: "high",
		},
	}

	rec2 := CertObservationRecord{
		Path:           "/etc/nginx/ssl/server.crt",
		ResolvedPath:   strPtr("/etc/ssl/certs/cert.pem"),
		Filename:       strPtr("server.crt"),
		Format:         strPtr("PEM"),
		ScanRoot:       strPtr("/etc/nginx"),
		ChainPosition:  0,
		FileMode:       strPtr("0644"),
		IsSymlink:      true,
		ParseStatus:    "ok",
		DaysToExpiry:   &days,
		ValidityStatus: &valid,
		DiscoveredAt:   &now,
		Certificate:    certData, // Duplicate!
	}

	recordsBytes, err := json.Marshal([]CertObservationRecord{rec1, rec2})
	if err != nil {
		t.Fatalf("marshal records failed: %v", err)
	}

	env1 := BatchEnvelope{
		Seq:  1,
		Kind: "cert_observations",
		Host: HostInfo{
			MachineIDHash: "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899",
			Hostname:      "test-vm",
		},
		Records: recordsBytes,
	}

	// 2. Process batch 1
	already, err := ProcessBatch(ctx, s, runID, env1)
	if err != nil {
		t.Fatalf("ProcessBatch failed: %v", err)
	}
	if already {
		t.Fatalf("expected already=false on first submission")
	}

	// Assert: In-Go aggregation combined duplicate keys, seen_count must be 2!
	var seenCount int64
	err = s.DB().QueryRowContext(ctx, "SELECT seen_count FROM certificates WHERE fingerprint_sha256 = ?", fp).Scan(&seenCount)
	if err != nil {
		t.Fatalf("query seen_count failed: %v", err)
	}
	if seenCount != 2 {
		t.Fatalf("expected seen_count=2 after duplicate batch, got %d", seenCount)
	}

	// Assert: 2 observations recorded
	var obsCount int
	err = s.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM cert_observations WHERE run_id = ?", runID).Scan(&obsCount)
	if err != nil || obsCount != 2 {
		t.Fatalf("expected 2 cert_observations, got %d (err: %v)", obsCount, err)
	}

	// 3. Test IDEMPOTENCY: re-posting same seq changes nothing
	already, err = ProcessBatch(ctx, s, runID, env1)
	if err != nil {
		t.Fatalf("ProcessBatch retry failed: %v", err)
	}
	if !already {
		t.Fatalf("expected already=true on re-post of seq 1")
	}

	err = s.DB().QueryRowContext(ctx, "SELECT seen_count FROM certificates WHERE fingerprint_sha256 = ?", fp).Scan(&seenCount)
	if err != nil || seenCount != 2 {
		t.Fatalf("expected seen_count to remain 2 after re-post, got %d", seenCount)
	}

	// 4. Test Complete with access_denied => status must be 'partial', counts recomputed
	completeReq := CompleteRequest{
		StatusHint: "completed",
		StartedAt:  &now,
		FinishedAt: timePtr(now.Add(2 * time.Second)),
		Agent: AgentMeta{
			Version:    "1.0.0",
			UID:        0,
			Privileged: true,
		},
		Errors: []ScanError{
			{
				Code:    "access_denied",
				Message: "open /root/secret: permission denied",
				Path:    strPtr("/root/secret"),
			},
		},
		Logs: []LogItem{
			{
				TS:    &now,
				Level: "warn",
				Msg:   "denied /root/secret",
			},
		},
	}

	finalRun, err := CompleteRun(ctx, s, runID, completeReq)
	if err != nil {
		t.Fatalf("CompleteRun failed: %v", err)
	}

	if finalRun.Status != "partial" {
		t.Fatalf("expected final status 'partial', got '%s'", finalRun.Status)
	}
	if finalRun.ErrorCount != 1 {
		t.Fatalf("expected error_count=1, got %d", finalRun.ErrorCount)
	}
	if finalRun.RecordsTotal != 2 {
		t.Fatalf("expected recomputed records_total=2, got %d", finalRun.RecordsTotal)
	}
	if finalRun.RecordsUnique != 1 {
		t.Fatalf("expected recomputed records_unique=1, got %d", finalRun.RecordsUnique)
	}
	if finalRun.DurationMs == nil || *finalRun.DurationMs <= 0 {
		t.Fatalf("expected positive duration_ms, got %v", finalRun.DurationMs)
	}

	// Check run logs were inserted
	logs, err := s.GetRunLogs(ctx, runID)
	if err != nil || len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d (err: %v)", len(logs), err)
	}

	// Check graph derivation
	graph, err := s.GetRunGraph(ctx, runID)
	if err != nil {
		t.Fatalf("GetRunGraph failed: %v", err)
	}
	if len(graph.Nodes) < 2 {
		t.Fatalf("expected at least 2 nodes (host, cert), got %d", len(graph.Nodes))
	}
	if len(graph.Edges) < 1 {
		t.Fatalf("expected at least 1 edge (STORES), got %d", len(graph.Edges))
	}
}

func TestIngest_ValidationRules(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// 1. Unknown run_id => ErrRunNotFound
	_, err = ProcessBatch(ctx, s, "01JNONEXISTENT00000000000", BatchEnvelope{Seq: 1, Kind: "cert_observations"})
	if err != ErrRunNotFound {
		t.Fatalf("expected ErrRunNotFound, got %v", err)
	}

	// 2. Create FS run, submit TLS batch => ErrWrongRunType
	runID := "01JTESTFS00000000000000002"
	_ = s.CreateRun(ctx, &store.Run{
		RunID:       runID,
		Type:        "filesystem",
		Trigger:     "manual",
		Status:      "running",
		RequestedAt: time.Now().UTC(),
	})

	_, err = ProcessBatch(ctx, s, runID, BatchEnvelope{Seq: 1, Kind: "tls_observations"})
	if err == nil || err != ErrWrongRunType {
		// Can be wrapped
		if !errorsIs(err, ErrWrongRunType) {
			t.Fatalf("expected ErrWrongRunType, got %v", err)
		}
	}

	// 3. Malformed fingerprint => ErrInvalidFingerprint
	recBadFP := CertObservationRecord{
		Path:          "/a.crt",
		ChainPosition: 0,
		ParseStatus:   "ok",
		Certificate: &CertificateData{
			FingerprintSHA256: "not-64-hex",
		},
	}
	recBytes, _ := json.Marshal([]CertObservationRecord{recBadFP})
	_, err = ProcessBatch(ctx, s, runID, BatchEnvelope{
		Seq:     1,
		Kind:    "cert_observations",
		Records: recBytes,
	})
	if !errorsIs(err, ErrInvalidFingerprint) {
		t.Fatalf("expected ErrInvalidFingerprint, got %v", err)
	}
}

func errorsIs(err, target error) bool {
	if err == nil {
		return target == nil
	}
	return err == target || (err != nil && target != nil && err.Error() != "" && target.Error() != "" && (err == target || len(err.Error()) >= len(target.Error()) && (err.Error() == target.Error() || containsError(err.Error(), target.Error()))))
}

func containsError(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func timePtr(t time.Time) *time.Time {
	return &t
}
