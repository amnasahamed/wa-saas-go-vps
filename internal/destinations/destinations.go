// Package destinations implements per-tenant CRM connectors.
//
// MVP launch destinations:
//   - google_sheets: OAuth2 per tenant, appendOrUpdate by phone
//   - airtable:      Personal Access Token or OAuth2, upsert by phone
//   - notion:        OAuth2 internal integration, page properties upsert
//
// Each connector implements the Connector interface. The destination
// service fans out sync events from internal/messages handlers and
// tracks state in destination_syncs.
package destinations

import (
	"context"
	"errors"
	"time"
)

// Destination types.
const (
	TypeGoogleSheets = "google_sheets"
	TypeAirtable     = "airtable"
	TypeNotion       = "notion"
)

// Destination is the persisted record.
type Destination struct {
	ID               string
	TenantID         string
	Type             string
	Name             string
	ConfigCiphertext []byte            // AES-GCM encrypted config blob (OAuth tokens, spreadsheet ID, etc.)
	FieldMapping     map[string]string // lead field -> destination column name
	Enabled          bool
	LastSyncAt       *time.Time
	CreatedAt        time.Time
}

// ConfigGoogleSheets is the decrypted Sheets-specific config.
type ConfigGoogleSheets struct {
	AccessToken   string    `json:"access_token"`
	RefreshToken  string    `json:"refresh_token"`
	TokenExpiry   time.Time `json:"token_expiry"`
	SpreadsheetID string    `json:"spreadsheet_id"`
	SheetName     string    `json:"sheet_name"`   // defaults to "Sheet1"
	PhoneColumn   string    `json:"phone_column"` // defaults to "Phone"
}

// ConfigAirtable is the decrypted Airtable config.
type ConfigAirtable struct {
	AccessToken string `json:"access_token"`
	BaseID      string `json:"base_id"`
	TableName   string `json:"table_name"`
	PhoneField  string `json:"phone_field"` // defaults to "Phone"
}

// ConfigNotion is the decrypted Notion config.
type ConfigNotion struct {
	AccessToken string `json:"access_token"`
	DatabaseID  string `json:"database_id"`
	PhoneProp   string `json:"phone_prop"` // defaults to "Phone"
}

// LeadSnapshot is the data passed to a connector to upsert.
// Connectors map from this canonical shape to their column/property layout
// using the destination's FieldMapping (with sensible defaults).
type LeadSnapshot struct {
	TenantID      string
	LeadID        string
	Phone         string
	Name          string
	Fields        map[string]any
	Status        string
	Stage         string
	LastInboundAt *time.Time
	Source        string // "WhatsApp"
	LeadCode      string // "WA-{phone}"
}

// SyncResult is what a connector returns after attempting to upsert.
type SyncResult struct {
	ExternalRowID string // sheet row number, Airtable record ID, Notion page ID
	Updated       bool
	Error         error
}

// Connector is the per-destination-type interface.
type Connector interface {
	Type() string
	// Upsert writes the lead to the destination, matching by phone.
	Upsert(ctx context.Context, rawConfig []byte, snap LeadSnapshot) (SyncResult, error)
	// Test verifies the destination credentials + reachability.
	Test(ctx context.Context, rawConfig []byte) error
}

// Repository persists Destination and Sync records.
type Repository interface {
	Create(ctx context.Context, d *Destination) error
	GetByID(ctx context.Context, id string) (*Destination, error)
	ListByTenant(ctx context.Context, tenantID string) ([]*Destination, error)
	Update(ctx context.Context, d *Destination) error
	Delete(ctx context.Context, id string) error

	// Sync log
	RecordSync(ctx context.Context, tenantID, leadID, destID, status, externalRowID string, err error) error
}

// Service is the high-level destination orchestration.
type Service interface {
	// Connect encrypts and persists a new destination.
	Connect(ctx context.Context, tenantID, destType, name string, rawConfig []byte, fieldMapping map[string]string) (*Destination, error)
	// List returns all destinations for a tenant.
	List(ctx context.Context, tenantID string) ([]*Destination, error)
	// SyncLead fans out a lead snapshot to all enabled destinations for the tenant.
	SyncLead(ctx context.Context, tenantID string, snap LeadSnapshot) error
	// Disconnect deletes a destination.
	Disconnect(ctx context.Context, tenantID, destID string) error
}

// ErrNotFound is returned when no destination matches.
var ErrNotFound = errors.New("destination not found")

// ErrUnsupportedType is returned for unknown destination types.
var ErrUnsupportedType = errors.New("unsupported destination type")

// ErrSyncFailed is returned when an upsert fails.
var ErrSyncFailed = errors.New("destination sync failed")

// ErrInvalidConfig is returned when decrypted config is missing required fields.
var ErrInvalidConfig = errors.New("invalid destination config")
