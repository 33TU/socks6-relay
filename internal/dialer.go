package internal

import (
	"context"
	"net"
	"sync"
)

// Dialer dials outbound connections using generated local IPv6 addresses.
type Dialer struct {
	Generator *IPv6Generator

	// ProxyNDP, when set, publishes a proxy NDP entry for the local address
	// for as long as the connection is open. Required on on-link prefixes.
	ProxyNDP *ProxyNDP
}

// DialContext dials a network address using the next generated local IPv6 address.
func (d *Dialer) DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	localIP := d.Generator.Next()

	if d.ProxyNDP != nil {
		if err := d.ProxyNDP.Acquire(localIP); err != nil {
			return nil, err
		}
	}

	dialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: localIP,
		},
	}

	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		if d.ProxyNDP != nil {
			d.ProxyNDP.Release(localIP)
		}
		return nil, err
	}

	if d.ProxyNDP == nil {
		return conn, nil
	}

	return &releaseConn{Conn: conn, release: func() { d.ProxyNDP.Release(localIP) }}, nil
}

// ListenPacket opens a UDP socket bound to the next generated local IPv6
// address, holding a proxy NDP entry for it while the socket is open.
func (d *Dialer) ListenPacket(ctx context.Context) (net.PacketConn, error) {
	localIP := d.Generator.Next()

	if d.ProxyNDP != nil {
		if err := d.ProxyNDP.Acquire(localIP); err != nil {
			return nil, err
		}
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: localIP})
	if err != nil {
		if d.ProxyNDP != nil {
			d.ProxyNDP.Release(localIP)
		}
		return nil, err
	}

	if d.ProxyNDP == nil {
		return conn, nil
	}

	return &releasePacketConn{PacketConn: conn, release: func() { d.ProxyNDP.Release(localIP) }}, nil
}

// releaseConn releases a held resource when the connection is closed.
type releaseConn struct {
	net.Conn
	release func()
	once    sync.Once
}

func (c *releaseConn) Close() error {
	defer c.once.Do(c.release)
	return c.Conn.Close()
}

// releasePacketConn releases a held resource when the packet connection is closed.
type releasePacketConn struct {
	net.PacketConn
	release func()
	once    sync.Once
}

func (c *releasePacketConn) Close() error {
	defer c.once.Do(c.release)
	return c.PacketConn.Close()
}
