-- name: InsertMessage :one
INSERT INTO messages (
  id, tenant_id, whatsapp_session_id, provider_message_id, phone,
  direction, sender, message_type, body, media_s3_key, media_mimetype,
  idempotency_key, timestamp
) VALUES (
  $1, $2, $3, $4, $5,
  $6, $7, $8, $9, $10, $11,
  $12, $13
)
RETURNING *;

-- name: GetMessageByID :one
SELECT * FROM messages WHERE id = $1;

-- name: GetMessageByIdempotencyKey :one
SELECT * FROM messages WHERE idempotency_key = $1;

-- name: ListMessagesByPhone :many
SELECT * FROM messages
WHERE tenant_id = $1 AND phone = $2
ORDER BY timestamp ASC
LIMIT $3;

-- name: ListRecentMessagesByTenant :many
SELECT * FROM messages
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpsertLead :one
INSERT INTO leads (id, tenant_id, phone, name, fields, status, stage)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, phone) DO UPDATE SET
  name = COALESCE(EXCLUDED.name, leads.name)
RETURNING *;

-- name: GetLeadByID :one
SELECT * FROM leads WHERE id = $1;

-- name: GetLeadByPhone :one
SELECT * FROM leads WHERE tenant_id = $1 AND phone = $2;

-- name: ListLeadsByTenant :many
SELECT * FROM leads
WHERE tenant_id = $1
  AND ($2::text IS NULL OR status = $2)
ORDER BY updated_at DESC
LIMIT $3 OFFSET $4;

-- name: UpdateLeadFields :exec
UPDATE leads SET fields = $2 WHERE id = $1;

-- name: UpdateLeadName :exec
UPDATE leads SET name = $2 WHERE id = $1;

-- name: UpdateLeadAlternatePhone :exec
UPDATE leads SET alternate_phone = $2 WHERE id = $1;

-- name: UpdateLeadStatus :exec
UPDATE leads SET status = $2 WHERE id = $1;

-- name: UpdateLeadStage :exec
UPDATE leads SET stage = $2 WHERE id = $1;

-- name: UpdateLeadTimestamps :exec
UPDATE leads
SET
  last_inbound_at = COALESCE($2, last_inbound_at),
  last_contacted_at = COALESCE($3, last_contacted_at),
  next_followup_at = COALESCE($4, next_followup_at)
WHERE id = $1;

-- name: SetLeadNurturePaused :exec
UPDATE leads
SET nurture_paused = TRUE, nurture_paused_until = $2
WHERE id = $1;

-- name: ClearLeadNurturePaused :exec
UPDATE leads
SET nurture_paused = FALSE, nurture_paused_until = NULL
WHERE id = $1 AND (nurture_paused_until IS NULL OR nurture_paused_until <= NOW());

-- name: IncrementLeadFollowupCount :exec
UPDATE leads SET followup_count = followup_count + 1 WHERE id = $1;

-- name: InsertExtraction :one
INSERT INTO extractions (id, tenant_id, message_id, lead_id, prompt_template, raw_response, fields)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListExtractionsByLead :many
SELECT * FROM extractions
WHERE lead_id = $1
ORDER BY created_at DESC
LIMIT $2;
