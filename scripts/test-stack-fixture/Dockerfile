FROM golang:1.25.5-alpine3.22@sha256:3587db7cc96576822c606d119729370dbf581931c5f43ac6d3fa03ab4ed85a10 AS builder
WORKDIR /fixture
COPY main.go .
RUN CGO_ENABLED=0 go build -o /fixture-server main.go

FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce
COPY --from=builder /fixture-server /fixture-server
ENTRYPOINT ["/fixture-server"]
CMD ["serve"]
