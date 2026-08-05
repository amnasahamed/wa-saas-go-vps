-- +goose Up
-- +goose StatementBegin
-- Migration 00002: WhatsApp sessions
-- One session per tenant in MVP (UNIQUE on tenant_id). The agency plan
-- allows more tenants per organization but each is its own tenant row.
-- worker_id is set by the orchestrator when assigning the session to a
-- bridge worker (stable hash-based sharding).

CREATE TABLE whatsapp_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,

  -- Assigned after QR link completes (whatsmeow exposes PhoneNumber once authenticated)
  phone_number TEXT,
  push_name TEXT,

  status TEXT NOT NULL DEFAULT 'pending_qr'
    CHECK (status IN ('pending_qr', 'qr_required', 'authenticated', 'ready', 'disconnected', 'error')),
  worker_id TEXT,
  qr_code TEXT,
  qr_expires_at TIMESTAMPTZ,
  last_event_at TIMESTAMPTZ,
  last_error TEXT,

  -- whatsmeow device state lives on the bridge worker's local disk;
  -- this column is the S3 backup key (snapshotted on every successful login).
  device_state_s3_key TEXT,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX whatsapp_sessions_worker_idx ON whatsapp_sessions(worker_id) WHERE worker_id IS NOT NULL;
CREATE INDEX whatsapp_sessions_status_idx ON whatsapp_sessions(status);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS whatsapp_sessions;
-- +goose StatementEnd
