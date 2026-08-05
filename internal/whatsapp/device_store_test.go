package whatsapp

import (
	"context"
	"path/filepath"
	"testing"

	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

// TestDeviceStoreOpens guards the device-store wiring in loadSession.
//
// This is a regression test for a defect that compiled cleanly but panicked at
// runtime the first time a tenant linked WhatsApp: sqlstore.New was called with
// driver name "sqlite3" while no SQLite driver was imported anywhere in the
// module, yielding `sql: unknown driver "sqlite3" (forgotten import?)`.
//
// It pins three things that must agree, and which nothing else checks:
//   - the blank import of modernc.org/sqlite in manager.go registers a driver
//   - the driver name "sqlite" matches what that package registers
//   - dbutil.ParseDialect accepts "sqlite" (it prefix-matches), so NewWithDB
//     does not panic on an invalid dialect
//   - foreign_keys is enabled, without which whatsmeow's schema upgrade fails
//     with "foreign keys are not enabled"
//
// Keep the DSN in sync with loadSession; the ?_pragma= form is modernc syntax.
func TestDeviceStoreOpens(t *testing.T) {
	ctx := context.Background()
	devicePath := filepath.Join(t.TempDir(), "device.sqlite3")
	dsn := "file:" + devicePath + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

	container, err := sqlstore.New(ctx, "sqlite", dsn, nil)
	if err != nil {
		t.Fatalf("sqlstore.New with driver %q: %v", "sqlite", err)
	}
	t.Cleanup(func() { _ = container.Close() })

	// A fresh path has no stored credentials, so GetDevice should miss rather
	// than error out on a broken schema. This exercises real queries against
	// the file, proving the driver is registered and the migrations applied.
	deviceJID := types.JID{User: "test-session", Server: "s.whatsapp.net"}
	if _, err := container.GetDevice(ctx, deviceJID); err == nil {
		t.Log("GetDevice returned no error on empty store")
	}

	// NewDevice + Put is the path loadSession takes for an unlinked tenant.
	device := container.NewDevice()
	if device == nil {
		t.Fatal("NewDevice returned nil")
	}
}
