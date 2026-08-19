build:
    go build -o bin/socks-ipv6-relay cmd/socks-ipv6-relay/*.go
    go build -o bin/socks-ipv6-relay-test cmd/socks-ipv6-relay-test/*.go

# requires cap NET_ADMIN and cap NET_RAW (or root privileges) and net.ipv6.ip_nonlocal_bind=1
run-proxy *args:
    bin/socks-ipv6-relay {{ args }}

test-proxy *args:
    bin/socks-ipv6-relay-test {{ args }}

docker-build:
    docker build -t socks-ipv6-relay .

docker-run *args:
    docker run --rm \
        --cap-add NET_ADMIN --cap-add NET_RAW \
        --network host socks-ipv6-relay {{ args }}

docker-run-test *args:
    docker run --rm \
        --add-host=host.docker.internal:host-gateway \
        --entrypoint /app/bin/socks-ipv6-relay-test \
        socks-ipv6-relay {{ args }}
