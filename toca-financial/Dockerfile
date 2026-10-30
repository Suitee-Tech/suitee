# Build stage
FROM golang:1.25.5-alpine3.22@sha256:3587db7cc96576822c606d119729370dbf581931c5f43ac6d3fa03ab4ed85a10 AS builder

WORKDIR /app

# Install git (required for go mod operations)
RUN apk add --no-cache git

# Copy dependencies first (better layer caching)
COPY pkg/go.mod pkg/go.sum ./pkg/
COPY toca-financial/go.mod ./toca-financial/
COPY toca-financial/go.sum* ./toca-financial/

# Download dependencies
WORKDIR /app/toca-financial
RUN --mount=type=cache,target=/go/pkg/mod \
    GOWORK=off GOFLAGS=-mod=readonly go mod download

# Copy source code
WORKDIR /app
COPY pkg/ ./pkg/
COPY toca-financial/ ./toca-financial/

# Build with optimizations
WORKDIR /app/toca-financial
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOWORK=off CGO_ENABLED=0 GOOS=linux go build -mod=readonly \
    -ldflags="-w -s" \
    -o /server ./cmd/toca-financial

# Runtime stage - minimal scratch image
FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce

WORKDIR /app

COPY --from=builder /server ./server
COPY --from=builder /app/toca-financial/migrations ./migrations

EXPOSE 8083

ENTRYPOINT ["/app/server"]
