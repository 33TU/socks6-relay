package internal

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"

	"github.com/33TU/socks/proxy"
	"github.com/33TU/socks/socks4"
	"github.com/33TU/socks/socks5"
)

const (
	defaultTCPBufferSize = 32 * 1024 // 32KB (same as io.CopyBuffer's default buffer size)
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// NewServerHandler creates a new ServerHandler with the given parameters.
func NewServerHandler(ctx context.Context,
	network string, addr string,
	username string, password string,
	tcpTimeout time.Duration, udpTimeout time.Duration,
	generator *IPv6Generator,
) *proxy.ServerHandler {
	// SOCKS4
	socks4Handler := &socks4Handler{
		ctx:       ctx,
		generator: generator,
	}
	socks4Handler.AllowConnect = true
	socks4Handler.BaseServerHandler.ConnectConnTimeout = tcpTimeout

	// SOCKS5
	socks5Handler := &socks5Handler{
		ctx:       ctx,
		generator: generator,
	}
	socks5Handler.AllowConnect = true
	socks5Handler.AllowUDPAssociate = true
	socks5Handler.BaseServerHandler.ConnectConnTimeout = tcpTimeout

	// Auth
	if username != "" && password != "" {
		userPass4 := username + ":" + password

		socks4Handler.UserIDChecker = func(ctx context.Context, userID string) error {
			if userID == userPass4 {
				return nil
			}
			return ErrInvalidCredentials
		}

		socks5Handler.UserPassAuthenticator = func(ctx context.Context, u string, p string) error {
			if u == username && p == password {
				return nil
			}
			return ErrInvalidCredentials
		}
	}

	// multi-protocol handler
	return &proxy.ServerHandler{
		Socks4: socks4Handler,
		Socks5: socks5Handler,
	}
}

// socks4Handler implements the socks4.Handler interface for handling SOCKS4 requests.
type socks4Handler struct {
	ctx       context.Context
	generator *IPv6Generator
	socks4.BaseServerHandler
}

// OnRequest is called when a new SOCKS4 request is received.
func (d *socks4Handler) OnRequest(ctx context.Context, conn net.Conn, req *socks4.Request) error {
	err := socks4.BaseOnRequest(ctx, d, conn, req)
	if err != nil {
		slog.ErrorContext(ctx, "request handling failed", "error", err, "from", conn.RemoteAddr(), "request", req)
	}
	return err
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

// socks5Handler implements the socks5.Handler interface for handling SOCKS5 requests.
type socks5Handler struct {
	ctx       context.Context
	generator *IPv6Generator
	socks5.BaseServerHandler
}

// OnRequest is called when a new SOCKS5 request is received.
func (d *socks5Handler) OnRequest(ctx context.Context, conn net.Conn, req *socks5.Request) error {
	err := socks5.BaseOnRequest(ctx, d, conn, req)
	if err != nil {
		slog.ErrorContext(ctx, "request handling failed", "error", err, "from", conn.RemoteAddr(), "request", req)
	}
	return err
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

// OnUDPAssociate is called when a new SOCKS5 UDP associate request is received.
func (s *socks5Handler) OnUDPAssociate(ctx context.Context, conn net.Conn, req *socks5.Request) error {
	localIP := s.generator.Next()

	localAddr := &net.UDPAddr{
		IP: localIP,
	}

	return socks5.BaseOnUDPAssociate(ctx, conn, req, s.UDPAssociateTimeout, s.ConnectBufferSize, localAddr)
}
