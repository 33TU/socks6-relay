build:
    go build -o bin/socks-ipv6-relay ./cmd/socks-ipv6-relay
    go build -o bin/socks-ipv6-relay-test ./cmd/socks-ipv6-relay-test
    go build -o bin/ndp-proxy ./cmd/ndp-proxy

# requires cap NET_RAW (or root privileges)
run-ndp-proxy *args:
    bin/ndp-proxy {{ args }}

# needs no capabilities; run `just setup-host` first
run-proxy *args:
    bin/socks-ipv6-relay {{ args }}

test-proxy *args:
    bin/socks-ipv6-relay-test {{ args }}

docker-build:
    docker build -t socks-ipv6-relay .

# needs no capabilities; run `just setup-host` on the host first
docker-run *args:
    docker run --rm \
        --network host socks-ipv6-relay {{ args }}

# ndp-proxy needs NET_RAW for its packet socket
docker-run-ndp-proxy *args:
    docker run --rm \
        --cap-add NET_RAW \
        --network host \
        --entrypoint /app/bin/ndp-proxy \
        socks-ipv6-relay {{ args }}

docker-run-test *args:
    docker run --rm \
        --add-host=host.docker.internal:host-gateway \
        --entrypoint /app/bin/socks-ipv6-relay-test \
        socks-ipv6-relay {{ args }}

# one-off host setup for a trial; not persistent across reboots (requires root)
setup-host prefix iface:
    sudo sysctl -w net.ipv6.ip_nonlocal_bind=1
    sudo ip -6 route replace local {{ prefix }} dev {{ iface }} table local

# undo setup-host
teardown-host prefix iface:
    sudo ip -6 route del local {{ prefix }} dev {{ iface }} table local
