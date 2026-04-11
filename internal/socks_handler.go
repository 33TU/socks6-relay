package internal

import (
	"context"
	"errors"
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
	tcpTimeout time.Duration,
	generator *IPv6Generator,
) *proxy.ServerHandler {
	// Dialer
	dialer := &Dialer{
		Generator: generator,
	}

	// SOCKS4
	socks4Handler := &socks4.BaseServerHandler{
		Dialer:             dialer,
		AllowConnect:       true,
		ConnectConnTimeout: tcpTimeout,
	}

	// SOCKS5
	socks5Handler := &socks5.BaseServerHandler{
		Dialer:             dialer,
		AllowConnect:       true,
		ConnectConnTimeout: tcpTimeout,
	}

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
