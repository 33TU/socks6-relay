package internal

import (
	"encoding/binary"
	"log/slog"
	"math/rand"
	"net"
	"sync/atomic"
)

// IPv6Generator generates IPv6 addresses within a specified CIDR block, either sequentially or randomly.
type IPv6Generator struct {
	base       [16]byte
	maskBits   int // prefix length (e.g. 64, 56, etc)
	random     bool
	counter    uint64
	hostBytes  int // number of bytes available for generation
	hostOffset int // starting byte index for host part
}

// NewIPv6Generator creates a new IPv6 address generator with the given CIDR prefix.
// If random is true, generated addresses will be random within the host space; otherwise, they will be sequential.
func NewIPv6Generator(prefix string, random bool) (*IPv6Generator, error) {
	slog.Debug("creating IPv6 generator", "prefix", prefix, "random", random)

	ip, ipnet, err := net.ParseCIDR(prefix)
	if err != nil {
		return nil, err
	}

	maskBits, _ := ipnet.Mask.Size()

	var base [16]byte
	copy(base[:], ip.To16())

	hostOffset := maskBits / 8
	hostBytes := 16 - hostOffset

	slog.Debug("IPv6 generator created", "prefix", prefix, "mask_bits", maskBits, "host_bytes", hostBytes, "random", random)

	return &IPv6Generator{
		base:       base,
		maskBits:   maskBits,
		random:     random,
		hostBytes:  hostBytes,
		hostOffset: hostOffset,
	}, nil
}

func (g *IPv6Generator) Next() net.IP {
	var ip [16]byte
	copy(ip[:], g.base[:])

	host := ip[g.hostOffset:]

	if g.random {
		g.fillRandom(host)
	} else {
		g.fillIncremental(host)
	}

	return net.IP(ip[:])
}

func (g *IPv6Generator) fillRandom(dst []byte) {
	for i := range dst {
		dst[i] = byte(rand.Uint32())
	}
}

func (g *IPv6Generator) fillIncremental(dst []byte) {
	id := atomic.AddUint64(&g.counter, 1)

	// write counter into last bytes (truncate if needed)
	if len(dst) >= 8 {
		binary.BigEndian.PutUint64(dst[len(dst)-8:], id)
	} else {
		// small host space (e.g. /120)
		for i := range dst {
			dst[len(dst)-1-i] = byte(id >> (8 * i))
		}
	}
}
