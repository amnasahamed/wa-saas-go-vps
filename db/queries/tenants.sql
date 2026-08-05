-- name: CreateTenant :one
INSERT INTO tenants (id, slug, name, plan, status, country_code, timezone, locale)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetTenantByID :one
SELECT * FROM tenants WHERE id = $1;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = $1;

-- name: GetTenantByStripeCustomer :one
SELECT * FROM tenants WHERE stripe_customer_id = $1;

-- name: UpdateTenantSettings :one
UPDATE tenants
SET
  name = COALESCE(sqlc.narg('name'), name),
  country_code = COALESCE(sqlc.narg('country_code'), country_code),
  timezone = COALESCE(sqlc.narg('timezone'), timezone),
  locale = COALESCE(sqlc.narg('locale'), locale),
  prompt_override = COALESCE(sqlc.narg('prompt_override'), prompt_override)
WHERE id = $1
RETURNING *;

-- name: UpdateTenantStripeCustomer :exec
UPDATE tenants SET stripe_customer_id = $2 WHERE id = $1;

-- name: SetTenantStatus :exec
UPDATE tenants SET status = $2 WHERE id = $1;

-- name: SetTenantOpenAIKey :exec
UPDATE tenants SET openai_key_ciphertext = $2 WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (id, tenant_id, email, password_hash, role)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: VerifyUserEmail :exec
UPDATE users SET email_verified_at = NOW() WHERE id = $1;

-- name: InsertEmailVerifyToken :exec
INSERT INTO email_verify_tokens (token, user_id, expires_at) VALUES ($1, $2, $3);

-- name: ConsumeEmailVerifyToken :one
UPDATE email_verify_tokens
SET consumed_at = NOW()
WHERE token = $1 AND consumed_at IS NULL AND expires_at > NOW()
RETURNING user_id;

-- name: CreateAPIKey :one
INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, scope)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAPIKeyByPrefix :one
SELECT * FROM api_keys
WHERE key_prefix = $1 AND revoked_at IS NULL
LIMIT 1;

-- name: ListAPIKeysByTenant :many
SELECT * FROM api_keys
WHERE tenant_id = $1 AND revoked_at IS NULL
ORDER BY created_at DESC;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = NOW() WHERE id = $1;

-- name: RevokeAPIKey :exec
UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND tenant_id = $2;
