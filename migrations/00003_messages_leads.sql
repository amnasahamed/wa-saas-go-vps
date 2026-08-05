-- +goose Up
-- +goose StatementBegin
-- Migration 00003: Messages, leads, extractions
-- The hot tables. Indexed for the two main access patterns:
--   1) Get last N messages for (tenant, phone) — AI history load
--   2) Get lead + status — dashboard table view

CREATE TABLE messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  whatsapp_session_id UUID NOT NULL REFERENCES whatsapp_sessions(id) ON DELETE CASCADE,

  -- whatsmeow message ID; combined with tenant_id for idempotency
  provider_message_id TEXT NOT NULL,

  phone TEXT NOT NULL,
  direction TEXT NOT NULL CHECK (direction IN ('inbound', 'outbound')),
  sender TEXT NOT NULL CHECK (sender IN ('customer', 'ai', 'counselor')),
  message_type TEXT NOT NULL,

  body TEXT,
  media_s3_key TEXT,
  media_mimetype TEXT,

  -- (tenant_id, provider_message_id) is the durable idempotency anchor.
  -- A UNIQUE constraint here kills webhook replays.
  idempotency_key TEXT NOT NULL UNIQUE,

  timestamp TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Hot path: load conversation history for AI prompt
CREATE INDEX messages_tenant_phone_ts_idx
  ON messages(tenant_id, phone, timestamp DESC);

-- Dashboard: list by tenant
CREATE INDEX messages_tenant_created_idx
  ON messages(tenant_id, created_at DESC);

CREATE TABLE leads (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

  phone TEXT NOT NULL,
  alternate_phone TEXT,
  name TEXT,

  -- JSONB bag of all extracted CRM fields. Schema is fluid per tenant.
  fields JSONB NOT NULL DEFAULT '{}'::jsonb,

  status TEXT NOT NULL DEFAULT 'New',
  stage TEXT NOT NULL DEFAULT 'ask_name',

  nurture_paused BOOLEAN NOT NULL DEFAULT FALSE,
  nurture_paused_until TIMESTAMPTZ,

  last_inbound_at TIMESTAMPTZ,
  last_contacted_at TIMESTAMPTZ,
  next_followup_at TIMESTAMPTZ,
  followup_count INT NOT NULL DEFAULT 0,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

  UNIQUE(tenant_id, phone)
);

CREATE INDEX leads_tenant_status_idx ON leads(tenant_id, status);
CREATE INDEX leads_tenant_next_followup_idx ON leads(tenant_id, next_followup_at)
  WHERE next_followup_at IS NOT NULL;
CREATE INDEX leads_tenant_updated_idx ON leads(tenant_id, updated_at DESC);

-- Extractions audit log (one row per GPT-4o extraction call)
CREATE TABLE extractions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,

  prompt_template TEXT,
  raw_response JSONB,
  fields JSONB NOT NULL DEFAULT '{}'::jsonb,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX extractions_tenant_lead_idx ON extractions(tenant_id, lead_id, created_at DESC);
CREATE INDEX extractions_message_idx ON extractions(message_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS extractions;
DROP TABLE IF EXISTS leads;
DROP TABLE IF EXISTS messages;
-- +goose StatementEnd
