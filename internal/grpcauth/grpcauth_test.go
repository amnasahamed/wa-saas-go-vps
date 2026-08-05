package grpcauth

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestAppendToContext_EmptySecretErrors(t *testing.T) {
	if _, err := AppendToContext(context.Background(), ""); err == nil {
		t.Fatal("empty secret should error")
	} else if status.Code(err) != codes.Unauthenticated {
		t.Errorf("want Unauthenticated, got %v", err)
	}
}

func TestAppendToContext_AttachesMetadata(t *testing.T) {
	ctx, err := AppendToContext(context.Background(), "s3cret")
	if err != nil {
		t.Fatalf("AppendToContext: %v", err)
	}
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata on ctx")
	}
	if got := md.Get(MetadataKey); len(got) != 1 || got[0] != "s3cret" {
		t.Errorf("metadata = %v, want %q=s3cret", got, MetadataKey)
	}
}

func TestAuth_DevModeBypass(t *testing.T) {
	a := New("") // dev
	if err := a.check(context.Background()); err != nil {
		t.Errorf("empty Auth.check should be a no-op, got %v", err)
	}
}

func TestAuth_MissingMetadata(t *testing.T) {
	a := New("s3cret")
	err := a.check(context.Background())
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("missing metadata -> Unauthenticated, got %v", err)
	}
}

func TestAuth_MissingKey(t *testing.T) {
	a := New("s3cret")
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("other", "v"))
	err := a.check(ctx)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("missing key -> Unauthenticated, got %v", err)
	}
}

func TestAuth_WrongKey(t *testing.T) {
	a := New("s3cret")
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, "nope"))
	err := a.check(ctx)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("wrong key -> Unauthenticated, got %v", err)
	}
}

func TestAuth_RightKey(t *testing.T) {
	a := New("s3cret")
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, "s3cret"))
	if err := a.check(ctx); err != nil {
		t.Errorf("right key: %v", err)
	}
}

// sanity: interceptors do not panic on a no-op handler.
func TestInterceptors_Compose(t *testing.T) {
	a := New("s3cret")
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, "s3cret"))
	if _, err := a.UnaryServerInterceptor()(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}); err != nil {
		t.Errorf("unary: %v", err)
	}
}
