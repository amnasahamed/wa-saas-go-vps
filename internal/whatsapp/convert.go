package whatsapp

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// pgtypeToUUID converts a google/uuid.UUID to pgtype.UUID for sqlc params.
func pgtypeToUUID(u uuid.UUID) pgtype.UUID {
	var p pgtype.UUID
	copy(p.Bytes[:], u[:])
	p.Valid = true
	return p
}

// uuidFromPgtype converts a pgtype.UUID to a google/uuid.UUID.
// Returns ("", false) if the input is NULL.
func uuidFromPgtype(p pgtype.UUID) (string, error) {
	if !p.Valid {
		return "", nil
	}
	u, err := uuid.FromBytes(p.Bytes[:])
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// uuidToString converts a pgtype.UUID to its canonical string form.
// Returns "" for NULL.
func uuidToString(p pgtype.UUID) string {
	s, _ := uuidFromPgtype(p)
	return s
}

// textToPgtype converts a string to pgtype.Text (always Valid).
func textToPgtype(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}
