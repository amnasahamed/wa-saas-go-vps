-- +goose Up
-- +goose StatementBegin
-- Migration 00004: Destinations + sync log
-- Per-tenant CRM connectors. Config blob is envelope-encrypted
-- (KMS data key per tenant) so DB dumps don't leak OAuth tokens.

CREATE TABLE destinations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

  type TEXT NOT NULL CHECK (type IN ('google_sheets', 'airtable', 'notion')),
  name TEXT NOT NULL,

  -- Envelope-encrypted: nonce(12) || aesgcm_seal(JSON config, aad=tenant_id)
  config_encrypted BYTEA NOT NULL,

  -- lead.field → destination column name override
  field_mapping JSONB NOT NULL DEFAULT '{}'::jsonb,

  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  last_sync_at TIMESTAMPTZ,
  last_error TEXT,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX destinations_tenant_idx ON destinations(tenant_id);
CREATE INDEX destinations_tenant_enabled_idx ON destinations(tenant_id) WHERE enabled = TRUE;

-- Sync log: every upsert attempt (success or failure) is recorded for
-- debugging + retrying. Keeps the retry queue simple.
CREATE TABLE destination_syncs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
  destination_id UUID NOT NULL REFERENCES destinations(id) ON DELETE CASCADE,

  status TEXT NOT NULL CHECK (status IN ('pending', 'synced', 'failed')),
  external_row_id TEXT,
  error TEXT,

  attempts INT NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ,

  synced_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX destination_syncs_tenant_lead_idx ON destination_syncs(tenant_id, lead_id, created_at DESC);
CREATE INDEX destination_syncs_pending_retry_idx
  ON destination_syncs(next_retry_at)
  WHERE status = 'pending';
CREATE INDEX destination_syncs_destination_idx
  ON destination_syncs(destination_id, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS destination_syncs;
DROP TABLE IF EXISTS destinations;
-- +goose StatementEnd
