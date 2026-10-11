package tlsscan

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"sds/agent/internal/certinfo"
)

// ProbeEndpoint probes a remote TLS 1.3 endpoint for its leaf certificate.
// Per docs/02 and prompt: AFTER capture ends (so probes never appear in the capture),
// for each distinct (ip, port, sni) that completed a handshake but has no peer cert (TLS 1.3),
// perform tls.Dial with a 3s timeout.
//
// InsecureSkipVerify is acceptable and necessary here because:
// 1. We are a cryptographic discovery and inventory tool collecting observational metadata,
//    not an authenticating client application.
// 2. We must discover and inspect invalid, self-signed, untrusted, or expired certificates
//    without failing the handshake (which authenticating clients would do).
func ProbeEndpoint(ip string, port int, sni string) (*certinfo.Certificate, *string, error) {
	addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))

	serverName := sni
	if serverName == "" {
		serverName = ip
	}

	dialer := &net.Dialer{
		Timeout: 3 * time.Second,
	}

	conf := &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true, // We collect metadata; we do not authenticate
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", addr, conf)
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, nil, fmt.Errorf("no peer certificates returned")
	}

	leaf := state.PeerCertificates[0]
	sum := sha256.Sum256(leaf.Raw)
	fp := hex.EncodeToString(sum[:])

	cert := certinfo.ExtractCertificate(leaf)
	return cert, &fp, nil
}
