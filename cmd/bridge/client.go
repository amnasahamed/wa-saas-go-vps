package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/amnasahamed/niosengine/internal/grpcauth"

	bridgepb "github.com/amnasahamed/niosengine/internal/bridgepb"
)

// apiClient is the bridge's outbound gRPC client to cmd/api.
//
// In phase-1 it only carries ReportInbound (inbound events). Phase-3+ adds
// nudge RPCs and phase-4 adds destination sync pings. We keep it as a single
// type so future RPCs slot in without re-plumbing the connect/dial path.
//
// The connection is lazy: grpc.NewClient does not establish a connection
// until the first RPC. This means the bridge boots fine even if the API is
// not up yet. The first ReportInbound after the API comes up succeeds.
type apiClient struct {
	conn *grpc.ClientConn
	bridgepb.BridgeToAPIClient
	secret string
	dialTO time.Duration
	log    *slog.Logger
}

// newAPIClient configures (does not dial) the gRPC connection to the API.
//
// `secret` is the GRPC_SHARED_SECRET value used as `x-api-key` metadata on
// every RPC. An empty secret is permitted only in dev (Config.Load refuses
// to boot outside dev without it).
func newAPIClient(ctx context.Context, apiAddr, secret string) (*apiClient, error) {
	if apiAddr == "" {
		return nil, fmt.Errorf("bridge: api gRPC address is required")
	}
	conn, err := grpc.NewClient(apiAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("bridge: dial api: %w", err)
	}
	return &apiClient{
		conn:              conn,
		BridgeToAPIClient: bridgepb.NewBridgeToAPIClient(conn),
		secret:            secret,
		dialTO:            5 * time.Second,
		log:               slog.Default(),
	}, nil
}

// ReportInboundWithRetry sends an inbound event with bounded exponential
// backoff. Only transient gRPC codes are retried; a tenant-mismatch or
// bad-arg error is returned immediately for the caller to log+drop.
//
// This is the in-process retry buffer for Phase 1a. A bridge restart
// drops pending events; a durable retry buffer backed by the queue layer
// is a separate follow-up (see plan §Phase 1e — defer).
func (c *apiClient) ReportInboundWithRetry(parent context.Context, ev *bridgepb.InboundEvent) (*bridgepb.InboundAck, error) {
	const (
		maxAttempts = 5
		baseDelay   = 500 * time.Millisecond
		maxDelay    = 30 * time.Second
	)
	delay := baseDelay
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Per-attempt deadline so a slow API does not stall a whatsmeow
		// event callback indefinitely.
		ctx, cancel := context.WithTimeout(parent, c.dialTO)
		ctx, err := grpcauth.AppendToContext(ctx, c.secret)
		if err != nil {
			cancel()
			return nil, err
		}

		ack, err := c.ReportInbound(ctx, ev)
		cancel()
		if err == nil {
			return ack, nil
		}
		lastErr = err
		if !isRetryable(err) {
			return nil, err
		}
		if attempt == maxAttempts {
			break
		}
		c.log.Warn("ReportInbound retry",
			slog.Int("attempt", attempt),
			slog.String("err", err.Error()),
			slog.Duration("next_delay", delay),
		)
		select {
		case <-parent.Done():
			return nil, parent.Err()
		case <-time.After(delay):
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
	return nil, fmt.Errorf("ReportInbound exhausted retries: %w", lastErr)
}

// isRetryable returns true for transient gRPC codes the bridge should
// retry. PermissionDenied / Unauthenticated / InvalidArgument are
// caller errors and short-circuit.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted:
		return true
	default:
		return false
	}
}

// Close releases the connection.
func (c *apiClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
