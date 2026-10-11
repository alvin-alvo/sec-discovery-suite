package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	apihttp "sds/api/internal/http"
	"sds/api/internal/runs"
	"sds/api/internal/scheduler"
	"sds/api/internal/store"
)

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	dbPath := getEnv("SDS_DB", "data/sds.duckdb")
	agentAddr := getEnv("SDS_AGENT_ADDR", "http://127.0.0.1:7471")
	token := os.Getenv("SDS_TOKEN")
	if token == "" {
		for _, p := range []string{".sds_token", "data/.sds_token"} {
			if b, err := os.ReadFile(p); err == nil {
				t := strings.TrimSpace(string(b))
				if t != "" {
					token = t
					slog.Info("Loaded authentication token from file", "path", p)
					break
				}
			}
		}
	}
	if token == "" {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			slog.Error("Failed to generate random token", "err", err)
			os.Exit(1)
		}
		token = hex.EncodeToString(b[:])
		slog.Info("SDS_TOKEN unset; generated random token for session", "token", token)
	}
	if _, err := os.Stat("data"); err == nil {
		_ = os.WriteFile("data/.sds_token", []byte(token), 0644)
	}
	if _, err := os.Stat("bin"); err == nil {
		_ = os.WriteFile(".sds_token", []byte(token), 0644)
	}

	callbackBase := getEnv("SDS_CALLBACK_BASE", "http://127.0.0.1:8080")
	bindAddr := getEnv("SDS_API_ADDR", "")
	if bindAddr == "" {
		bindAddr = getEnv("SDS_BIND_ADDR", "127.0.0.1:8080")
	}
	if port := os.Getenv("PORT"); port != "" && (bindAddr == "127.0.0.1:8080" || bindAddr == ":8080") {
		bindAddr = "127.0.0.1:" + port
	}

	slog.Info("Starting sec-discovery-suite backend",
		"db", dbPath,
		"agent_addr", agentAddr,
		"bind_addr", bindAddr,
		"callback_base", callbackBase,
	)

	st, err := store.Open(dbPath)
	if err != nil {
		slog.Error("Failed to open DuckDB store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	disp := runs.NewDispatcher(st, agentAddr, token, callbackBase)

	sched := scheduler.NewScheduler(st, disp, 5*time.Second)
	sched.Start()
	defer sched.Stop()

	router := apihttp.NewRouter(apihttp.ServerConfig{
		Store:        st,
		Dispatcher:   disp,
		Token:        token,
		AgentAddr:    agentAddr,
		CallbackBase: callbackBase,
	})

	server := &http.Server{
		Addr:              bindAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Handle graceful shutdown
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stopCh
		slog.Info("Shutdown signal received, shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("Server shutdown failed", "err", err)
		}
	}()

	slog.Info("Server listening", "addr", bindAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Server listen failed", "err", err)
		os.Exit(1)
	}
	slog.Info("Server stopped cleanly")
}
