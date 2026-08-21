# Build stage
FROM golang:alpine AS builder

WORKDIR /app

# Resolve dependencies from the committed go.mod/go.sum first so this layer
# is cached across source changes and the build cannot drift from go.sum.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o bin/socks-ipv6-relay ./cmd/socks-ipv6-relay
RUN go build -o bin/socks-ipv6-relay-test ./cmd/socks-ipv6-relay-test
RUN go build -o bin/ndp-proxy ./cmd/ndp-proxy

# Final stage
FROM alpine:latest

WORKDIR /app

RUN apk --no-cache add ca-certificates
COPY --from=builder /app/bin/socks-ipv6-relay /app/bin/socks-ipv6-relay
COPY --from=builder /app/bin/socks-ipv6-relay-test /app/bin/socks-ipv6-relay-test
COPY --from=builder /app/bin/ndp-proxy /app/bin/ndp-proxy

ENTRYPOINT [ "/app/bin/socks-ipv6-relay" ]
