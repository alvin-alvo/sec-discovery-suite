package certinfo

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"time"
)

// Certificate represents the docs/06 certificate wire format.
type Certificate struct {
	FingerprintSHA256 string   `json:"fingerprint_sha256"`
	Serial            string   `json:"serial"`
	SubjectDN         string   `json:"subject_dn"`
	SubjectCN         string   `json:"subject_cn"`
	IssuerDN          string   `json:"issuer_dn"`
	IssuerCN          string   `json:"issuer_cn"`
	SAN               []string `json:"san"`
	NotBefore         string   `json:"not_before"`
	NotAfter          string   `json:"not_after"`
	PubkeyAlgo        string   `json:"pubkey_algo"`
	PubkeyBits        *int     `json:"pubkey_bits"`
	Curve             *string  `json:"curve"`
	SigAlgo           string   `json:"sig_algo"`
	IsCA              bool     `json:"is_ca"`
	IsSelfSigned      bool     `json:"is_self_signed"`
	PQCClass          string   `json:"pqc_class"`
}

// ExtractCertificate converts an x509.Certificate to the docs/06 certificate model.
func ExtractCertificate(c *x509.Certificate) *Certificate {
	if c == nil {
		return nil
	}

	sum := sha256.Sum256(c.Raw)
	fp := hex.EncodeToString(sum[:])

	var sans []string
	for _, name := range c.DNSNames {
		sans = append(sans, "dns:"+name)
	}
	for _, ip := range c.IPAddresses {
		sans = append(sans, "ip:"+ip.String())
	}
	for _, email := range c.EmailAddresses {
		sans = append(sans, "email:"+email)
	}
	for _, uri := range c.URIs {
		sans = append(sans, "uri:"+uri.String())
	}

	pubAlgo := "unknown"
	var pubBits *int
	var curve *string

	switch k := c.PublicKey.(type) {
	case *rsa.PublicKey:
		pubAlgo = "RSA"
		bits := k.N.BitLen()
		pubBits = &bits
	case *ecdsa.PublicKey:
		pubAlgo = "ECDSA"
		bits := k.Curve.Params().BitSize
		pubBits = &bits
		name := k.Curve.Params().Name
		curve = &name
	case ed25519.PublicKey:
		pubAlgo = "Ed25519"
		bits := 256
		pubBits = &bits
	}

	pqc := "unknown"
	if pubAlgo == "RSA" || pubAlgo == "ECDSA" || pubAlgo == "Ed25519" {
		pqc = "classical"
	}

	selfSigned := bytes.Equal(c.RawSubject, c.RawIssuer) && c.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature) == nil

	return &Certificate{
		FingerprintSHA256: fp,
		Serial:            c.SerialNumber.String(),
		SubjectDN:         c.Subject.String(),
		SubjectCN:         c.Subject.CommonName,
		IssuerDN:          c.Issuer.String(),
		IssuerCN:          c.Issuer.CommonName,
		SAN:               sans,
		NotBefore:         c.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:          c.NotAfter.UTC().Format(time.RFC3339),
		PubkeyAlgo:        pubAlgo,
		PubkeyBits:        pubBits,
		Curve:             curve,
		SigAlgo:           c.SignatureAlgorithm.String(),
		IsCA:              c.IsCA,
		IsSelfSigned:      selfSigned,
		PQCClass:          pqc,
	}
}

// ExpiryInfo calculates days_to_expiry and validity_status relative to now.
// validity_status is "valid", "expiring" (<= 30 days remaining), or "expired".
func ExpiryInfo(c *x509.Certificate, now time.Time) (int, string) {
	if c == nil {
		return 0, "unknown"
	}

	diff := c.NotAfter.Sub(now)
	days := int(diff.Hours() / 24)

	if !now.Before(c.NotAfter) {
		return days, "expired"
	}
	if days <= 30 {
		return days, "expiring"
	}
	return days, "valid"
}
