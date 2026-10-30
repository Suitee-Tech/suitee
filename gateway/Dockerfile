# Build stage
FROM golang:1.25.5-alpine3.22@sha256:3587db7cc96576822c606d119729370dbf581931c5f43ac6d3fa03ab4ed85a10 AS builder

WORKDIR /app

# Install git (required for go mod operations)
RUN apk add --no-cache git

# Copy dependencies first (better layer caching)
COPY pkg/go.mod pkg/go.sum* ./pkg/
COPY gateway/go.mod ./gateway/
COPY gateway/go.sum* ./gateway/

# Download dependencies
WORKDIR /app/gateway
RUN --mount=type=cache,target=/go/pkg/mod \
    GOWORK=off GOFLAGS=-mod=readonly go mod download

# Copy source code
WORKDIR /app
COPY pkg/ ./pkg/
COPY gateway/ ./gateway/

# Build with optimizations (static binary, no cgo)
WORKDIR /app/gateway
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOWORK=off CGO_ENABLED=0 GOOS=linux go build -mod=readonly \
    -ldflags="-w -s" \
    -o /server ./cmd/gateway

# Runtime stage - minimal scratch image for true minimal size
FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce

COPY --from=builder /server /server

EXPOSE 8080

ENTRYPOINT ["/server"]
