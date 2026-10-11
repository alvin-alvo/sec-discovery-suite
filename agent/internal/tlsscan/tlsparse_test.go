package tlsscan

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"
)

type recordConn struct {
	net.Conn
	mu   sync.Mutex
	sent []byte
	recv []byte
}

func (c *recordConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if n > 0 {
		c.mu.Lock()
		c.recv = append(c.recv, b[:n]...)
		c.mu.Unlock()
	}
	return
}

func (c *recordConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	if n > 0 {
		c.mu.Lock()
		c.sent = append(c.sent, b[:n]...)
		c.mu.Unlock()
	}
	return
}

func generateTestCertificate(t *testing.T) (tls.Certificate, string) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey failed: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test.local",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		DNSNames:              []string{"test.local"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate failed: %v", err)
	}

	fp := sha256.Sum256(derBytes)
	fpHex := hex.EncodeToString(fp[:])

	cert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}

	return cert, fpHex
}

func TestTLSParser_TLS12(t *testing.T) {
	cert, expectedFP := generateTestCertificate(t)

	serverConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MaxVersion:   tls.VersionTLS12,
		MinVersion:   tls.VersionTLS12,
	}

	clientConfig := &tls.Config{
		ServerName:         "test.local",
		InsecureSkipVerify: true,
		MaxVersion:         tls.VersionTLS12,
		MinVersion:         tls.VersionTLS12,
	}

	c1, c2 := net.Pipe()
	clientConn := &recordConn{Conn: c1}
	serverConn := &recordConn{Conn: c2}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		tlsClient := tls.Client(clientConn, clientConfig)
		_ = tlsClient.Handshake()
		_, _ = tlsClient.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
		_ = tlsClient.Close()
	}()

	go func() {
		defer wg.Done()
		tlsServer := tls.Server(serverConn, serverConfig)
		_ = tlsServer.Handshake()
		var buf [128]byte
		_, _ = tlsServer.Read(buf[:])
		_ = tlsServer.Close()
	}()

	wg.Wait()

	// Parse client -> server stream
	clientReasm := NewFlowReassembler()
	clientReasm.Append(clientConn.sent)

	// Parse server -> client stream
	serverReasm := NewFlowReassembler()
	serverReasm.Append(serverConn.sent)

	state := NewTLSConnectionState()
	state.FeedBytes(clientReasm)
	state.FeedBytes(serverReasm)

	if state.TLSVersion == nil || *state.TLSVersion != "TLS1.2" {
		t.Errorf("expected TLS1.2, got %v", state.TLSVersion)
	}
	if state.SNI == nil || *state.SNI != "test.local" {
		t.Errorf("expected SNI test.local, got %v", state.SNI)
	}
	if state.CipherSuite == nil {
		t.Errorf("expected cipher suite to be present")
	}
	if state.PeerCertFP == nil || *state.PeerCertFP != expectedFP {
		t.Errorf("expected peer cert FP %s, got %v", expectedFP, state.PeerCertFP)
	}
	if state.PeerCertSource == nil || *state.PeerCertSource != "observed" {
		t.Errorf("expected peer cert source observed, got %v", state.PeerCertSource)
	}
	if state.RecordCount < 2 {
		t.Errorf("expected at least 2 records, got %d", state.RecordCount)
	}
}

func TestTLSParser_TLS13(t *testing.T) {
	cert, _ := generateTestCertificate(t)

	serverConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MaxVersion:   tls.VersionTLS13,
		MinVersion:   tls.VersionTLS13,
	}

	clientConfig := &tls.Config{
		ServerName:         "test13.local",
		InsecureSkipVerify: true,
		MaxVersion:         tls.VersionTLS13,
		MinVersion:         tls.VersionTLS13,
	}

	c1, c2 := net.Pipe()
	clientConn := &recordConn{Conn: c1}
	serverConn := &recordConn{Conn: c2}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		tlsClient := tls.Client(clientConn, clientConfig)
		_ = tlsClient.Handshake()
		_, _ = tlsClient.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
		_ = tlsClient.Close()
	}()

	go func() {
		defer wg.Done()
		tlsServer := tls.Server(serverConn, serverConfig)
		_ = tlsServer.Handshake()
		var buf [128]byte
		_, _ = tlsServer.Read(buf[:])
		_ = tlsServer.Close()
	}()

	wg.Wait()

	clientReasm := NewFlowReassembler()
	clientReasm.Append(clientConn.sent)

	serverReasm := NewFlowReassembler()
	serverReasm.Append(serverConn.sent)

	state := NewTLSConnectionState()
	state.FeedBytes(clientReasm)
	state.FeedBytes(serverReasm)

	if state.TLSVersion == nil || *state.TLSVersion != "TLS1.3" {
		t.Errorf("expected TLS1.3, got %v", state.TLSVersion)
	}
	if state.SNI == nil || *state.SNI != "test13.local" {
		t.Errorf("expected SNI test13.local, got %v", state.SNI)
	}
	if state.CipherSuite == nil {
		t.Errorf("expected cipher suite to be present")
	}
	// In TLS 1.3, Certificate is encrypted so peer cert is unavailable without active probe
	if state.PeerCertFP != nil {
		t.Errorf("expected peer cert FP to be nil in TLS 1.3 passive capture, got %v", *state.PeerCertFP)
	}
	if state.Outcome != "established" {
		t.Errorf("expected outcome established, got %s", state.Outcome)
	}
}
