# Build stage
FROM golang:1.25.5-alpine3.22@sha256:3587db7cc96576822c606d119729370dbf581931c5f43ac6d3fa03ab4ed85a10 AS builder

WORKDIR /app

# Install git (required for go mod operations)
RUN apk add --no-cache git

# Copy dependencies first (better layer caching)
COPY pkg/go.mod pkg/go.sum* ./pkg/
WORKDIR /app/toca-mcp
COPY toca-mcp/go.mod ./go.mod
COPY toca-mcp/go.sum* ./go.sum

# Download dependencies
RUN --mount=type=cache,target=/go/pkg/mod \
    GOWORK=off GOFLAGS=-mod=readonly go mod download

# Copy source code
WORKDIR /app
COPY pkg/ ./pkg/
COPY toca-mcp/ ./toca-mcp/

# Build with optimizations
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd /app/toca-mcp && \
    GOWORK=off CGO_ENABLED=0 GOOS=linux go build -mod=readonly \
    -ldflags="-w -s" \
    -o /server main.go

# Runtime stage
FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /server /server

# Central de Ajuda (fonte única da documentação de uso): o mesmo conteúdo da
# página /ajuda do frontend, servido pelas tools search_help/get_help_topic.
# O build deste Dockerfile usa o contexto da raiz do repositório.
COPY toka/src/help/docs/ /help-corpus/

ENV PORT=8090
ENV GATEWAY_URL=http://gateway:8080
ENV HELP_CORPUS_DIR=/help-corpus

EXPOSE ${PORT}

HEALTHCHECK --interval=15s --timeout=5s --retries=3 --start-period=10s \
    CMD wget --quiet --tries=1 --spider "http://127.0.0.1:${PORT}/ready" || exit 1

ENTRYPOINT ["/server"]
