-- name: CreateWhatsAppSession :one
INSERT INTO whatsapp_sessions (id, tenant_id, status)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetWhatsAppSessionByID :one
SELECT * FROM whatsapp_sessions WHERE id = $1;

-- name: GetWhatsAppSessionByTenant :one
SELECT * FROM whatsapp_sessions WHERE tenant_id = $1;

-- name: ListWhatsAppSessionsByWorker :many
SELECT * FROM whatsapp_sessions WHERE worker_id = $1;

-- name: ListUnassignedWhatsAppSessions :many
SELECT * FROM whatsapp_sessions WHERE worker_id IS NULL;

-- name: AssignWhatsAppSessionWorker :exec
UPDATE whatsapp_sessions SET worker_id = $2 WHERE id = $1;

-- name: UpdateWhatsAppSessionStatus :exec
UPDATE whatsapp_sessions
SET status = $2, last_error = $3, last_event_at = NOW()
WHERE id = $1;

-- name: UpdateWhatsAppSessionQR :exec
UPDATE whatsapp_sessions
SET status = 'qr_required', qr_code = $2, qr_expires_at = $3
WHERE id = $1;

-- name: SetWhatsAppSessionAuthenticated :one
UPDATE whatsapp_sessions
SET status = 'authenticated', phone_number = $2, push_name = $3,
    qr_code = NULL, qr_expires_at = NULL, last_event_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ClearWhatsAppSessionWorker :exec
UPDATE whatsapp_sessions SET worker_id = NULL WHERE worker_id = $1;

-- name: DeleteWhatsAppSession :exec
DELETE FROM whatsapp_sessions WHERE id = $1 AND tenant_id = $2;
