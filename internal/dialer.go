package internal

import (
	"context"
	"net"
)

// Dialer dials outbound connections using generated local IPv6 addresses.
type Dialer struct {
	Generator *IPv6Generator
}

// DialContext dials a network address using the next generated local IPv6 address.
func (d *Dialer) DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	localIP := d.Generator.Next()

	dialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: localIP,
		},
	}

	return dialer.DialContext(ctx, network, address)
}
