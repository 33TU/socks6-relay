# Build stage. Pinned to the build platform so multi-arch builds cross-compile
# natively instead of running the toolchain under emulation.
FROM --platform=$BUILDPLATFORM golang:alpine AS builder

WORKDIR /app

# Resolve dependencies from the committed go.mod/go.sum first so this layer
# is cached across source changes and the build cannot drift from go.sum.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS TARGETARCH
ENV CGO_ENABLED=0
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o bin/socks-ipv6-relay ./cmd/socks-ipv6-relay
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o bin/ndp-proxy ./cmd/ndp-proxy

# Final stage
FROM alpine:latest

WORKDIR /app

RUN apk --no-cache add ca-certificates
COPY --from=builder /app/bin/socks-ipv6-relay /app/bin/socks-ipv6-relay
COPY --from=builder /app/bin/ndp-proxy /app/bin/ndp-proxy

ENTRYPOINT [ "/app/bin/socks-ipv6-relay" ]
