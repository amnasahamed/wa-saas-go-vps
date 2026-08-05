package whatsapp

import (
	"context"
	"fmt"
	"time"

	"github.com/skip2/go-qrcode"
)

// QREvent is emitted by whatsmeow when a new QR code is generated.
// The bridge persists this to whatsapp_sessions.qr_code and streams it
// to the dashboard via gRPC so the user can scan with their phone.
type QREvent struct {
	SessionID string
	TenantID  string
	Code      string // raw QR string from whatsmeow
	DataURL   string // data:image/png;base64,... (rendered)
	IssuedAt  time.Time
	ExpiresAt time.Time // typically ~60s
}

// QRChannel delivers QR events for a single session to the dashboard
// during the linking flow. Implemented in cmd/bridge as a gRPC server stream.
type QRChannel interface {
	// Send blocks until the event is consumed or the context is cancelled.
	Send(ctx context.Context, ev QREvent) error

	// Close terminates the channel cleanly.
	Close() error
}

// RenderQRDataURL converts the raw QR string into a base64 PNG data URL for
// inline display in the dashboard. Wraps github.com/skip2/go-qrcode.
//
// The PNG is encoded at a 512px size with the medium error-correction level,
// which is sufficient for mobile camera scanning at typical phone-to-screen
// distances. Returns an empty string + nil if the input is empty so callers
// can pass through a not-yet-issued QR without treating it as an error.
func RenderQRDataURL(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	png, err := qrcode.Encode(raw, qrcode.Medium, 512)
	if err != nil {
		return "", fmt.Errorf("encode qr: %w", err)
	}
	return "data:image/png;base64," + base64Encode(png), nil
}
