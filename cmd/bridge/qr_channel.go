package main

import (
	"context"

	"google.golang.org/grpc"

	bridgepb "github.com/amnasahamed/niosengine/internal/bridgepb"
	"github.com/amnasahamed/niosengine/internal/whatsapp"
)

// streamQRChannel adapts a gRPC ServerStream to the whatsapp.QRChannel
// interface used by the manager's QR fanout. The adapter is single-use:
// one instance per StreamQR RPC.
//
// Once a stream context is cancelled (the client disconnected, or the
// server is shutting down), Send short-circuits so the manager's
// background goroutine does not block on a dead stream.
type streamQRChannel struct {
	stream grpc.ServerStreamingServer[bridgepb.QREvent]
	done   chan struct{}
	once   bool
}

func newStreamQRChannel(stream grpc.ServerStreamingServer[bridgepb.QREvent]) *streamQRChannel {
	return &streamQRChannel{
		stream: stream,
		done:   make(chan struct{}),
	}
}

func (c *streamQRChannel) Send(ctx context.Context, ev whatsapp.QREvent) error {
	select {
	case <-c.done:
		return context.Canceled
	default:
	}
	return c.stream.Send(&bridgepb.QREvent{
		Code:      ev.Code,
		DataUrl:   ev.DataURL,
		ExpiresAt: timestamppbOrZero(ev.ExpiresAt),
	})
}

func (c *streamQRChannel) Close() error {
	if c.once {
		return nil
	}
	c.once = true
	close(c.done)
	return nil
}
