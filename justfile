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
    docker build -t socks6-relay .

# needs no capabilities; run `just setup-host` on the host first
docker-run *args:
    docker run --rm \
        --network host socks6-relay {{ args }}

# ndp-proxy needs NET_RAW for its packet socket
docker-run-ndp-proxy *args:
    docker run --rm \
        --cap-add NET_RAW \
        --network host \
        --entrypoint /app/bin/ndp-proxy \
        socks6-relay {{ args }}

# the test helper is not shipped in the image; run it from bin/ instead


# one-off host setup for a trial; not persistent across reboots (requires root)
setup-host prefix iface:
    sudo sysctl -w net.ipv6.ip_nonlocal_bind=1
    sudo ip -6 route replace local {{ prefix }} dev {{ iface }} table local

# undo setup-host
teardown-host prefix iface:
    sudo ip -6 route del local {{ prefix }} dev {{ iface }} table local
