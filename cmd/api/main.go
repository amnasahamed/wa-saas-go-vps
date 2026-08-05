// Command api is the orchestrator: HTTP API for the dashboard, inbound
// webhook from the bridge, gRPC server for the bridge workers, and the
// background workers (destination sync retries, reply scheduling, stale-lead
// digests).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"

	bridgepb "github.com/amnasahamed/niosengine/internal/bridgepb"
	"github.com/amnasahamed/niosengine/internal/auth"
	"github.com/amnasahamed/niosengine/internal/config"
	"github.com/amnasahamed/niosengine/internal/db"
	"github.com/amnasahamed/niosengine/internal/destinations"
	"github.com/amnasahamed/niosengine/internal/grpcauth"
	"github.com/amnasahamed/niosengine/internal/leads"
	"github.com/amnasahamed/niosengine/internal/logging"
	"github.com/amnasahamed/niosengine/internal/messages"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logging.New(cfg.Env, cfg.LogLevel)
	log.Info("api starting",
		slog.String("env", cfg.Env),
		slog.String("listen_addr", cfg.APIListenAddr),
	)

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := db.Open(bootCtx, db.Config{DatabaseURL: cfg.DatabaseURL})
	bootCancel()
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer pool.Close()
	log.Info("postgres connected", slog.String("max_conns", "20"))

	// Wire repositories + services
	queries := db.New(pool)
	apiKeyValidator := auth.NewAPIKeyValidator(queries)
	messagesRepo := messages.NewPGRepository(queries)
	messagesSvc := messages.NewPGService(messagesRepo, log)
	leadsRepo := leads.NewPGRepository(queries)
	leadsSvc := leads.NewPGService(leadsRepo, log)
	destRepo := destinations.NewPGRepository(queries)
	destSvc := destinations.NewPGService(destRepo)

	// The dev fallback is a public constant in this repo, so anyone could forge
	// a session cookie with it. Tolerate it only in dev; refuse to boot without
	// a real secret anywhere else.
	jwtSecret := []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) < 32 {
		if !cfg.IsDev() {
			return fmt.Errorf("JWT_SECRET must be set to at least 32 bytes when ENV=%s "+
				"(generate one with: openssl rand -base64 48)", cfg.Env)
		}
		jwtSecret = []byte("dev-only-jwt-secret-please-set-JWT_SECRET-env-var-32b")
		log.Warn("using default JWT_SECRET — set JWT_SECRET env in production")
	}
	authSvc := auth.NewPGService(queries, jwtSecret, log)

	corsOrigins := strings.Split(getEnv("CORS_ORIGINS", "http://localhost:3000"), ",")

	mux := http.NewServeMux()

	// --- public health ---
	mux.HandleFunc("/healthz", healthzHandler(log))
	mux.HandleFunc("/readyz", readyzHandler(log, pool))

	// --- auth endpoints (public, cookie set on login) ---
	ad := &authDeps{Svc: authSvc, Queries: queries, Log: log, SecureCookie: cfg.IsProd()}
	mux.HandleFunc("/v1/auth/signup", handleSignup(ad))
	mux.HandleFunc("/v1/auth/login", handleLogin(ad))
	mux.HandleFunc("/v1/auth/logout", handleLogout(ad))
	mux.HandleFunc("/v1/auth/verify-email", handleVerifyEmail(ad))

	// --- authenticated dashboard endpoints ---
	dd := &dashboardDeps{
		Queries:     queries,
		AuthSvc:     authSvc,
		LeadsSvc:    leadsSvc,
		MessagesSvc: messagesSvc,
		DestSvc:     destSvc,
		Log:         log,
		BridgeCli:   newBridgeClient(cfg.BridgeGRPCTargets, cfg.GRPCSharedSecret, cfg.GRPCDialTimeout, log),
	}
	authMW := auth.RequireAuth(authSvc)

	mux.Handle("/v1/auth/me", authMW(http.HandlerFunc(handleMe(ad))))

	mux.Handle("/v1/me/workspace", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleGetWorkspace(dd).ServeHTTP(w, r)
		case http.MethodPatch:
			handlePatchWorkspace(dd).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or PATCH only")
		}
	})))
	mux.Handle("/v1/me/openai-key", authMW(http.HandlerFunc(handleSetOpenAIKey(dd))))
	mux.Handle("/v1/me/onboarding", authMW(http.HandlerFunc(handleOnboarding(dd))))
	mux.Handle("/v1/me/overview", authMW(http.HandlerFunc(handleOverview(dd))))
	mux.Handle("/v1/me/stage-usage", authMW(http.HandlerFunc(handleStageUsage(dd))))

	mux.Handle("/v1/me/api-keys", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListAPIKeys(dd).ServeHTTP(w, r)
		case http.MethodPost:
			handleMintAPIKey(dd).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or POST")
		}
	})))
	mux.Handle("/v1/me/api-keys/", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			handleRevokeAPIKey(dd).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "DELETE only")
		}
	})))

	mux.Handle("/v1/leads", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListLeads(dd).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET only")
		}
	})))
	mux.Handle("/v1/leads/", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/messages") && r.Method == http.MethodPost:
			handleSendLeadMessage(dd).ServeHTTP(w, r)
		case r.Method == http.MethodGet:
			handleGetLead(dd).ServeHTTP(w, r)
		case r.Method == http.MethodPatch:
			handlePatchLead(dd).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET, PATCH, or POST .../messages")
		}
	})))

	mux.Handle("/v1/destinations", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListDestinations(dd).ServeHTTP(w, r)
		case http.MethodPost:
			handleConnectDestination(dd).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or POST")
		}
	})))
	mux.Handle("/v1/destinations/", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			handlePatchDestination(dd).ServeHTTP(w, r)
		case http.MethodDelete:
			handleDeleteDestination(dd).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "PATCH or DELETE")
		}
	})))

	mux.Handle("/v1/whatsapp/session", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleGetWhatsAppSession(dd).ServeHTTP(w, r)
		case http.MethodPost:
			handlePostWhatsAppSession(dd).ServeHTTP(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or POST")
		}
	})))
	mux.Handle("/v1/whatsapp/session/refresh-qr", authMW(http.HandlerFunc(handlePostWhatsAppSession(dd))))

	mux.Handle("/v1/prompts/default", authMW(http.HandlerFunc(handleGetDefaultPrompt())))
	mux.Handle("/v1/prompts/preview", authMW(http.HandlerFunc(handlePreviewPrompt())))
	mux.Handle("/v1/prompts/versions", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []any{})
	})))

	// --- inbound webhook from bridge ---
	mux.HandleFunc("/v1/inbound", handleInbound(&inboundDeps{
		APIKey:   apiKeyValidator,
		Messages: messagesSvc,
		Leads:    leadsSvc,
		Queries:  queries,
		Log:      log,
	}))

	handler := logRequests(log, withCORS(corsOrigins, mux))

	srv := &http.Server{
		Addr:         cfg.APIListenAddr,
		Handler:      handler,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
	}

	// gRPC server for the bridge workers. Boots alongside HTTP. Both share
	// the same shutdown path below. A bridge-not-up is NOT a fatal error here
	// — the bridge may join later — so gRPC server start is async.
	grpcAuth := grpcauth.New(cfg.GRPCSharedSecret)
	grpcSrv := grpc.NewServer(
		grpc.UnaryInterceptor(grpcAuth.UnaryServerInterceptor()),
		grpc.StreamInterceptor(grpcAuth.StreamServerInterceptor()),
	)
	bridgepb.RegisterBridgeToAPIServer(grpcSrv, newAPIBridgeService(
		newInboundProcessor(inboundProcessorDeps{
			Queries:     queries,
			MessagesSvc: messagesSvc,
			LeadsSvc:    leadsSvc,
			Log:         log,
		}),
		log,
	))

	grpcListener, err := net.Listen("tcp", cfg.APIGRPCListenAddr)
	if err != nil {
		return fmt.Errorf("grpc listen %s: %w", cfg.APIGRPCListenAddr, err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Two server goroutines + an error channel big enough for both. Either
	// server failing short-circuits shutdown, but the other one is still
	// drained cleanly via the shared shutdown path.
	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Info("http server listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Info("grpc server listening", slog.String("addr", cfg.APIGRPCListenAddr))
		if err := grpcSrv.Serve(grpcListener); err != nil {
			errCh <- fmt.Errorf("grpc server: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		// One server died; cancel ctx so the other one drains through the
		// shared shutdown path. The original error is returned after cleanup.
		log.Error("server error, shutting down", slog.String("err", err.Error()))
		cancel()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownGracePeriod)
	defer shutdownCancel()

	// http.Server.Shutdown is context-aware; grpcSrv.GracefulStop is not.
	// Run them concurrently, fall back to hard close on deadline.
	var shutdownWG sync.WaitGroup
	shutdownWG.Add(2)
	go func() {
		defer shutdownWG.Done()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("http shutdown", slog.String("err", err.Error()))
		}
	}()
	go func() {
		defer shutdownWG.Done()
		stopped := make(chan struct{})
		go func() {
			grpcSrv.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-shutdownCtx.Done():
			log.Warn("grpc graceful stop timed out; forcing")
			grpcSrv.Stop()
		}
	}()
	shutdownWG.Wait()

	// Bridge gRPC clients last — handlers may have been mid-RPC when we began
	// draining. After this returns, no further outbound calls.
	dd.BridgeCli.Close()
	pool.Close()
	// Drain the server goroutines (they should already be done after Shutdown,
	// but make sure we don't leak them before returning).
	wg.Wait()
	log.Info("api stopped cleanly")
	return nil
}

// healthzHandler is the liveness probe.
func healthzHandler(log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      true,
			"service": "api",
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// readyzHandler is the readiness probe.
func readyzHandler(log *slog.Logger, pool *db.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		status := map[string]any{
			"service": "api",
			"checks":  map[string]any{},
		}
		checks := status["checks"].(map[string]any)
		allOK := true

		if err := pool.Ping(ctx); err != nil {
			checks["postgres"] = map[string]any{
				"ok":    false,
				"error": err.Error(),
			}
			log.Warn("readyz: postgres ping failed", slog.String("err", err.Error()))
			allOK = false
		} else {
			stats := pool.Stats()
			checks["postgres"] = map[string]any{
				"ok":             true,
				"acquired_conns": stats.AcquiredConns(),
				"idle_conns":     stats.IdleConns(),
				"total_conns":    stats.TotalConns(),
				"max_conns":      stats.MaxConns(),
			}
		}

		status["ok"] = allOK
		w.Header().Set("Content-Type", "application/json")
		if !allOK {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(status)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
