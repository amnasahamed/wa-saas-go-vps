package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/amnasahamed/niosengine/internal/grpcauth"
	bridgepb "github.com/amnasahamed/niosengine/internal/bridgepb"
)

// bridgeClient holds one grpc.ClientConn per bridge worker. Constructed
// lazily on first use of SendMessage(ctx, workerID, ...) so that the api
// does not block at boot when the bridge is not yet up.
//
// The shared secret is attached per-RPC via grpcauth.AppendToContext — see
// internal/grpcauth for why we do not use per-RPC credentials.
type bridgeClient struct {
	mu      sync.Mutex
	targets map[string]string
	conns   map[string]*grpc.ClientConn
	clients map[string]bridgepb.APIToBridgeClient
	secret  string
	dialTO  time.Duration
	log     *slog.Logger
}

func newBridgeClient(targets map[string]string, secret string, dialTimeout time.Duration, log *slog.Logger) *bridgeClient {
	if targets == nil {
		targets = make(map[string]string)
	}
	return &bridgeClient{
		targets: targets,
		conns:   make(map[string]*grpc.ClientConn),
		clients: make(map[string]bridgepb.APIToBridgeClient),
		secret:  secret,
		dialTO:  dialTimeout,
		log:     log,
	}
}

// ResolveTarget picks the configured gRPC target for the given worker ID
// from the targets map passed at construction. Returns an error if the
// worker is unknown — handler maps that to a 5xx and the dashboard surfaces
// "bridge worker not configured" so the operator knows to update
// BRIDGE_GRPC_TARGETS.
func (b *bridgeClient) ResolveTarget(workerID string) (string, error) {
	if b == nil {
		return "", fmt.Errorf("bridge client not configured")
	}
	if workerID == "" {
		return "", fmt.Errorf("worker_id is empty")
	}
	if target, ok := b.targets[workerID]; ok {
		return target, nil
	}
	return "", fmt.Errorf("bridge worker %q not configured (set BRIDGE_GRPC_TARGETS)", workerID)
}

// get returns the cached client for the worker, dialing on first use.
//
// The first-call dial uses grpc.NewClient (lazy — does not establish a
// connection until the first RPC). WithBlock is intentionally NOT used:
// the bridge may not be up when the api starts, and we do not want a
// missing bridge to refuse the api's boot. The first RPC itself fails
// fast (after dialTO) if the bridge is unreachable, and the handler maps
// that to a clear error for the dashboard.
func (b *bridgeClient) get(ctx context.Context, workerID, target string) (bridgepb.APIToBridgeClient, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c, ok := b.clients[workerID]; ok {
		return c, nil
	}
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", target, err)
	}
	c := bridgepb.NewAPIToBridgeClient(conn)
	b.conns[workerID] = conn
	b.clients[workerID] = c
	b.log.Info("bridge gRPC client configured",
		slog.String("worker_id", workerID),
		slog.String("target", target),
	)
	return c, nil
}

// SendMessage routes a request to the bridge worker that owns tenantID's
// session. workerID is the value of whatsapp_sessions.worker_id.
func (b *bridgeClient) SendMessage(ctx context.Context, workerID, target string, req *bridgepb.SendMessageRequest) (*bridgepb.SendMessageResponse, error) {
	c, err := b.get(ctx, workerID, target)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, b.dialTO)
	defer cancel()
	ctx, err = grpcauth.AppendToContext(ctx, b.secret)
	if err != nil {
		return nil, err
	}
	return c.SendMessage(ctx, req)
}

// StartLink is exposed for the dashboard linking flow.
func (b *bridgeClient) StartLink(ctx context.Context, workerID, target string, req *bridgepb.StreamQRRequest) (*bridgepb.LinkResult, error) {
	c, err := b.get(ctx, workerID, target)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, b.dialTO)
	defer cancel()
	ctx, err = grpcauth.AppendToContext(ctx, b.secret)
	if err != nil {
		return nil, err
	}
	return c.StartLink(ctx, req)
}

// StreamQR returns a server-streaming client. The caller is responsible for
// receiving from the stream and for closing it (the grpc client closes when
// the stream is exhausted or the context is cancelled).
func (b *bridgeClient) StreamQR(ctx context.Context, workerID, target string, req *bridgepb.StreamQRRequest) (grpc.ServerStreamingClient[bridgepb.QREvent], error) {
	c, err := b.get(ctx, workerID, target)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, b.dialTO)
	defer cancel()
	ctx, err = grpcauth.AppendToContext(ctx, b.secret)
	if err != nil {
		return nil, err
	}
	return c.StreamQR(ctx, req)
}

// Close shuts every cached connection. Called from main on shutdown.
func (b *bridgeClient) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, conn := range b.conns {
		if err := conn.Close(); err != nil {
			b.log.Warn("bridge gRPC close", slog.String("worker_id", id), slog.String("err", err.Error()))
		}
	}
	b.conns = nil
	b.clients = nil
}
