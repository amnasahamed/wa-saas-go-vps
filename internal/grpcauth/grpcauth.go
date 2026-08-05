// Package grpcauth provides shared-secret authentication for the api↔bridge
// gRPC link.
//
// Both services send a `x-api-key` metadata header on every RPC. The server
// validates it with a constant-time compare. We use plain metadata rather
// than per-RPC credentials because grpc-go v1.82 no longer exposes a public
// PerRPCCreds type, and metadata is enough for a private-network link. TLS
// would still be the right answer for an untrusted network; this auth is
// the belt to that suspenders.
//
// In `dev`, an empty secret is accepted so local docker-compose.dev.yml
// still boots without extra config. In any other env, Config.Load refuses
// to start without a secret.
package grpcauth

import (
	"context"
	"crypto/subtle"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// MetadataKey is the gRPC metadata header both sides use.
const MetadataKey = "x-api-key"

// AppendToContext returns a context that carries the shared secret as
// `x-api-key` metadata. Pass it as the first argument to a gRPC call:
//
//	ctx, cancel := grpcauth.AppendToContext(ctx, secret, timeout)
//	defer cancel()
//	resp, err := client.ReportInbound(ctx, &evt{})
//
// An empty secret is a programming error at this layer; Config.Load
// refuses to boot outside dev when the secret is empty, so callers
// reaching here with an empty secret are usually in tests.
func AppendToContext(ctx context.Context, secret string) (context.Context, error) {
	if secret == "" {
		return nil, status.Error(codes.Unauthenticated, "grpcauth: empty shared secret")
	}
	return metadata.AppendToOutgoingContext(ctx, MetadataKey, secret), nil
}

// Auth holds the server-side secret and provides the gRPC interceptors.
type Auth struct {
	secret string
}

// New returns an Auth that admits requests carrying the given secret.
//
// An empty secret turns Auth into a no-op (every request passes). The
// Config layer enforces non-empty secrets outside dev, so an empty Auth
// here means we are intentionally permissive for a local dev run.
func New(secret string) *Auth {
	return &Auth{secret: secret}
}

// UnaryServerInterceptor validates the x-api-key metadata on unary RPCs.
func (a *Auth) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := a.check(ctx); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamServerInterceptor validates the x-api-key metadata on streaming RPCs.
func (a *Auth) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := a.check(ss.Context()); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func (a *Auth) check(ctx context.Context) error {
	if a.secret == "" {
		return nil // dev mode bypass
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get(MetadataKey)
	if len(values) == 0 {
		return status.Error(codes.Unauthenticated, "missing x-api-key")
	}
	// constant-time compare guards against timing oracles even though the
	// value comes over an already-trusted private network.
	if subtle.ConstantTimeCompare([]byte(values[0]), []byte(a.secret)) != 1 {
		return status.Error(codes.Unauthenticated, "invalid x-api-key")
	}
	return nil
}
