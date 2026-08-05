-- +goose Up
-- +goose StatementBegin
-- Migration 00001: Tenants, users, api_keys
-- The top-level identity tables. tenants own users + everything else.
-- All other tables FK back to tenants.id with ON DELETE CASCADE so
-- tenant deletion (rare, only post-cancellation) cleans up correctly.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE tenants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  slug TEXT UNIQUE NOT NULL,
  name TEXT NOT NULL,
  plan TEXT NOT NULL DEFAULT 'free'
    CHECK (plan IN ('free', 'pro', 'agency')),
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'suspended', 'canceled')),
  stripe_customer_id TEXT UNIQUE,

  -- Settings (jsonb) — country prefix, timezone, locale, prompt override
  country_code TEXT NOT NULL DEFAULT '91',
  timezone TEXT NOT NULL DEFAULT 'Asia/Kolkata',
  locale TEXT NOT NULL DEFAULT 'en-IN',
  prompt_override TEXT,

  -- BYO AI key (envelope-encrypted with KMS-managed data key)
  openai_key_ciphertext BYTEA,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX tenants_stripe_customer_idx ON tenants(stripe_customer_id) WHERE stripe_customer_id IS NOT NULL;
CREATE INDEX tenants_status_idx ON tenants(status);

CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  email TEXT UNIQUE NOT NULL,
  password_hash TEXT,
  role TEXT NOT NULL DEFAULT 'admin'
    CHECK (role IN ('admin', 'member')),
  email_verified_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX users_tenant_idx ON users(tenant_id);
CREATE INDEX users_email_verified_idx ON users(email_verified_at) WHERE email_verified_at IS NULL;

-- Email verification tokens (one-time use, short TTL)
CREATE TABLE email_verify_tokens (
  token TEXT PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX email_verify_tokens_user_idx ON email_verify_tokens(user_id);
CREATE INDEX email_verify_tokens_expires_idx ON email_verify_tokens(expires_at) WHERE consumed_at IS NULL;

-- API keys (bridge / webhook / dashboard scopes)
CREATE TABLE api_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  key_prefix TEXT NOT NULL,         -- "wa_live_abc123" (safe to show in UI)
  key_hash TEXT NOT NULL,           -- bcrypt of the secret portion
  scope TEXT NOT NULL
    CHECK (scope IN ('bridge', 'webhook', 'dashboard')),
  last_used_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX api_keys_tenant_idx ON api_keys(tenant_id);
CREATE INDEX api_keys_prefix_idx ON api_keys(key_prefix) WHERE revoked_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS email_verify_tokens;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;
-- +goose StatementEnd
