# syntax=docker/dockerfile:1.7

FROM node:24.22.1-alpine AS base
RUN apk add --no-cache libc6-compat
RUN npm install --global pnpm@10.33.0
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1

# ---------------------------------------------------------------------------
# Dependencies
# ---------------------------------------------------------------------------
FROM base AS deps
COPY package.json pnpm-lock.yaml .npmrc ./
RUN pnpm install --frozen-lockfile

# ---------------------------------------------------------------------------
# Build
#
# The public values are baked into the client bundle by Next.js. The database
# placeholders exist only so the Payload config can be evaluated during build;
# no connection is made and they are not copied to the runtime stage.
# ---------------------------------------------------------------------------
FROM base AS builder
COPY --from=deps /app/node_modules ./node_modules
COPY . .

ARG NEXT_PUBLIC_SITES_API_URL=""
# Default local suffix keeps `next build` safe even when no build arg is
# passed; production overrides it at runtime via PUBLIC_SITE_HOST_SUFFIX.
ARG NEXT_PUBLIC_SITE_HOST_SUFFIX="imoveis.localhost"
ARG PUBLIC_SITE_HOST_SUFFIX="imoveis.localhost"

ENV NEXT_PUBLIC_SITES_API_URL=${NEXT_PUBLIC_SITES_API_URL}
ENV NEXT_PUBLIC_SITE_HOST_SUFFIX=${NEXT_PUBLIC_SITE_HOST_SUFFIX}
ENV PUBLIC_SITE_HOST_SUFFIX=${PUBLIC_SITE_HOST_SUFFIX}
ENV DATABASE_URI=postgresql://build:build@127.0.0.1:5432/build
ENV PAYLOAD_SECRET=build-only-secret-build-only-secret-1234
ENV PAYLOAD_PUBLIC_SERVER_URL=http://localhost:3000
ENV PAYLOAD_DB_PUSH=false

RUN pnpm build

# ---------------------------------------------------------------------------
# Migrator
#
# Payload's schema migrations are versioned in src/migrations. The runtime
# image is Next standalone and does NOT ship the Payload CLI/drizzle-kit, so
# schema changes run in this dedicated one-shot image before the web container
# starts. `docker compose run --rm toca-web-migrate` / the compose service
# `toca-web-migrate` uses this target.
# ---------------------------------------------------------------------------
FROM base AS migrator
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PAYLOAD_DB_PUSH=false
CMD ["pnpm", "payload", "migrate"]

# ---------------------------------------------------------------------------
# Runtime
# ---------------------------------------------------------------------------
FROM node:24.22.1-alpine AS runner
RUN apk add --no-cache libc6-compat
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
ENV HOSTNAME=0.0.0.0
ENV MEDIA_DIR=/app/media

RUN addgroup --system --gid 1001 nodejs \
  && adduser --system --uid 1001 nextjs

COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

RUN mkdir -p /app/media /app/.next/cache \
  && chown -R nextjs:nodejs /app/media /app/.next/cache

USER nextjs

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD node -e "fetch('http://127.0.0.1:3000/api/health').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"

# Next standalone server. The public and .next/static directories are copied
# above; server.js already contains the traced runtime dependencies.
CMD ["node", "server.js"]
