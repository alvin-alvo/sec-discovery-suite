package main

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: pemprobe <path>")
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Args[1])
	n := 0
	for b, r := pem.Decode(data); b != nil; b, r = pem.Decode(r) {
		if b.Type == "CERTIFICATE" {
			if c, err := x509.ParseCertificate(b.Bytes); err == nil {
				k := "unknown"
				if r, ok := c.PublicKey.(*rsa.PublicKey); ok {
					k = fmt.Sprintf("RSA-%d", r.N.BitLen())
				} else if e, ok := c.PublicKey.(*ecdsa.PublicKey); ok {
					k = e.Curve.Params().Name
				}
				fmt.Printf("%x | %s | %s | %s | %s | %s\n", sha256.Sum256(c.Raw), c.Subject, c.Issuer, c.NotAfter.Format(time.RFC3339), c.PublicKeyAlgorithm, k)
				n++
			}
		}
	}
	if err != nil || n == 0 {
		fmt.Fprintf(os.Stderr, "read error or no certs: %v\n", err)
		os.Exit(1)
	}
}
