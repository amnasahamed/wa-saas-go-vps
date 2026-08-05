# NiosEngine

Multi-tenant WhatsApp CRM SaaS. Replaces a single-tenant n8n + Node bridge
setup with a Go-based orchestrator (`cmd/api`) and a `whatsmeow`-powered
WhatsApp worker pool (`cmd/bridge`).

## Architecture

```
┌──────────────────────────┐         ┌────────────────────────────────────┐
│  Next.js Dashboard       │         │  Go Orchestrator (cmd/api)         │
│  (Vercel)                │◀─REST──▶│  - HTTP API (chi)                  │
│  - signup, login         │         │  - Inbound webhook from bridge     │
│  - WhatsApp QR linking   │         │  - Extraction pipeline             │
│  - leads table           │         │  - Nurture state machine           │
│  - destinations config   │         │  - Destination sync workers        │
│  - billing portal        │         │  - Stripe webhooks                 │
└──────────────────────────┘         │                                    │
                                      │  State: Postgres + Redis           │
                                      └────────┬──────────────────┬────────┘
                                               │ gRPC             │ gRPC
                                               ▼                  ▼
                                      ┌──────────────────┐  ┌────────────────┐
                                      │ cmd/bridge pool  │  │ AWS S3         │
                                      │ - whatsmeow      │  │ - inbound media│
                                      │ - one Client per │  │ - assets       │
                                      │   tenant session │  └────────────────┘
                                      └──────────────────┘
                                               │
                                               ▼
                                       ┌──────────────────┐
                                       │ WhatsApp         │
                                       │ (per tenant)     │
                                       └──────────────────┘
```

## Repo layout

```
.
├── cmd/
│   ├── api/        Orchestrator HTTP + gRPC server
│   ├── bridge/     whatsmeow worker pool
│   ├── migrate/    goose migration runner
│   └── healthcheck/ static HTTP probe for distroless container healthchecks
├── internal/
│   ├── auth/       JWT, bcrypt, API keys
│   ├── tenants/    Tenant CRUD + settings
│   ├── billing/    Stripe webhooks
│   ├── whatsapp/   whatsmeow wrapper, phone normalization, media helpers
│   ├── messages/   Idempotent message persistence
│   ├── leads/      Lead aggregate + stage state
│   ├── extraction/ GPT-4o extraction pipeline
│   ├── nurture/    Progressive stage machine + reply generator
│   ├── destinations/  Sheets / Airtable / Notion connectors
│   ├── queue/      Redis ZSET delay queue
│   ├── crypto/     Envelope encryption with KMS data keys
│   ├── config/     Runtime configuration loader
│   └── logging/    slog setup
├── migrations/     goose SQL files (run in numeric order)
├── db/
│   ├── sqlc.yaml   sqlc config
│   └── queries/    type-safe SQL → Go
├── proto/          gRPC contracts (bridge.proto)
├── web/            Next.js 14 dashboard (VPS: containerized; AWS: Vercel)
├── deploy/
│   ├── terraform/  AWS infra (VPC, ECS, RDS, Redis, S3, KMS, ECR)
│   └── vps/        Single-VPS deploy: Caddyfile, bootstrap, backup, runbook
└── docker/         Dockerfiles: api/bridge/migrate (Go) + web (Next.js)
```

## Quick start (local dev)

### Prerequisites

- Go 1.26+
- Node 20+ (for the dashboard)
- pnpm (`corepack enable`)
- Docker + Docker Compose (for the dev stack)

### One-shot bringup

```bash
# 1. Bring up Postgres, Redis, LocalStack (S3), MailHog, api, bridge
make up

# 2. Run migrations
make migrate-up

# 3. Open dashboard (optional, in another terminal)
docker compose -f docker-compose.dev.yml --profile frontend up dashboard
```

Dashboard at http://localhost:3000, API at http://localhost:8080/healthz,
Bridge at http://localhost:9090/healthz, MailHog at http://localhost:8025,
LocalStack at http://localhost:4566.

### Working with the Go code

```bash
make build           # build all binaries
make test            # run tests
make lint            # golangci-lint
make fmt             # gofmt
make sqlc            # regenerate internal/db from db/queries
make migrate-up      # apply migrations
make migrate-status  # see applied migrations
```

### Adding a query

1. Add SQL to `db/queries/<domain>.sql` with `-- name: ... :one|:many|:exec`
2. Run `make sqlc` to generate Go in `internal/db/`
3. Use the generated function from your service

### Adding a migration

```bash
make migrate-create NAME=add_user_preferences
# Edit migrations/<NNNNNN>_add_user_preferences.sql
# Then:
make migrate-up
```

## Phases

- **Phase 0** (this commit) — repo scaffold, migrations, package stubs, infra
- **Phase 1** — bridge + inbound persistence + reply send
- **Phase 2** — multi-tenant auth + dashboard + Stripe
- **Phase 3** — extraction + nurture (port `workflownew.json`)
- **Phase 4** — Sheets / Airtable / Notion destinations
- **Phase 5** — polish + launch

See `/Users/amnasahamed/.claude/plans/i-need-you-to-noble-creek.md` for the
full plan.

## Deployment

Two supported targets.

### Single VPS (Docker Compose + Caddy)

Runs everything — api, bridge, Postgres, Redis, and the dashboard — on one
server with automatic HTTPS. See **[deploy/vps/README.md](deploy/vps/README.md)**
for the full runbook.

```bash
sudo bash deploy/vps/bootstrap.sh      # once per server
cp .env.prod.example .env.prod         # then fill in secrets
make prod-up
```

### AWS (Terraform + ECS Fargate)

```bash
cd deploy/terraform
terraform init
terraform plan -out tfplan
terraform apply tfplan
```

CI (`.github/workflows/deploy.yml`) handles image build, ECR push, and ECS
rolling deploy on every push to `main` (or via `workflow_dispatch` for
prod). See `deploy/terraform/README.md` for the per-env workflow.

## Tech stack

- **Language**: Go 1.26 + Node 20 (dashboard)
- **WhatsApp**: [whatsmeow](https://github.com/tulir/whatsmeow)
- **DB**: Postgres 16 + sqlc
- **Cache / queues**: Redis (ElastiCache)
- **Object storage**: S3
- **AI**: OpenAI gpt-4o
- **Auth**: JWT (api) + bcrypt + per-tenant API keys (bridge)
- **Billing**: Stripe
- **gRPC**: google.golang.org/grpc + buf
- **Frontend**: Next.js 14 + shadcn/ui
- **Cloud**: single VPS (Docker Compose + Caddy) or AWS (ECS Fargate + RDS + ElastiCache + S3 + KMS)
- **CI**: GitHub Actions + golangci-lint + sqlc + buf + terraform validate

## Secrets

Tenant OAuth tokens, BYO OpenAI keys, and destination configs are
envelope-encrypted with KMS data keys. See `internal/crypto/`. The DB only
ever sees `BYTEA` ciphertext + AAD bound to the tenant_id.

## What's NOT in this repo yet (Phase 0)

- Real OpenAI integration (Phase 3)
- Real Stripe integration (Phase 2)
- Real Sheets / Airtable / Notion upserts (Phase 4)
- WhatsApp linking flow (Phase 1)
- Dashboard auth flow (Phase 2)
- Per-tenant prompt editor (Phase 5)

The package interfaces and DB schema are in place; implementations follow.
