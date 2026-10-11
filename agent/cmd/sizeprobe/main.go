package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"golang.org/x/sys/unix"
	"software.sslmate.com/src/go-pkcs12"
)

func main() {
	// log/slog
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("sizeprobe baseline")

	// net/http: start server on 127.0.0.1:0 and client call
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	server := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		logger.Error("listen error", "err", err)
		return
	}
	defer ln.Close()

	go func() {
		_ = server.Serve(ln)
	}()
	defer server.Close()

	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/health", ln.Addr().String()))
	if err == nil {
		defer resp.Body.Close()
		var res map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&res)
		logger.Info("http call result", "status", res["status"])
	}

	// crypto/x509
	pool, err := x509.SystemCertPool()
	if err == nil && pool != nil {
		logger.Info("cert pool initialized")
	}
	_, _ = x509.ParseCertificate([]byte{0x30, 0x00})

	// crypto/tls (tls.Dial usage)
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", "127.0.0.1:0", tlsConfig)
	if err == nil && conn != nil {
		_ = conn.Close()
	}

	// encoding/json
	payload, _ := json.Marshal(map[string]any{"timestamp": time.Now().Unix()})
	var decoded map[string]any
	_ = json.Unmarshal(payload, &decoded)

	// golang.org/x/sys/unix (AF_PACKET raw socket probe)
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(unix.ETH_P_ALL))
	if err == nil {
		_ = unix.Close(fd)
	}

	// software.sslmate.com/src/go-pkcs12
	_, _, _ = pkcs12.Decode([]byte{}, "")

	logger.Info("sizeprobe complete")
}
