-- name: CreateDestination :one
INSERT INTO destinations (id, tenant_id, type, name, config_encrypted, field_mapping, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetDestinationByID :one
SELECT * FROM destinations WHERE id = $1;

-- name: ListDestinationsByTenant :many
SELECT * FROM destinations
WHERE tenant_id = $1 AND enabled = TRUE
ORDER BY created_at DESC;

-- name: ListAllDestinationsByTenant :many
SELECT * FROM destinations
WHERE tenant_id = $1
ORDER BY created_at DESC;

-- name: UpdateDestinationFieldMapping :exec
UPDATE destinations SET field_mapping = $2 WHERE id = $1;

-- name: UpdateDestinationConfig :exec
UPDATE destinations SET config_encrypted = $2 WHERE id = $1;

-- name: SetDestinationEnabled :exec
UPDATE destinations SET enabled = $2 WHERE id = $1;

-- name: TouchDestinationLastSync :exec
UPDATE destinations
SET last_sync_at = NOW(), last_error = $2
WHERE id = $1;

-- name: DeleteDestination :exec
DELETE FROM destinations WHERE id = $1 AND tenant_id = $2;

-- name: RecordSync :one
INSERT INTO destination_syncs (id, tenant_id, lead_id, destination_id, status, external_row_id, error, attempts, synced_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListSyncsByLead :many
SELECT * FROM destination_syncs
WHERE lead_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ListPendingRetries :many
SELECT * FROM destination_syncs
WHERE status = 'pending' AND next_retry_at <= NOW()
ORDER BY next_retry_at ASC
LIMIT $1;

-- name: UpdateSyncRetry :exec
UPDATE destination_syncs
SET attempts = $2, next_retry_at = $3, error = $4
WHERE id = $1;

-- name: UpdateSubscription :one
INSERT INTO subscriptions (id, tenant_id, stripe_subscription_id, plan, status,
                            current_period_start, current_period_end, cancel_at_period_end)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (stripe_subscription_id) DO UPDATE SET
  plan = EXCLUDED.plan,
  status = EXCLUDED.status,
  current_period_start = EXCLUDED.current_period_start,
  current_period_end = EXCLUDED.current_period_end,
  cancel_at_period_end = EXCLUDED.cancel_at_period_end,
  updated_at = NOW()
RETURNING *;

-- name: GetSubscriptionByTenant :one
SELECT * FROM subscriptions
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: CancelSubscription :exec
UPDATE subscriptions SET status = 'canceled', canceled_at = NOW(), updated_at = NOW()
WHERE stripe_subscription_id = $1;

-- name: InsertStripeEvent :exec
INSERT INTO stripe_events (event_id, event_type, payload) VALUES ($1, $2, $3)
ON CONFLICT (event_id) DO NOTHING;

-- name: InsertAuditLog :one
INSERT INTO audit_logs (tenant_id, user_id, api_key_id, action, resource, metadata, ip, user_agent)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListAuditLogs :many
SELECT * FROM audit_logs
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;
