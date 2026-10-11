package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"sds/agent/internal/fsscan"
	"sds/agent/internal/server"
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
					fmt.Printf("%s | %s | status=%s | fp=%s | chain=%d/%d | val=%s | algo=%s\n",
						obs.Path, obs.Format, obs.ParseStatus, fp, obs.ChainPosition, obs.ChainLength, val, algo)
				}
			}

		case "tls":
			fmt.Fprintln(os.Stderr, "tls scanning not implemented yet")
			os.Exit(1)

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
