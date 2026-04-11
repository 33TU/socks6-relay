# Build stage
FROM golang:alpine AS builder

WORKDIR /app

COPY . .
RUN go mod tidy
RUN go build -o bin/socks-ipv6-relay cmd/socks-ipv6-relay/*.go
RUN go build -o bin/socks-ipv6-relay-test cmd/socks-ipv6-relay-test/*.go

# Final stage
FROM alpine:latest

WORKDIR /app

RUN apk --no-cache add ca-certificates
COPY --from=builder /app/bin/socks-ipv6-relay /app/bin/socks-ipv6-relay
COPY --from=builder /app/bin/socks-ipv6-relay-test /app/bin/socks-ipv6-relay-test

ENTRYPOINT [ "/app/bin/socks-ipv6-relay" ]