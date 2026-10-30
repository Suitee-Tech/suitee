# Build stage
FROM node:24.22.1-bookworm-slim@sha256:55fb5f5a527ea2c5c4a95f963e48ef90aa38c7b346054fdc8ee9957a2b55d205 AS builder

WORKDIR /app

# Enable pnpm via corepack. Version is pinned through "packageManager" in
# package.json, so corepack reads it from there — do NOT use pnpm@latest
# here; the lockfile and packageManager field define the reviewed toolchain.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN corepack enable
RUN pnpm install --frozen-lockfile

# Build-time env
ARG VITE_APP_VERSION=unknown
ARG VITE_API_URL=/api/v1
ARG VITE_WS_URL
ARG VITE_MEDIA_URL
ARG VITE_IMGPROXY_URL
ARG VITE_MULTI_PARTY_UI
ARG BUILD_MINIFY=esbuild
ENV VITE_APP_VERSION=$VITE_APP_VERSION
ENV VITE_API_URL=$VITE_API_URL
ENV VITE_WS_URL=$VITE_WS_URL
ENV VITE_MEDIA_URL=$VITE_MEDIA_URL
ENV VITE_IMGPROXY_URL=$VITE_IMGPROXY_URL
ENV VITE_MULTI_PARTY_UI=$VITE_MULTI_PARTY_UI
ENV BUILD_MINIFY=$BUILD_MINIFY

# Copy source files (exclude node_modules via .dockerignore)
COPY src ./src
COPY public ./public
# Scripts de validação do bundle (budget + guarda de credencial) rodam no
# build:ci. A pasta inteira vem porque o build depende de scripts novos com
# frequência — copiar arquivo a arquivo já quebrou o build do frontend quando a
# guarda de credencial foi adicionada e ficou de fora desta lista.
COPY scripts ./scripts
COPY index.html vite.config.ts postcss.config.mjs tsconfig.json tsconfig.node.json tsconfig.app.json ./

# Build and validate the exact release bundle. This keeps the Docker artifact
# behind the same bundle budget used by CI.
RUN pnpm run build:ci

# Run stage
FROM nginx:1.31.3-alpine@sha256:4a73073bd557c65b759505da037898b61f1be6cbcc3c2c3aeac22d2a470c1752

# Copy optimized nginx config with caching & compression
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html
# The deploy pipeline publishes the source tree write-once (umask 077 +
# chmod a-w), so copied assets can arrive mode 0400 (root-only). nginx serves
# as the unprivileged `nginx` user and would 403 on them; make the tree
# world-readable (and directories traversable).
RUN chmod -R a+rX /usr/share/nginx/html
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

EXPOSE 80

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1/ || exit 1

ENTRYPOINT ["/docker-entrypoint.sh"]
