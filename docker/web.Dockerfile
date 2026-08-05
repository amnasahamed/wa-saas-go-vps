# Next.js dashboard (web/) for the VPS deployment.
#
# Replaces the Vercel deploy: builds the standalone server bundle and runs it
# under a non-root user. Requires `output: "standalone"` in web/next.config.mjs.
#
# Build from the REPO ROOT so .dockerignore applies:
#   docker build -f docker/web.Dockerfile -t niosengine/web:prod .

# ---- deps stage: install with a warm pnpm store ----
FROM node:20-alpine AS deps

WORKDIR /app

RUN corepack enable

# Install from the lockfile alone so this layer caches until deps change.
COPY web/package.json web/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/pnpm-store \
    pnpm config set store-dir /pnpm-store && \
    pnpm install --frozen-lockfile

# ---- build stage ----
FROM node:20-alpine AS build

WORKDIR /app

RUN corepack enable

COPY --from=deps /app/node_modules ./node_modules
COPY web/ ./

# NEXT_PUBLIC_* vars are inlined into the client bundle at build time, so any
# that the browser needs must be passed here as build args, not just at runtime.
# The API base URL is only read server-side by next.config.mjs rewrites (Caddy
# owns /api/v1 routing in prod), so it does not need to be baked in.
ARG NEXT_PUBLIC_APP_URL=""
ENV NEXT_PUBLIC_APP_URL=${NEXT_PUBLIC_APP_URL}

ENV NEXT_TELEMETRY_DISABLED=1

# web/ has no public/ dir today. Create it so the runtime COPY below is valid
# now and keeps working if static assets are added later.
RUN mkdir -p /app/public

RUN pnpm build

# ---- runtime stage ----
FROM node:20-alpine AS runtime

LABEL org.opencontainers.image.title="niosengine-web"

WORKDIR /app

ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0

# node:20-alpine ships a "node" user (uid 1000); run as it rather than root.
# The standalone bundle already contains its own minimal node_modules.
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
COPY --from=build --chown=node:node /app/public ./public

USER node

EXPOSE 3000

# wget is present in busybox; used by the compose healthcheck.
CMD ["node", "server.js"]
