package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"sds/agent/internal/fsscan"
	"sds/agent/internal/server"
	"sds/agent/internal/tlsscan"
)

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage:
  sds-agent scan fs [--roots a,b,c] [--json]
  sds-agent scan tls [--duration 30s] [--json]
  sds-agent serve   [--addr 127.0.0.1:7471]
`)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "scan":
		if len(os.Args) < 3 {
			printUsage()
			os.Exit(1)
		}
		scanType := os.Args[2]
		switch scanType {
		case "fs":
			fsCmd := flag.NewFlagSet("scan fs", flag.ExitOnError)
			rootsFlag := fsCmd.String("roots", "", "Comma-separated scan roots")
			jsonFlag := fsCmd.Bool("json", false, "Output newline-delimited JSON")
			_ = fsCmd.Parse(os.Args[3:])

			var roots []string
			if *rootsFlag != "" {
				for _, r := range strings.Split(*rootsFlag, ",") {
					r = strings.TrimSpace(r)
					if r != "" {
						roots = append(roots, r)
					}
				}
			}

			res, err := fsscan.Scan(roots, time.Now())
			if err != nil {
				fmt.Fprintf(os.Stderr, "scan error: %v\n", err)
				os.Exit(1)
			}

			if *jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				for _, obs := range res.Observations {
					if err := enc.Encode(obs); err != nil {
						fmt.Fprintf(os.Stderr, "json encode error: %v\n", err)
					}
				}
			} else {
				for _, obs := range res.Observations {
					fp := "none"
					algo := "none"
					if obs.Certificate != nil {
						fp = obs.Certificate.FingerprintSHA256
						algo = obs.Certificate.PubkeyAlgo
					}
					val := "none"
					if obs.ValidityStatus != nil {
						val = *obs.ValidityStatus
					}
					formatStr := "none"
					if obs.Format != nil {
						formatStr = *obs.Format
					}
					fmt.Printf("%s | %s | status=%s | fp=%s | chain=%d/%d | val=%s | algo=%s\n",
						obs.Path, formatStr, obs.ParseStatus, fp, obs.ChainPosition, obs.ChainLength, val, algo)
				}
			}

		case "tls":
			tlsCmd := flag.NewFlagSet("scan tls", flag.ExitOnError)
			durationFlag := tlsCmd.Duration("duration", 30*time.Second, "Scan duration")
			jsonFlag := tlsCmd.Bool("json", false, "Output newline-delimited JSON")
			_ = tlsCmd.Parse(os.Args[3:])

			ctx, cancel := context.WithTimeout(context.Background(), *durationFlag)
			defer cancel()

			var observations []*tlsscan.TLSObservation
			var obsMu sync.Mutex

			sniffer := tlsscan.NewAFPacketSniffer()
			err := sniffer.Run(ctx, func(obs *tlsscan.TLSObservation) {
				obsMu.Lock()
				observations = append(observations, obs)
				obsMu.Unlock()
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "tls scan error: %v\n", err)
				os.Exit(1)
			}

			// Active probe for TLS 1.3 handshakes without peer cert (cap 20 probes/scan)
			probedEndpoints := make(map[string]bool)
			probeCount := 0
			for _, obs := range observations {
				if obs.PeerCertFP == nil && obs.Outcome != nil && *obs.Outcome == "established" && obs.RemoteIP != nil && obs.RemotePort > 0 {
					epKey := fmt.Sprintf("%s:%d", *obs.RemoteIP, obs.RemotePort)
					if probedEndpoints[epKey] || probeCount >= 20 {
						continue
					}
					probedEndpoints[epKey] = true
					probeCount++
					sni := ""
					if obs.SNI != nil {
						sni = *obs.SNI
					}
					leafCert, fp, err := tlsscan.ProbeEndpoint(*obs.RemoteIP, obs.RemotePort, sni)
					if err == nil && fp != nil {
						src := "probed"
						obs.PeerCertFP = fp
						obs.PeerCertSource = &src
						obs.PeerCertificate = leafCert
					}
				}
			}

			if *jsonFlag {
				enc := json.NewEncoder(os.Stdout)
				for _, obs := range observations {
					if err := enc.Encode(obs); err != nil {
						fmt.Fprintf(os.Stderr, "json encode error: %v\n", err)
					}
				}
			} else {
				for _, obs := range observations {
					sni := "none"
					if obs.SNI != nil {
						sni = *obs.SNI
					}
					ver := "none"
					if obs.TLSVersion != nil {
						ver = *obs.TLSVersion
					}
					cipher := "none"
					if obs.CipherSuite != nil {
						cipher = *obs.CipherSuite
					}
					fp := "none"
					if obs.PeerCertFP != nil {
						fp = *obs.PeerCertFP
					}
					src := "unavailable"
					if obs.PeerCertSource != nil {
						src = *obs.PeerCertSource
					}
					proc := "unavailable"
					if obs.Process != nil && obs.Process.Comm != nil {
						proc = *obs.Process.Comm
					}
					lip := "unknown"
					if obs.LocalIP != nil {
						lip = *obs.LocalIP
					}
					rip := "unknown"
					if obs.RemoteIP != nil {
						rip = *obs.RemoteIP
					}
					fmt.Printf("%s:%d -> %s:%d | %s | %s | sni=%s | proc=%s | cert=%s (%s) | events=%d\n",
						lip, obs.LocalPort, rip, obs.RemotePort,
						ver, cipher, sni, proc, fp, src, obs.RecordCount)
				}
			}

		default:
			printUsage()
			os.Exit(1)
		}

	case "serve":
		serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
		defaultAddr := os.Getenv("SDS_AGENT_ADDR")
		if defaultAddr == "" {
			defaultAddr = "127.0.0.1:7471"
		}
		addrFlag := serveCmd.String("addr", defaultAddr, "Address to listen on")
		_ = serveCmd.Parse(os.Args[2:])

		token := os.Getenv("SDS_TOKEN")
		srv := server.NewServer(*addrFlag, token, slog.Default())
		if err := srv.ListenAndServe(); err != nil {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}

	default:
		printUsage()
		os.Exit(1)
	}
}
