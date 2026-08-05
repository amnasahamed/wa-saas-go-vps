# Deploying NiosEngine to a single VPS

Runs the whole product — Go API, WhatsApp bridge, Postgres, Redis, and the
Next.js dashboard — on one server behind Caddy with automatic HTTPS.

This is an alternative to `deploy/terraform/` (AWS ECS). Both are kept; nothing
here touches the Terraform path.

## What you need

- A VPS running Ubuntu 22.04 or 24.04. **2 GB RAM minimum** — the Next.js build
  is the memory bottleneck; `bootstrap.sh` adds swap to make 1–2 GB survivable,
  but 4 GB is a much better time.
- A domain with an **A record already pointing at the VPS IP**. Caddy cannot
  issue a certificate before DNS resolves.
- A private Git remote holding this repo.

## Architecture

```
                    Internet
                       │  :80 / :443 (only published ports)
                 ┌─────▼─────┐
                 │   caddy   │  auto-TLS via Let's Encrypt
                 └─────┬─────┘
          /api/*       │        everything else
        ┌──────────────┴───────────────┐
        ▼                              ▼
   ┌─────────┐  gRPC :8081 ◀──┐   ┌─────────┐
   │   api   │                │   │   web   │  Next.js standalone
   └────┬────┘  ──▶ :9090 ─┐  │   └─────────┘
        │                  ▼  │
        │            ┌──────────┐
        │            │  bridge  │──▶ WhatsApp
        │            └────┬─────┘    (whatsmeow)
        │                 │
   ┌────▼─────┐     ┌─────▼────┐
   │ postgres │     │  redis   │      ← internal network only,
   └──────────┘     └──────────┘        never published to the host
```

Everything except Caddy is unreachable from the internet.

## First deploy

### 1. Prepare the server

```bash
ssh root@YOUR_VPS_IP
git clone <your-private-repo> /tmp/niosengine   # or scp just this script
sudo bash /tmp/niosengine/deploy/vps/bootstrap.sh
```

Installs Docker, creates a `deploy` user, adds swap, and locks the firewall down
to 22/80/443.

### 2. Configure

```bash
su - deploy
git clone <your-private-repo> niosengine && cd niosengine
cp .env.prod.example .env.prod
```

Edit `.env.prod`:

| Variable | How to fill it |
|---|---|
| `APP_DOMAIN` | your domain, e.g. `app.example.com` |
| `ACME_EMAIL` | your email, for expiry notices |
| `JWT_SECRET` | `openssl rand -base64 48` |
| `POSTGRES_PASSWORD` | `openssl rand -base64 32 \| tr -d '/+='` |
| `DATABASE_URL` | paste the same password into the URL |

`.env.prod` is gitignored. Store a copy in a password manager: losing
`JWT_SECRET` signs every user out, and losing `POSTGRES_PASSWORD` is worse.

### 3. Launch

```bash
make prod-up
make prod-logs
```

First boot builds the images (several minutes), runs migrations, then requests a
certificate. You want to see `certificate obtained successfully` from Caddy.

### 4. Verify

```bash
curl -fsS https://$APP_DOMAIN/api/healthz     # {"status":"ok",...}
curl -fsS https://$APP_DOMAIN/api/readyz      # checks Postgres
make prod-ps                                  # all services Up/healthy
```

Then open `https://your-domain` and sign up.

### 5. Schedule backups

```bash
sudo cp deploy/vps/backup.sh /usr/local/bin/niosengine-backup
sudo chmod +x /usr/local/bin/niosengine-backup
( crontab -l 2>/dev/null; echo "17 3 * * * /usr/local/bin/niosengine-backup" ) | crontab -
```

## Updating

```bash
cd ~/niosengine
git pull
make prod-up        # rebuilds changed images, reruns migrations, restarts
```

Data volumes are untouched by `make prod-down`; only `docker compose down -v`
destroys them, so avoid that flag.

## Operations

| Task | Command |
|---|---|
| Logs (all) | `make prod-logs` |
| Logs (one service) | `docker compose -f docker-compose.prod.yml --env-file .env.prod logs -f bridge` |
| Status | `make prod-ps` |
| Migrations | `make prod-migrate` / `make prod-migrate-status` |
| DB shell | `make prod-psql` |
| Backup now | `make prod-backup` |
| Restart app only | `make prod-restart` |

## Things that will bite you

**The bridge state volume is the only copy of every WhatsApp session.** Device
state (signal keys) lives in per-session SQLite files in the `bridge_state`
volume. There is no S3 backup in the code, despite the `device_state_s3_key`
column suggesting otherwise. Destroy that volume and every tenant must re-scan a
QR code. `backup.sh` archives it nightly.

**Do not change `BRIDGE_WORKER_ID` after go-live.** Sessions are sharded by
worker ID and device state is stored per worker. Renaming it orphans every
existing session.

**Keep the `caddy_data` volume.** It holds issued certificates. Wiping it on
repeated redeploys will hit Let's Encrypt's rate limit (5 duplicate certs per
week) and leave you without TLS until it resets.

**`/readyz` on the bridge returns 503 until a tenant links WhatsApp.** That is
intentional, not a fault. Container healthchecks therefore use `/healthz`.

**Docker bypasses ufw.** This stack only publishes 80/443, so the firewall rules
hold — but if you ever add a host port mapping for Postgres or Redis, ufw will
*not* block it. Don't.

**Backups sit on the same disk as the data.** That covers "I dropped a table",
not "the VPS is gone". Add an off-box copy (restic/rclone) to `backup.sh`.

## Restoring

Postgres:

```bash
cd ~/niosengine
gunzip -c /var/backups/niosengine/pg-<STAMP>.sql.gz | \
  docker compose -f docker-compose.prod.yml --env-file .env.prod \
  exec -T postgres psql -U niosengine -d niosengine
```

Bridge device state (stop the bridge first so it is not writing):

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod stop bridge
docker run --rm -v niosengine_bridge_state:/state -v /var/backups/niosengine:/backup \
  alpine:3 sh -c "rm -rf /state/* && tar xzf /backup/bridge-state-<STAMP>.tar.gz -C /state"
docker compose -f docker-compose.prod.yml --env-file .env.prod start bridge
```

## Adding a second bridge worker

The bridge cannot be replicated behind a load balancer — each worker owns its
tenants' device state locally. To scale, run distinct workers:

1. Copy the `bridge` service block in `docker-compose.prod.yml` to `bridge2`.
2. Give it `BRIDGE_WORKER_ID: worker-2` and its own volume (`bridge2_state`).
3. Add it to `BRIDGE_GRPC_TARGETS`, e.g.
   `worker-1=bridge:9090,worker-2=bridge2:9090`.

Sessions are assigned to a worker via the `whatsapp_sessions.worker_id` column.

## Troubleshooting

**Caddy will not get a certificate.** Check `dig +short $APP_DOMAIN` matches the
VPS IP, and that ports 80/443 are open. Use the staging CA line in the Caddyfile
while iterating to avoid rate limits.

**`api` exits immediately.** Almost always `JWT_SECRET` shorter than 32 bytes —
the API refuses to boot outside dev rather than fall back to a public dev
secret. Check `docker compose ... logs api`.

**Build gets OOM-killed.** The Next.js build needs roughly 2 GB. Confirm swap is
active (`swapon --show`), or build the images elsewhere and push to a registry.

**Migrations fail on first boot.** Check `make prod-migrate-status`. The migrate
container needs `migrations/` inside the image; it is copied in by
`docker/api.Dockerfile`.
