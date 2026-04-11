package internal

import (
	"context"
	"net"

	"github.com/33TU/socks/proxy"
	"github.com/33TU/socks/socks4"
	"github.com/33TU/socks/socks5"
)

const (
	defaultTCPBufferSize = 32 * 1024 // 32KB (same as io.CopyBuffer's default buffer size)
)

// NewServerHandler creates a new ServerHandler with the given parameters.
func NewServerHandler(ctx context.Context,
	network string, addr string,
	username string, password string,
	tcpTimeout int,
	generator *IPv6Generator,
) *proxy.ServerHandler {
	// SOCKS4
	socks4Handler := &socks4Handler{
		ctx:       ctx,
		generator: generator,
	}
	socks4Handler.AllowConnect = true

	// SOCKS5
	socks5Handler := &socks5Handler{
		ctx:       ctx,
		generator: generator,
	}
	socks5Handler.AllowConnect = true

	// multi-protocol handler
	return &proxy.ServerHandler{
		Socks4: socks4Handler,
		Socks5: socks5Handler,
	}
}

// socks5Handler implements the socks5.Handler interface for handling SOCKS5 requests.
type socks5Handler struct {
	ctx       context.Context
	generator *IPv6Generator
	socks5.BaseServerHandler
}

// OnConnect is called when a new SOCKS5 connection is established.
func (s *socks5Handler) OnConnect(ctx context.Context, conn net.Conn, req *socks5.Request) error {
	localIP := s.generator.Next()

	dialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: localIP,
		},
	}

	return socks5.BaseOnConnect(ctx, conn, req, dialer, s.ConnectConnTimeout, s.ConnectBufferSize)
}

// socks4Handler implements the socks4.Handler interface for handling SOCKS4 requests.
type socks4Handler struct {
	ctx       context.Context
	generator *IPv6Generator
	socks4.BaseServerHandler
}

// OnConnect is called when a new SOCKS4 connection is established.
func (s *socks4Handler) OnConnect(ctx context.Context, conn net.Conn, req *socks4.Request) error {
	localIP := s.generator.Next()

	dialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: localIP,
		},
	}

	return socks4.BaseOnConnect(ctx, conn, req, dialer, s.ConnectConnTimeout, s.ConnectBufferSize)
}
