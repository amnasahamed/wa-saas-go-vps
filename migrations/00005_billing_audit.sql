-- +goose Up
-- +goose StatementBegin
-- Migration 00005: Subscriptions, Stripe events, audit logs

CREATE TABLE subscriptions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  stripe_subscription_id TEXT UNIQUE NOT NULL,

  plan TEXT NOT NULL CHECK (plan IN ('free', 'pro', 'agency')),
  status TEXT NOT NULL
    CHECK (status IN ('trialing', 'active', 'past_due', 'canceled', 'unpaid', 'incomplete', 'incomplete_expired')),

  current_period_start TIMESTAMPTZ,
  current_period_end TIMESTAMPTZ,
  cancel_at_period_end BOOLEAN NOT NULL DEFAULT FALSE,
  canceled_at TIMESTAMPTZ,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX subscriptions_tenant_idx ON subscriptions(tenant_id);
CREATE INDEX subscriptions_status_idx ON subscriptions(status);

-- Stripe webhook events (idempotency for webhook handling)
CREATE TABLE stripe_events (
  event_id TEXT PRIMARY KEY,        -- Stripe event.id
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX stripe_events_type_idx ON stripe_events(event_type, processed_at DESC);

-- Audit log: every dashboard-side action that changes tenant state
CREATE TABLE audit_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  api_key_id UUID REFERENCES api_keys(id) ON DELETE SET NULL,

  action TEXT NOT NULL,            -- "tenant.settings.update", "destination.connect", etc.
  resource TEXT,                   -- "tenant", "destination:<id>", "session:<id>"
  metadata JSONB,

  ip INET,
  user_agent TEXT,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_logs_tenant_created_idx ON audit_logs(tenant_id, created_at DESC);
CREATE INDEX audit_logs_user_idx ON audit_logs(user_id, created_at DESC) WHERE user_id IS NOT NULL;

-- Helper: updated_at trigger function used by all tables
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply the trigger to every table that has updated_at
CREATE TRIGGER tenants_updated_at BEFORE UPDATE ON tenants
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER whatsapp_sessions_updated_at BEFORE UPDATE ON whatsapp_sessions
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER leads_updated_at BEFORE UPDATE ON leads
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER destinations_updated_at BEFORE UPDATE ON destinations
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER subscriptions_updated_at BEFORE UPDATE ON subscriptions
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS subscriptions_updated_at ON subscriptions;
DROP TRIGGER IF EXISTS destinations_updated_at ON destinations;
DROP TRIGGER IF EXISTS leads_updated_at ON leads;
DROP TRIGGER IF EXISTS whatsapp_sessions_updated_at ON whatsapp_sessions;
DROP TRIGGER IF EXISTS users_updated_at ON users;
DROP TRIGGER IF EXISTS tenants_updated_at ON tenants;
DROP FUNCTION IF EXISTS set_updated_at();
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS stripe_events;
DROP TABLE IF EXISTS subscriptions;
-- +goose StatementEnd
