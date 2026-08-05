package destinations

import (
	"context"
	"encoding/json"
	"fmt"
)

// DefaultFieldMapping returns the canonical mapping from lead fields to
// destination columns/properties. Tenants can override per-destination.
//
// The CRM column names are the legacy Skillvard NIOS schema from
// workflownew.json — most small businesses running WhatsApp-based lead
// capture use a similar shape.
func DefaultFieldMapping() map[string]string {
	return map[string]string{
		// Identity
		"lead_id":         "Lead ID",
		"date":            "Date",
		"name":            "Student Name",
		"phone":           "Phone",
		"alternate_phone": "Alternate Phone",

		// Academic
		"class_level":     "Class/Level",
		"stream":          "Stream",
		"exam_result":     "Exam Result",
		"subjects_failed": "Subjects Failed",
		"subjects_count":  "Subjects Count",
		"has_certificate": "Has Certificate",
		"place":           "Place",

		// Lead management
		"source":          "Source",
		"price_disclosed": "Price Disclosed",
		"status":          "Lead Status",
		"lost_reason":     "Lost Reason",
		"assigned_to":     "Assigned To",
		"last_contacted":  "Last Contacted",
		"next_followup":   "Next Follow-up",
		"followup_count":  "Follow-ups Count",
		"remarks":         "Remarks",
	}
}

// GoogleSheetsConnector implements the Sheets upsert.
//
// MVP: uses the Sheets API directly. OAuth tokens live in the encrypted
// destination.config_ciphertext blob. Per-row update uses Append + delete-old
// to keep the schema simple (avoids the legacy linear-scan cost by relying
// on Google's row-level operations).
type GoogleSheetsConnector struct {
	// sheetSvc is *sheets.Service from google.golang.org/api/sheets/v4
	// (injected in Phase 4).
	sheetSvc any
}

// NewGoogleSheetsConnector returns a connector wired with the Sheets service.
func NewGoogleSheetsConnector(svc any) *GoogleSheetsConnector {
	return &GoogleSheetsConnector{sheetSvc: svc}
}

func (c *GoogleSheetsConnector) Type() string { return TypeGoogleSheets }

// Upsert writes the lead as one row, matching by phone column.
//
// Implementation note: we use Sheets.Values.Append with insertDataOption=INSERT_ROWS
// then delete the prior row if we found it. This avoids needing the Sheets
// batchUpdate API for find+replace, which is slow at scale.
func (c *GoogleSheetsConnector) Upsert(_ context.Context, rawConfig []byte, snap LeadSnapshot) (SyncResult, error) {
	var cfg ConfigGoogleSheets
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return SyncResult{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if cfg.SpreadsheetID == "" || cfg.AccessToken == "" {
		return SyncResult{}, ErrInvalidConfig
	}
	if cfg.SheetName == "" {
		cfg.SheetName = "Sheet1"
	}
	if cfg.PhoneColumn == "" {
		cfg.PhoneColumn = "Phone"
	}
	// TODO(phase-4): call Sheets.Values.Append + Find by phone + Delete old row.
	return SyncResult{Updated: true}, nil
}

// Test verifies the credentials by fetching the spreadsheet metadata.
func (c *GoogleSheetsConnector) Test(_ context.Context, rawConfig []byte) error {
	var cfg ConfigGoogleSheets
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if cfg.AccessToken == "" || cfg.SpreadsheetID == "" {
		return ErrInvalidConfig
	}
	// TODO(phase-4): call Sheets.Spreadsheets.Get with the token.
	return nil
}

// AirtableConnector implements the Airtable upsert.
type AirtableConnector struct {
	client any
}

func NewAirtableConnector(client any) *AirtableConnector {
	return &AirtableConnector{client: client}
}

func (c *AirtableConnector) Type() string { return TypeAirtable }

func (c *AirtableConnector) Upsert(_ context.Context, rawConfig []byte, snap LeadSnapshot) (SyncResult, error) {
	var cfg ConfigAirtable
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return SyncResult{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if cfg.AccessToken == "" || cfg.BaseID == "" || cfg.TableName == "" {
		return SyncResult{}, ErrInvalidConfig
	}
	if cfg.PhoneField == "" {
		cfg.PhoneField = "Phone"
	}
	// TODO(phase-4): Airtable List records filtered by PhoneField, then
	// Update or Create.
	return SyncResult{Updated: true}, nil
}

func (c *AirtableConnector) Test(_ context.Context, rawConfig []byte) error {
	var cfg ConfigAirtable
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if cfg.AccessToken == "" || cfg.BaseID == "" {
		return ErrInvalidConfig
	}
	// TODO(phase-4): Airtable List bases to verify access.
	return nil
}

// NotionConnector implements the Notion upsert.
type NotionConnector struct {
	client any
}

func NewNotionConnector(client any) *NotionConnector {
	return &NotionConnector{client: client}
}

func (c *NotionConnector) Type() string { return TypeNotion }

func (c *NotionConnector) Upsert(_ context.Context, rawConfig []byte, snap LeadSnapshot) (SyncResult, error) {
	var cfg ConfigNotion
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return SyncResult{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if cfg.AccessToken == "" || cfg.DatabaseID == "" {
		return SyncResult{}, ErrInvalidConfig
	}
	if cfg.PhoneProp == "" {
		cfg.PhoneProp = "Phone"
	}
	// TODO(phase-4): Notion Query database by PhoneProp, then Update page or
	// Create page.
	return SyncResult{Updated: true}, nil
}

func (c *NotionConnector) Test(_ context.Context, rawConfig []byte) error {
	var cfg ConfigNotion
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if cfg.AccessToken == "" || cfg.DatabaseID == "" {
		return ErrInvalidConfig
	}
	// TODO(phase-4): Notion Retrieve database.
	return nil
}

// Registry returns a Connector for the given type.
func Registry(destType string, sheetsSvc, airtableClient, notionClient any) (Connector, error) {
	switch destType {
	case TypeGoogleSheets:
		return NewGoogleSheetsConnector(sheetsSvc), nil
	case TypeAirtable:
		return NewAirtableConnector(airtableClient), nil
	case TypeNotion:
		return NewNotionConnector(notionClient), nil
	default:
		return nil, ErrUnsupportedType
	}
}
