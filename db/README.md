# Database

Postgres schema lives in `migrations/` (goose format). Generated Go code
lives in `internal/db/` (sqlc).

## Migrations

We use [goose](https://github.com/pressly/goose) for migrations:

```bash
# Install
go install github.com/pressly/goose/v3/cmd/goose@latest

# Run against local Postgres
goose -dir migrations postgres "postgres://postgres:postgres@localhost:5432/niosengine?sslmode=disable" up

# Roll back last
goose -dir migrations postgres "..." down
```

`cmd/migrate` wraps goose so the same migrations run automatically on ECS
deploy (see `cmd/migrate/main.go`).

## sqlc

We use [sqlc](https://sqlc.dev) to generate type-safe Go from raw SQL:

```bash
# Install
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Regenerate
sqlc generate
```

Query files live in `db/queries/*.sql`. Each file is one domain area
(tenants, whatsapp, messages_leads, destinations). Re-generate whenever
you add a query or change the schema.

## UUIDs

All primary keys are `UUID` (Postgres `gen_random_uuid()` from pgcrypto).
sqlc maps these to `github.com/google/uuid.UUID`.

## Idempotency

The `messages.idempotency_key` UNIQUE constraint is the durable replay
guard. Webhook duplicates from the bridge return `ErrDuplicate` and the
orchestrator treats it as a successful no-op.

## Encryption

Tenant secrets (OAuth tokens, BYO OpenAI keys, destination configs) are
envelope-encrypted with KMS data keys. See `internal/crypto/`. The DB only
ever sees `BYTEA` ciphertext + AAD bound to the tenant_id.
