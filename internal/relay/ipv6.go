package relay

import (
	"encoding/binary"
	"log/slog"
	"math/rand"
	"net"
	"sync/atomic"

	"socks-ipv6-relay/internal/host"
)

// IPv6Generator generates IPv6 addresses within a specified CIDR block, either sequentially or randomly.
type IPv6Generator struct {
	base       [16]byte // network part, host bits zeroed
	mask       [16]byte // prefix mask
	maskBits   int      // prefix length (e.g. 64, 56, etc)
	random     bool
	counter    uint64
	hostOffset int // first byte index containing any host bits
}

// NewIPv6Generator creates a new IPv6 address generator with the given CIDR prefix.
// If random is true, generated addresses will be random within the host space; otherwise, they will be sequential.
func NewIPv6Generator(prefix string, random bool) (*IPv6Generator, error) {
	slog.Debug("creating IPv6 generator", "prefix", prefix, "random", random)

	ip, ipnet, err := host.ParseIPv6Prefix(prefix)
	if err != nil {
		return nil, err
	}

	maskBits, _ := ipnet.Mask.Size()

	// Keep the prefix bits and zero the host bits, so a prefix that is not
	// byte-aligned (e.g. /60) never leaks host bits into the network part.
	var base, mask [16]byte
	copy(base[:], ip.To16())
	copy(mask[:], ipnet.Mask)
	for i := range base {
		base[i] &= mask[i]
	}

	slog.Debug("IPv6 generator created", "prefix", prefix, "mask_bits", maskBits, "host_bits", 128-maskBits, "random", random)

	return &IPv6Generator{
		base:       base,
		mask:       mask,
		maskBits:   maskBits,
		random:     random,
		hostOffset: maskBits / 8,
	}, nil
}

func (g *IPv6Generator) Next() net.IP {
	var host [16]byte

	if g.random {
		g.fillRandom(host[g.hostOffset:])
	} else {
		g.fillIncremental(host[:])
	}

	ip := g.base
	for i := g.hostOffset; i < len(ip); i++ {
		ip[i] |= host[i] &^ g.mask[i]
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

	// write counter into the low 64 bits; host bits outside the mask are kept
	binary.BigEndian.PutUint64(dst[8:], id)
}
