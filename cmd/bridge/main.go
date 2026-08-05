// Command bridge is the WhatsApp worker pool. One process owns many
// whatsmeow clients (one per tenant whose worker_id matches this process).
//
// Phase 1: connects Postgres + Redis, owns the whatsmeow client manager,
// serves the APIToBridge gRPC service (SendMessage / StreamQR / StartLink),
// dials cmd/api as a gRPC client to forward inbound events, polls the
// Redis delay queue for scheduled replies, and snapshots device state
// to S3 on shutdown.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/amnasahamed/niosengine/internal/config"
	"github.com/amnasahamed/niosengine/internal/db"
	"github.com/amnasahamed/niosengine/internal/logging"
	"github.com/amnasahamed/niosengine/internal/queue"
	"github.com/amnasahamed/niosengine/internal/whatsapp"
	bridgepb "github.com/amnasahamed/niosengine/internal/bridgepb"
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
	if cfg.BridgeWorkerID == "" {
		return fmt.Errorf("BRIDGE_WORKER_ID is required for cmd/bridge (set to a unique id like hostname or ECS task ARN)")
	}

	log := logging.New(cfg.Env, cfg.LogLevel).With(
		slog.String("worker_id", cfg.BridgeWorkerID),
	)
	log.Info("bridge starting",
		slog.String("env", cfg.Env),
		slog.Int("max_sessions", cfg.BridgeMaxSessions),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Phase-1: connect Postgres (pgx pool) — load assigned sessions.
	bootCtx, bootCancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := db.Open(bootCtx, db.Config{DatabaseURL: cfg.DatabaseURL})
	bootCancel()
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer pool.Close()
	log.Info("postgres connected")

	// Phase-1: connect Redis (go-redis) — pop due reply schedule.
	redisClient, err := queue.NewClient(cfg.RedisURL, 5*time.Second)
	if err != nil {
		return fmt.Errorf("redis client: %w", err)
	}
	defer redisClient.Close()
	q := queue.NewRedisQueue(redisClient)
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := q.Ping(pingCtx); err != nil {
		pingCancel()
		return fmt.Errorf("redis ping: %w", err)
	}
	pingCancel()
	log.Info("redis connected")

	// Phase-1: start whatsmeow client manager.
	// Device state (signal keys, app state) lives in per-session SQLite files
	// under this dir. It is the ONLY durable copy — there is no S3 backup — so
	// it must be a persistent volume, never /tmp, or every tenant has to
	// re-scan a QR code after each restart.
	stateDir := filepath.Join(cfg.BridgeStateDir, cfg.BridgeWorkerID)
	mgr, err := whatsapp.NewManager(pool, whatsapp.ManagerConfig{
		WorkerID:    cfg.BridgeWorkerID,
		MaxSessions: cfg.BridgeMaxSessions,
		StateDir:    stateDir,
		Logger:      log,
	})
	if err != nil {
		return fmt.Errorf("new manager: %w", err)
	}

	// Start gRPC client to cmd/api (for ReportInbound inbound events).
	// Phase 1a: target comes from cfg.APIGRPCTarget (default "api:8081"),
	// NOT from cfg.APIListenAddr — that was the old code path and it dialed
	// gRPC over the API's HTTP port, which always failed.
	apiAddr := cfg.APIGRPCTarget
	apiCli, err := newAPIClient(ctx, apiAddr, cfg.GRPCSharedSecret)
	if err != nil {
		return fmt.Errorf("api client: %w", err)
	}
	defer apiCli.Close()
	log.Info("api gRPC client configured", slog.String("addr", apiAddr))

	// Phase 1b: hook the manager's event handler to the api client so
	// inbound whatsmeow events become ReportInbound RPCs. Order matters —
	// the reporter must be installed before mgr.Start so the handler can
	// dispatch the first message it sees.
	mgr.SetInboundReporter(newInboundReporter(apiCli))

	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("manager start: %w", err)
	}
	log.Info("whatsmeow manager started", slog.String("state_dir", stateDir))

	// Phase-1: start gRPC server for cmd/api (APIToBridge: SendMessage / StreamQR / StartLink).
	bridgeSvc := newBridgeService(mgr)
	grpcServer := grpc.NewServer()
	bridgepb.RegisterAPIToBridgeServer(grpcServer, bridgeSvc)
	reflection.Register(grpcServer)

	grpcLis, err := net.Listen("tcp", cfg.BridgeGRPCAddr)
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}
	go func() {
		log.Info("bridge gRPC server listening", slog.String("addr", grpcLis.Addr().String()))
		if err := grpcServer.Serve(grpcLis); err != nil {
			log.Error("grpc server failed", slog.String("err", err.Error()))
		}
	}()

	// Phase-1: start the Redis delay-queue poller (drains due replies).
	poller := newQueuePoller(q, mgr, log)
	go poller.Run(ctx)

	// HTTP /healthz + /readyz (unchanged shape, readyz now checks the manager).
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler(log))
	mux.HandleFunc("/readyz", readyzHandler(log, mgr))

	httpAddr := os.Getenv("BRIDGE_HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":9091"
	}
	srv := &http.Server{
		Addr:         httpAddr,
		Handler:      mux,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("bridge health server listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	}

	// Phase-1: gracefully stop gRPC, manager (snapshot to S3), and HTTP.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownGracePeriod)
	defer shutdownCancel()

	grpcServer.GracefulStop()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown failed", slog.String("err", err.Error()))
	}
	if err := mgr.Stop(shutdownCtx); err != nil {
		log.Error("manager stop failed", slog.String("err", err.Error()))
	}
	log.Info("bridge stopped cleanly")
	return nil
}

func healthzHandler(log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      true,
			"service": "bridge",
			"worker":  os.Getenv("BRIDGE_WORKER_ID"),
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// readyz reports whether at least one whatsmeow session is connected.
// During the first ~minute after a fresh deploy (before any tenant has
// linked WhatsApp), /readyz returns 503 so ECS doesn't route traffic
// at the load-balancer level.
func readyzHandler(log *slog.Logger, mgr whatsapp.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ready := mgr != nil && mgr.HasAuthenticatedSession()
		body := map[string]any{
			"service": "bridge",
			"ready":   ready,
		}
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			body["reason"] = "no authenticated sessions yet"
		}
		_ = json.NewEncoder(w).Encode(body)
	}
}