# Build stage
FROM golang:1.25.5-alpine3.22@sha256:3587db7cc96576822c606d119729370dbf581931c5f43ac6d3fa03ab4ed85a10 AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY pkg/go.mod pkg/go.sum* ./pkg/
WORKDIR /app/toca-ai
COPY toca-ai/go.mod ./go.mod
COPY toca-ai/go.sum* ./go.sum

RUN --mount=type=cache,target=/go/pkg/mod \
    GOWORK=off GOFLAGS=-mod=readonly go mod download

WORKDIR /app
COPY pkg/ ./pkg/
COPY toca-ai/ ./toca-ai/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd /app/toca-ai && \
    GOWORK=off CGO_ENABLED=0 GOOS=linux go build -mod=readonly \
    -ldflags="-w -s" \
    -o /server ./cmd/toca-ai

FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce

WORKDIR /app

COPY --from=builder /server ./server

EXPOSE 8092

HEALTHCHECK --interval=15s --timeout=5s --retries=3 --start-period=15s \
    CMD wget --quiet --tries=1 --spider "http://127.0.0.1:8092/ready" || exit 1

ENTRYPOINT ["/app/server"]
