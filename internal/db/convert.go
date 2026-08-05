// Conversion helpers between pgx's nullable types (pgtype.UUID,
// pgtype.Timestamptz, etc.) and the idiomatic Go types used by the rest of
// the codebase (google/uuid.UUID, time.Time).
//
// Also handles jsonb columns — sqlc generates them as []byte (raw JSON),
// we unmarshal to map[string]any for ergonomic access in the repository
// layer.
//
// These helpers are kept here (not in each package) so the conversion
// rules live in one place and can evolve together with the schema.

package db

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UUID converts a google/uuid.UUID to pgx's pgtype.UUID.
func PgtypeUUID(u uuid.UUID) pgtype.UUID {
	var p pgtype.UUID
	copy(p.Bytes[:], u[:])
	p.Valid = true
	return p
}

// UUID parses a pgx pgtype.UUID to a google/uuid.UUID.
// Returns uuid.Nil (and no error) if the input is NULL — callers that need
// to distinguish "missing" from "valid zero UUID" should check .Valid first.
func UUIDFromPgtype(p pgtype.UUID) (uuid.UUID, error) {
	if !p.Valid {
		return uuid.Nil, nil
	}
	return uuid.FromBytes(p.Bytes[:])
}

// MustUUID is the panicking version of UUIDFromPgtype. Use only when you
// know the value is valid.
func MustUUID(p pgtype.UUID) uuid.UUID {
	u, err := UUIDFromPgtype(p)
	if err != nil {
		panic(err)
	}
	return u
}

// UUIDPtr returns a *uuid.UUID for use in sqlc Params structs.
func UUIDPtr(u uuid.UUID) *pgtype.UUID {
	v := PgtypeUUID(u)
	return &v
}

// TimeToTimestamptz converts a time.Time to a valid pgtype.Timestamptz.
func TimeToTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// TimeFromTimestamptz returns the time.Time value if valid, or zero time.
func TimeFromTimestamptz(p pgtype.Timestamptz) time.Time {
	if !p.Valid {
		return time.Time{}
	}
	return p.Time
}

// TimePtrToTimestamptz converts *time.Time → *pgtype.Timestamptz.
func TimePtrToTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return TimeToTimestamptz(*t)
}

// TimeFromTimestamptzPtr returns *time.Time (nil if NULL).
func TimeFromTimestamptzPtr(p pgtype.Timestamptz) *time.Time {
	if !p.Valid {
		return nil
	}
	t := p.Time
	return &t
}

// StringFromText returns the string value of a nullable text column.
func StringFromText(p pgtype.Text) string {
	if !p.Valid {
		return ""
	}
	return p.String
}

// StringPtrToText converts *string → pgtype.Text.
func StringPtrToText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// StringToText converts string → pgtype.Text (always Valid).
func StringToText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

// JSONToMap unmarshals a jsonb []byte to a map. Returns nil for NULL or
// empty bytes.
func JSONToMap(b []byte) (map[string]any, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// MapToJSON marshals a map to jsonb []byte. Returns nil for nil maps.
func MapToJSON(m map[string]any) ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// ErrNoRows is re-exported so callers don't have to import pgx directly.
var ErrNoRows = errors.New("db: no rows in result set")
