package host

import (
	"errors"
	"net"
)

// ErrNotIPv6Prefix is returned when a prefix parses but is not IPv6.
var ErrNotIPv6Prefix = errors.New("prefix must be IPv6")

// ParseIPv6Prefix parses an IPv6 CIDR, returning the address exactly as given
// and the network it belongs to. IPv4 prefixes are rejected.
//
// Both binaries take a --prefix flag and validate it the same way, so the
// parsing lives here rather than being duplicated in each.
func ParseIPv6Prefix(prefix string) (net.IP, *net.IPNet, error) {
	ip, ipnet, err := net.ParseCIDR(prefix)
	if err != nil {
		return nil, nil, err
	}

	if ipnet.IP.To4() != nil {
		return nil, nil, ErrNotIPv6Prefix
	}

	return ip, ipnet, nil
}
