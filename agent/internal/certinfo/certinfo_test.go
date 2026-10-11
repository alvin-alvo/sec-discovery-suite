package certinfo

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"
)

func TestExtractCertificate(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa generate key: %v", err)
	}

	u, _ := url.Parse("https://example.com/cert")
	template := &x509.Certificate{
		SerialNumber: big.NewInt(123456),
		Subject: pkix.Name{
			CommonName: "test.local",
		},
		Issuer: pkix.Name{
			CommonName: "test.local",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour * 365),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
		DNSNames:              []string{"test.local", "alt.local"},
		IPAddresses:           []net.IP{net.ParseIP("192.168.1.1")},
		EmailAddresses:        []string{"admin@example.com"},
		URIs:                  []*url.URL{u},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	info := ExtractCertificate(cert)
	if info == nil {
		t.Fatal("expected non-nil cert info")
	}

	if len(info.FingerprintSHA256) != 64 {
		t.Errorf("expected 64-char hex fingerprint, got %s", info.FingerprintSHA256)
	}
	if info.SubjectCN != "test.local" {
		t.Errorf("expected SubjectCN test.local, got %s", info.SubjectCN)
	}
	if info.PubkeyAlgo != "RSA" {
		t.Errorf("expected PubkeyAlgo RSA, got %s", info.PubkeyAlgo)
	}
	if info.PubkeyBits == nil || *info.PubkeyBits != 2048 {
		t.Errorf("expected 2048 bits, got %v", info.PubkeyBits)
	}
	if !info.IsCA {
		t.Errorf("expected IsCA true")
	}
	if !info.IsSelfSigned {
		t.Errorf("expected IsSelfSigned true")
	}
	if info.PQCClass != "classical" {
		t.Errorf("expected pqc_class classical, got %s", info.PQCClass)
	}

	expectedSANs := map[string]bool{
		"dns:test.local":            true,
		"dns:alt.local":             true,
		"ip:192.168.1.1":            true,
		"email:admin@example.com":   true,
		"uri:https://example.com/cert": true,
	}
	if len(info.SAN) != len(expectedSANs) {
		t.Errorf("expected %d SANs, got %d: %v", len(expectedSANs), len(info.SAN), info.SAN)
	}
	for _, san := range info.SAN {
		if !expectedSANs[san] {
			t.Errorf("unexpected SAN: %s", san)
		}
	}
}

func TestExtractCertificate_ECDSA(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ec keygen: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(999),
		Subject:      pkix.Name{CommonName: "ec.local"},
		Issuer:       pkix.Name{CommonName: "ec.local"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour * 10),
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &ecKey.PublicKey, ecKey)
	if err != nil {
		t.Fatalf("create ec cert: %v", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse ec cert: %v", err)
	}

	info := ExtractCertificate(cert)
	if info.PubkeyAlgo != "ECDSA" {
		t.Errorf("expected ECDSA, got %s", info.PubkeyAlgo)
	}
	if info.Curve == nil || *info.Curve != "P-256" {
		t.Errorf("expected curve P-256, got %v", info.Curve)
	}
	if info.PQCClass != "classical" {
		t.Errorf("expected classical, got %s", info.PQCClass)
	}
}

func TestExpiryInfo(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

	// Valid (> 30 days)
	cValid := &x509.Certificate{NotAfter: now.Add(40 * 24 * time.Hour)}
	days, status := ExpiryInfo(cValid, now)
	if status != "valid" || days < 39 {
		t.Errorf("expected valid, got status=%s days=%d", status, days)
	}

	// Expiring (<= 30 days)
	cExpiring := &x509.Certificate{NotAfter: now.Add(15 * 24 * time.Hour)}
	days, status = ExpiryInfo(cExpiring, now)
	if status != "expiring" || days < 14 {
		t.Errorf("expected expiring, got status=%s days=%d", status, days)
	}

	// Expired
	cExpired := &x509.Certificate{NotAfter: now.Add(-5 * 24 * time.Hour)}
	days, status = ExpiryInfo(cExpired, now)
	if status != "expired" || days > 0 {
		t.Errorf("expected expired, got status=%s days=%d", status, days)
	}
}
