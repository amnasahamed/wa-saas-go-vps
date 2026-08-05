package destinations

import (
	"context"
	"fmt"
)

// PGService implements Service using a Repository.
type PGService struct {
	repo Repository
}

// NewPGService wires the destination service.
func NewPGService(repo Repository) *PGService {
	return &PGService{repo: repo}
}

// Connect validates the type and persists a new destination.
//
// rawConfig is stored as-is. In production this should be envelope-encrypted
// via internal/crypto before being passed in; here we accept plaintext bytes
// for the dashboard MVP. TODO(phase-4): KMS envelope encryption.
func (s *PGService) Connect(
	ctx context.Context,
	tenantID, destType, name string,
	rawConfig []byte,
	fieldMapping map[string]string,
) (*Destination, error) {
	if destType != TypeGoogleSheets && destType != TypeAirtable && destType != TypeNotion {
		return nil, ErrUnsupportedType
	}
	if fieldMapping == nil {
		fieldMapping = map[string]string{}
	}
	d := &Destination{
		TenantID:         tenantID,
		Type:             destType,
		Name:             name,
		ConfigCiphertext: rawConfig,
		FieldMapping:     fieldMapping,
		Enabled:          true,
	}
	if err := s.repo.Create(ctx, d); err != nil {
		return nil, fmt.Errorf("create destination: %w", err)
	}
	return d, nil
}

func (s *PGService) List(ctx context.Context, tenantID string) ([]*Destination, error) {
	return s.repo.ListByTenant(ctx, tenantID)
}

func (s *PGService) SyncLead(ctx context.Context, tenantID string, snap LeadSnapshot) error {
	dests, err := s.repo.ListByTenant(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, d := range dests {
		if !d.Enabled {
			continue
		}
		conn, err := Registry(d.Type, nil, nil, nil)
		if err != nil {
			continue
		}
		_, _ = conn.Upsert(ctx, d.ConfigCiphertext, snap)
	}
	return nil
}

func (s *PGService) Disconnect(ctx context.Context, tenantID, destID string) error {
	d, err := s.repo.GetByID(ctx, destID)
	if err != nil {
		return err
	}
	if d.TenantID != tenantID {
		return ErrNotFound
	}
	return s.repo.Delete(ctx, destID)
}
