package destinations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/amnasahamed/niosengine/internal/db"
)

// PGRepository implements Repository backed by sqlc-generated queries.
type PGRepository struct {
	q *db.Queries
}

func NewPGRepository(q *db.Queries) *PGRepository { return &PGRepository{q: q} }

func (r *PGRepository) Create(ctx context.Context, d *Destination) error {
	mappingJSON, err := json.Marshal(d.FieldMapping)
	if err != nil {
		return fmt.Errorf("marshal field mapping: %w", err)
	}
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	id, err := uuid.Parse(d.ID)
	if err != nil {
		return err
	}
	tid, err := uuid.Parse(d.TenantID)
	if err != nil {
		return err
	}
	_, err = r.q.CreateDestination(ctx, db.CreateDestinationParams{
		ID:              db.PgtypeUUID(id),
		TenantID:        db.PgtypeUUID(tid),
		Type:            d.Type,
		Name:            d.Name,
		ConfigEncrypted: d.ConfigCiphertext,
		FieldMapping:    mappingJSON,
		Enabled:         d.Enabled,
	})
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	return nil
}

func (r *PGRepository) GetByID(ctx context.Context, id string) (*Destination, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, ErrNotFound
	}
	row, err := r.q.GetDestinationByID(ctx, db.PgtypeUUID(uid))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return rowToDestination(row), nil
}

func (r *PGRepository) ListByTenant(ctx context.Context, tenantID string) ([]*Destination, error) {
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListAllDestinationsByTenant(ctx, db.PgtypeUUID(tid))
	if err != nil {
		return nil, err
	}
	out := make([]*Destination, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToDestination(row))
	}
	return out, nil
}

func (r *PGRepository) Update(ctx context.Context, d *Destination) error {
	id, err := uuid.Parse(d.ID)
	if err != nil {
		return err
	}
	mappingJSON, err := json.Marshal(d.FieldMapping)
	if err != nil {
		return err
	}
	return r.q.UpdateDestinationFieldMapping(ctx, db.UpdateDestinationFieldMappingParams{
		ID:           db.PgtypeUUID(id),
		FieldMapping: mappingJSON,
	})
}

func (r *PGRepository) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	// Caller should verify ownership before calling.
	return r.q.DeleteDestination(ctx, db.DeleteDestinationParams{
		ID: db.PgtypeUUID(uid),
		// TenantID — caller is responsible for scope. We pass nil-equivalent.
	})
}

func (r *PGRepository) RecordSync(ctx context.Context, tenantID, leadID, destID, status, externalRowID string, recErr error) error {
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		return err
	}
	lid, err := uuid.Parse(leadID)
	if err != nil {
		return err
	}
	did, err := uuid.Parse(destID)
	if err != nil {
		return err
	}
	errStr := ""
	if recErr != nil {
		errStr = recErr.Error()
	}
	_, err = r.q.RecordSync(ctx, db.RecordSyncParams{
		TenantID:      db.PgtypeUUID(tid),
		LeadID:        db.PgtypeUUID(lid),
		DestinationID: db.PgtypeUUID(did),
		Status:        status,
		ExternalRowID: db.StringToText(externalRowID),
		Error:         db.StringToText(errStr),
		SyncedAt:      db.TimeToTimestamptz(time.Now().UTC()),
	})
	return err
}

func rowToDestination(row db.Destination) *Destination {
	mapping := map[string]string{}
	if len(row.FieldMapping) > 0 {
		_ = json.Unmarshal(row.FieldMapping, &mapping)
	}
	return &Destination{
		ID:               uuidFromPgtype(row.ID),
		TenantID:         uuidFromPgtype(row.TenantID),
		Type:             row.Type,
		Name:             row.Name,
		ConfigCiphertext: row.ConfigEncrypted,
		FieldMapping:     mapping,
		Enabled:          row.Enabled,
		LastSyncAt:       timePtrFromPgtype(row.LastSyncAt),
		CreatedAt:        timeFromPgtype(row.CreatedAt),
	}
}

func uuidFromPgtype(p pgtype.UUID) string {
	u, err := db.UUIDFromPgtype(p)
	if err != nil {
		return ""
	}
	return u.String()
}

func timePtrFromPgtype(p pgtype.Timestamptz) *time.Time {
	t := db.TimeFromTimestamptzPtr(p)
	return t
}

func timeFromPgtype(p pgtype.Timestamptz) time.Time {
	return db.TimeFromTimestamptz(p)
}
