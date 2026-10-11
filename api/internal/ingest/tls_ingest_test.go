package ingest

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"sds/api/internal/store"
)

func TestIngest_TLSAndLinkage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.duckdb")

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// 1. First, create an FS run and ingest a cert with fingerprint FP1
	fsRunID := "01JTESTFS0000000000000000A"
	err = s.CreateRun(ctx, &store.Run{
		RunID:       fsRunID,
		Type:        "filesystem",
		Trigger:     "manual",
		Status:      "running",
		RequestedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	sharedCert, _ := generateTestCert(t)
	sharedFP := sharedCert.FingerprintSHA256

	now := time.Now().UTC()
	fsRecord := CertObservationRecord{
		Path:          "/etc/ssl/certs/shared.crt",
		Filename:      strPtr("shared.crt"),
		Format:        strPtr("PEM"),
		ChainPosition: 0,
		ParseStatus:   "ok",
		DiscoveredAt:  &now,
		Certificate:   sharedCert,
	}
	fsBytes, _ := json.Marshal([]CertObservationRecord{fsRecord})
	_, err = ProcessBatch(ctx, s, fsRunID, BatchEnvelope{
		Seq:  1,
		Kind: "cert_observations",
		Host: HostInfo{MachineIDHash: "host1", Hostname: "server1"},
		Records: fsBytes,
	})
	if err != nil {
		t.Fatalf("ProcessBatch FS failed: %v", err)
	}

	// 2. Now create a TLS run and ingest TLS observations using the SAME certificate
	tlsRunID := "01JTESTTLS0000000000000001"
	err = s.CreateRun(ctx, &store.Run{
		RunID:       tlsRunID,
		Type:        "tls",
		Trigger:     "manual",
		Status:      "running",
		RequestedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateRun TLS failed: %v", err)
	}

	// Observation 1: Inbound connection with peer cert
	// Inbound presenter = LOCAL endpoint
	directionInbound := "inbound"
	outcomeEst := "established"
	localIP := "10.0.0.1"
	localPort := 443
	remoteIP := "192.168.1.50"
	remotePort := 54321
	tlsVer := "TLS1.3"
	cipher := "TLS_AES_128_GCM_SHA256"

	tlsRec1 := TLSObservationRecord{
		ConnKey:        "conn1111111111111111111111111111111111111111",
		FirstTS:        &now,
		LastTS:         timePtr(now.Add(1 * time.Second)),
		Direction:      &directionInbound,
		LocalIP:        &localIP,
		LocalPort:      localPort,
		RemoteIP:       &remoteIP,
		RemotePort:     remotePort,
		TLSVersion:     &tlsVer,
		CipherSuite:    &cipher,
		Outcome:        &outcomeEst,
		RecordCount:    10,
		HandshakeCount: 4,
		AppdataCount:   6,
		Process: &TLSProcess{
			PID:  intPtr(100),
			Exe:  "/usr/sbin/nginx",
			Comm: "nginx",
		},
		PeerCertificate: sharedCert, // Shared with FS cert!
	}

	// Observation 2: Outbound connection with peer cert
	// Outbound presenter = REMOTE endpoint
	directionOutbound := "outbound"
	remoteIP2 := "93.184.216.34"
	remotePort2 := 443
	sni := "example.com"
	tlsRec2 := TLSObservationRecord{
		ConnKey:        "conn2222222222222222222222222222222222222222",
		FirstTS:        timePtr(now.Add(2 * time.Second)),
		LastTS:         timePtr(now.Add(3 * time.Second)),
		Direction:      &directionOutbound,
		LocalIP:        &localIP,
		LocalPort:      45678,
		RemoteIP:       &remoteIP2,
		RemotePort:     remotePort2,
		SNI:            &sni,
		TLSVersion:     &tlsVer,
		CipherSuite:    &cipher,
		Outcome:        &outcomeEst,
		RecordCount:    5,
		HandshakeCount: 2,
		AppdataCount:   3,
		Process: &TLSProcess{
			PID:  intPtr(200),
			Exe:  "/usr/bin/curl",
			Comm: "curl",
		},
		PeerCertificate: sharedCert, // Duplicate key within batch!
	}

	tlsBytes, _ := json.Marshal([]TLSObservationRecord{tlsRec1, tlsRec2})
	envTLS := BatchEnvelope{
		Seq:  1,
		Kind: "tls_observations",
		Host: HostInfo{MachineIDHash: "host1", Hostname: "server1"},
		Records: tlsBytes,
	}

	already, err := ProcessBatch(ctx, s, tlsRunID, envTLS)
	if err != nil {
		t.Fatalf("ProcessBatch TLS failed: %v", err)
	}
	if already {
		t.Fatalf("expected already=false on first submission")
	}

	// Assert: Idempotency re-posting changes nothing
	already, err = ProcessBatch(ctx, s, tlsRunID, envTLS)
	if err != nil || !already {
		t.Fatalf("expected already=true on re-posting seq 1, got err=%v already=%v", err, already)
	}

	// Complete run
	completeReq := CompleteRequest{
		StatusHint: "completed",
		StartedAt:  &now,
		FinishedAt: timePtr(now.Add(5 * time.Second)),
		Agent:      AgentMeta{Version: "0.1.0"},
	}
	_, err = CompleteRun(ctx, s, tlsRunID, completeReq)
	if err != nil {
		t.Fatalf("CompleteRun failed: %v", err)
	}

	// 3. Verify Summary and Evidence
	summary, err := s.GetTLSSummary(ctx, tlsRunID)
	if err != nil {
		t.Fatalf("GetTLSSummary failed: %v", err)
	}

	if summary.Events != 15 {
		t.Fatalf("expected 15 events (10+5), got %d", summary.Events)
	}
	if summary.Connections != 2 {
		t.Fatalf("expected 2 connections, got %d", summary.Connections)
	}
	if summary.UniqueEndpoints != 2 {
		t.Fatalf("expected 2 unique endpoints, got %d", summary.UniqueEndpoints)
	}
	if summary.CertsObserved != 1 {
		t.Fatalf("expected 1 cert observed, got %d", summary.CertsObserved)
	}

	evidence, err := s.GetTLSEvidence(ctx, tlsRunID)
	if err != nil {
		t.Fatalf("GetTLSEvidence failed: %v", err)
	}

	// Evidence totals equal summary totals
	if evidence.Events.Total != summary.Events {
		t.Fatalf("evidence events total (%d) != summary events (%d)", evidence.Events.Total, summary.Events)
	}
	if evidence.UniqueEndpoints.Total != summary.UniqueEndpoints {
		t.Fatalf("evidence endpoints total (%d) != summary endpoints (%d)", evidence.UniqueEndpoints.Total, summary.UniqueEndpoints)
	}
	if evidence.CertsObserved.Total != summary.CertsObserved {
		t.Fatalf("evidence certs total (%d) != summary certs (%d)", evidence.CertsObserved.Total, summary.CertsObserved)
	}

	// 4. Check inbound observation produced PRESENTS edge from the LOCAL endpoint
	localEP := "ep:10.0.0.1:443"
	certNodeID := "cert:" + sharedFP
	inboundEdgeID := sha1Hex(localEP + "|PRESENTS|" + certNodeID)

	var edgeWeight int64
	err = s.DB().QueryRowContext(ctx, "SELECT weight FROM edges WHERE edge_id = ?", inboundEdgeID).Scan(&edgeWeight)
	if err != nil || edgeWeight < 1 {
		t.Fatalf("expected PRESENTS edge from local endpoint %s -> %s, got err=%v weight=%d", localEP, certNodeID, err, edgeWeight)
	}

	// 5. FS cert and TLS peer cert with same fingerprint resolve to ONE certificate node with both STORES and PRESENTS edges
	var certNodeCount int
	err = s.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes WHERE node_id = ?", certNodeID).Scan(&certNodeCount)
	if err != nil || certNodeCount != 1 {
		t.Fatalf("expected exactly 1 node for shared cert, got %d", certNodeCount)
	}

	var storesCount, presentsCount int
	_ = s.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM edges WHERE dst = ? AND type = 'STORES'", certNodeID).Scan(&storesCount)
	_ = s.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM edges WHERE dst = ? AND type = 'PRESENTS'", certNodeID).Scan(&presentsCount)

	if storesCount != 1 || presentsCount < 1 {
		t.Fatalf("expected cert to have both STORES (%d) and PRESENTS (%d) edges", storesCount, presentsCount)
	}

	// 6. Test Grouped endpoints listing
	grpItems, grpTotal, err := s.ListTLSObservations(ctx, tlsRunID, "endpoint", "", "", "", "", "", 10, 0)
	if err != nil {
		t.Fatalf("ListTLSObservations endpoint group failed: %v", err)
	}
	if grpTotal != 2 {
		t.Fatalf("expected 2 endpoint groups, got %d", grpTotal)
	}
	groups := grpItems.([]store.TLSEndpointGroupItem)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups in items, got %d", len(groups))
	}
}
