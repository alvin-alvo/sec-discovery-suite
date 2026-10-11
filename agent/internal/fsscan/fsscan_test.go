package fsscan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScan_Corpus(t *testing.T) {
	// Look for .test-certs/corpus
	corpusDir := "../../../.test-certs/corpus"
	if _, err := os.Stat(corpusDir); err != nil {
		corpusDir = ".test-certs/corpus"
		if _, err := os.Stat(corpusDir); err != nil {
			t.Skip("corpus not found, skipping corpus test")
		}
	}

	cleanCorpus, _ := filepath.Abs(corpusDir)
	now := time.Now()

	res, err := Scan([]string{cleanCorpus}, now)
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(res.Observations) == 0 {
		t.Fatal("expected observations from corpus, got 0")
	}

	foundCount := 0
	parseFailedCount := 0
	accessDeniedCount := 0
	passwordProtectedCount := 0
	uniqueFPs := make(map[string]bool)

	foundSymlink := false
	foundBundle := false
	bundlePositions := make(map[int]bool)
	foundPrivKeyFlag := false

	for _, obs := range res.Observations {
		switch obs.ParseStatus {
		case "ok":
			foundCount++
			if obs.Certificate != nil && obs.Certificate.FingerprintSHA256 != "" {
				uniqueFPs[obs.Certificate.FingerprintSHA256] = true
			}
		case "parse_failed":
			parseFailedCount++
		case "access_denied":
			accessDeniedCount++
		case "password_protected":
			passwordProtectedCount++
		}

		if obs.IsSymlink {
			foundSymlink = true
			if obs.ResolvedPath == nil || *obs.ResolvedPath == "" {
				t.Errorf("symlink observation missing resolved_path: %s", obs.Path)
			}
		}

		if strings.Contains(obs.Path, "bundle_3cert.pem") {
			foundBundle = true
			bundlePositions[obs.ChainPosition] = true
			if obs.ChainLength != 3 {
				t.Errorf("expected bundle chain_length 3, got %d", obs.ChainLength)
			}
		}

		if strings.Contains(obs.Path, "cert_and_key.pem") {
			if !obs.ContainsPrivateKey {
				t.Errorf("expected contains_private_key true for cert_and_key.pem")
			}
			foundPrivKeyFlag = true
		}

		// Security assertion: verify private key headers or bytes never leak
		data, _ := json.Marshal(obs)
		dataStr := string(data)
		if strings.Contains(dataStr, "PRIVATE KEY") && !strings.Contains(dataStr, `"contains_private_key":true`) {
			t.Fatalf("SECURITY VIOLATION: private key leaked in observation JSON: %s", dataStr)
		}
	}

	uniqueCount := len(uniqueFPs)
	duplicateCount := foundCount - uniqueCount

	t.Logf("Corpus Scan Summary: found=%d unique=%d duplicates=%d parse_failed=%d access_denied=%d password_protected=%d",
		foundCount, uniqueCount, duplicateCount, parseFailedCount, accessDeniedCount, passwordProtectedCount)

	if !foundSymlink {
		t.Errorf("expected at least one symlink observation")
	}
	if !foundBundle || len(bundlePositions) != 3 {
		t.Errorf("expected 3 bundle positions, got %v", bundlePositions)
	}
	if !foundPrivKeyFlag {
		t.Errorf("expected cert_and_key.pem to be flagged contains_private_key")
	}
	if parseFailedCount == 0 {
		t.Errorf("expected at least 1 parse_failed (e.g. corrupt.pem)")
	}
	if accessDeniedCount == 0 {
		t.Errorf("expected at least 1 access_denied (e.g. restricted_dir)")
	}
	if passwordProtectedCount == 0 {
		t.Errorf("expected at least 1 password_protected (e.g. password_protected.p12)")
	}
	if duplicateCount == 0 {
		t.Errorf("expected at least 1 duplicate certificate observation")
	}

	// Assert determinism: second scan produces identical results
	res2, err := Scan([]string{cleanCorpus}, now)
	if err != nil {
		t.Fatalf("second scan error: %v", err)
	}
	if len(res.Observations) != len(res2.Observations) {
		t.Errorf("expected deterministic observation counts: %d vs %d", len(res.Observations), len(res2.Observations))
	}
	for i := range res.Observations {
		if res.Observations[i].Path != res2.Observations[i].Path ||
			res.Observations[i].ChainPosition != res2.Observations[i].ChainPosition {
			t.Errorf("observation order non-deterministic at index %d: %s#%d vs %s#%d",
				i, res.Observations[i].Path, res.Observations[i].ChainPosition,
				res2.Observations[i].Path, res2.Observations[i].ChainPosition)
		}
	}
}
