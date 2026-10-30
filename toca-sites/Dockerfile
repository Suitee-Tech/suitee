# toca-sites — microsserviço Go da vitrine (ADR 0006)
#
# Build a partir da RAIZ do monorepo (context: .), como os demais serviços:
#   docker build -f toca-sites/Dockerfile -t toca-sites .
FROM golang:1.25.5-alpine3.22@sha256:3587db7cc96576822c606d119729370dbf581931c5f43ac6d3fa03ab4ed85a10 AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY pkg/go.mod pkg/go.sum ./pkg/
COPY toca-sites/go.mod toca-sites/go.sum* ./toca-sites/

WORKDIR /app/toca-sites
RUN --mount=type=cache,target=/go/pkg/mod \
    GOWORK=off GOFLAGS=-mod=readonly go mod download

WORKDIR /app
COPY pkg/ ./pkg/
COPY toca-sites/ ./toca-sites/

WORKDIR /app/toca-sites
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOWORK=off CGO_ENABLED=0 GOOS=linux go build -mod=readonly \
    -ldflags="-w -s" \
    -o /server ./cmd/server

FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce

WORKDIR /app
RUN addgroup -S app && adduser -S -G app app

COPY --from=builder /server ./server

# Diretório compartilhado com o Traefik. O Docker inicializa um volume nomeado
# vazio com o owner/mode deste diretório, permitindo que o usuário app escreva.
RUN mkdir -p /var/lib/toca/tls && chown -R app:app /var/lib/toca/tls

USER app
EXPOSE 8094

ENTRYPOINT ["/app/server"]
