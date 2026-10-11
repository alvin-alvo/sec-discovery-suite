package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

func main() {
	targetDir := ".test-certs/corpus"
	if len(os.Args) > 1 {
		targetDir = os.Args[1]
	}

	// Idempotent: allow cleanup of previously restricted dirs
	_ = os.Chmod(filepath.Join(targetDir, "restricted_dir"), 0755)
	_ = os.RemoveAll(targetDir)

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create target dir: %v\n", err)
		os.Exit(1)
	}

	now := time.Now()

	// 1. Valid RSA-2048 PEM
	rsa2048Key, _ := rsa.GenerateKey(rand.Reader, 2048)
	rsa2048DER := createCert(rsa2048Key, &rsa2048Key.PublicKey, "valid-rsa2048.local", now.Add(-24*time.Hour), now.Add(365*24*time.Hour), false, nil, nil)
	rsa2048PEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rsa2048DER})
	_ = os.WriteFile(filepath.Join(targetDir, "valid_rsa2048.pem"), rsa2048PEM, 0644)

	// 2. Valid EC P-256 PEM
	ecKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecDER := createCert(ecKey, &ecKey.PublicKey, "valid-ecp256.local", now.Add(-24*time.Hour), now.Add(365*24*time.Hour), false, nil, nil)
	ecPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ecDER})
	_ = os.WriteFile(filepath.Join(targetDir, "valid_ecp256.pem"), ecPEM, 0644)

	// 3. Expired cert (expired 30 days ago)
	expiredKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	expiredDER := createCert(expiredKey, &expiredKey.PublicKey, "expired.local", now.Add(-400*24*time.Hour), now.Add(-30*24*time.Hour), false, nil, nil)
	_ = os.WriteFile(filepath.Join(targetDir, "expired.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: expiredDER}), 0644)

	// 4. Cert expiring in 10 days
	expiringKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	expiringDER := createCert(expiringKey, &expiringKey.PublicKey, "expiring-10d.local", now.Add(-300*24*time.Hour), now.Add(10*24*time.Hour), false, nil, nil)
	_ = os.WriteFile(filepath.Join(targetDir, "expiring_10d.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: expiringDER}), 0644)

	// 5. Weak RSA-1024 cert
	rsa1024Key, _ := rsa.GenerateKey(rand.Reader, 1024)
	rsa1024DER := createCert(rsa1024Key, &rsa1024Key.PublicKey, "weak-rsa1024.local", now.Add(-24*time.Hour), now.Add(365*24*time.Hour), false, nil, nil)
	_ = os.WriteFile(filepath.Join(targetDir, "weak_rsa1024.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rsa1024DER}), 0644)

	// 6. DER copy of RSA-2048
	_ = os.WriteFile(filepath.Join(targetDir, "copy_rsa2048.der"), rsa2048DER, 0644)

	// 7. 3-cert bundle (Root CA -> Intermediate CA -> Leaf)
	rootKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	rootDER := createCert(rootKey, &rootKey.PublicKey, "root-ca.local", now.Add(-24*time.Hour), now.Add(3650*24*time.Hour), true, nil, nil)
	rootCert, _ := x509.ParseCertificate(rootDER)

	interKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	interDER := createCert(rootKey, &interKey.PublicKey, "intermediate-ca.local", now.Add(-24*time.Hour), now.Add(1800*24*time.Hour), true, rootCert, rootCert)
	interCert, _ := x509.ParseCertificate(interDER)

	leafKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	leafDER := createCert(interKey, &leafKey.PublicKey, "bundle-leaf.local", now.Add(-24*time.Hour), now.Add(365*24*time.Hour), false, interCert, rootCert)

	bundleBytes := append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		append(
			pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: interDER}),
			pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})...,
		)...,
	)
	_ = os.WriteFile(filepath.Join(targetDir, "bundle_3cert.pem"), bundleBytes, 0644)

	// 8. Exact duplicate in another dir
	subDir := filepath.Join(targetDir, "subdir")
	_ = os.MkdirAll(subDir, 0755)
	_ = os.WriteFile(filepath.Join(subDir, "duplicate_rsa2048.pem"), rsa2048PEM, 0644)

	// 9. Symlink to a cert
	symlinkPath := filepath.Join(targetDir, "symlink_ecp256.pem")
	_ = os.Symlink("valid_ecp256.pem", symlinkPath)

	// 10. Cert + Private Key PEM
	privKeyBytes := x509.MarshalPKCS1PrivateKey(rsa2048Key)
	privKeyBlock := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privKeyBytes})
	certAndKey := append(privKeyBlock, rsa2048PEM...)
	_ = os.WriteFile(filepath.Join(targetDir, "cert_and_key.pem"), certAndKey, 0600)

	// 11. Corrupt PEM
	corruptBytes := []byte("-----BEGIN CERTIFICATE-----\nInvalidNonBase64!!!Data\n-----END CERTIFICATE-----\n")
	_ = os.WriteFile(filepath.Join(targetDir, "corrupt.pem"), corruptBytes, 0644)

	// 12. PKCS12 with empty password and one with password
	rsaCert, _ := x509.ParseCertificate(rsa2048DER)
	pfxEmpty, err := pkcs12.Encode(rand.Reader, rsa2048Key, rsaCert, nil, "")
	if err == nil {
		_ = os.WriteFile(filepath.Join(targetDir, "unprotected.p12"), pfxEmpty, 0644)
	}

	pfxPassword, err := pkcs12.Encode(rand.Reader, rsa2048Key, rsaCert, nil, "secret123")
	if err == nil {
		_ = os.WriteFile(filepath.Join(targetDir, "password_protected.p12"), pfxPassword, 0644)
	}

	// 13. Restricted directory (unreadable unprivileged => access_denied)
	restrictedDir := filepath.Join(targetDir, "restricted_dir")
	_ = os.MkdirAll(restrictedDir, 0755)
	_ = os.WriteFile(filepath.Join(restrictedDir, "hidden.pem"), rsa2048PEM, 0644)
	// Make restricted directory unreadable to unprivileged users (mode 0000)
	_ = os.Chmod(restrictedDir, 0000)

	fmt.Printf("Seed corpus successfully created at %s\n", targetDir)
}

func createCert(signer any, pubKey any, cn string, notBefore, notAfter time.Time, isCA bool, parent *x509.Certificate, root *x509.Certificate) []byte {
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	issuerCN := cn
	if parent != nil {
		issuerCN = parent.Subject.CommonName
	}
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: cn,
		},
		Issuer: pkix.Name{
			CommonName: issuerCN,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}
	if isCA {
		template.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	} else {
		template.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	}

	parentCert := template
	if parent != nil {
		parentCert = parent
	}

	der, err := x509.CreateCertificate(rand.Reader, template, parentCert, pubKey, signer)
	if err != nil {
		panic(err)
	}
	return der
}
