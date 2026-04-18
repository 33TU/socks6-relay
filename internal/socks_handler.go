package internal

import (
	"context"
	"errors"
	"net"

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

// NewServerHandler creates a new ServerHandler with the given options.
func NewServerHandler(opts Options) *proxy.ServerHandler {
	dialer := &Dialer{
		Generator: opts.IPv6Generator,
	}

	socks4Handler := &socks4.BaseServerHandler{
		Dialer:             dialer,
		AllowConnect:       opts.AllowConnect,
		ConnectConnTimeout: opts.ConnectTimeout,
	}

	socks5Handler := &socks5.BaseServerHandler{
		Dialer:              dialer,
		AllowConnect:        opts.AllowConnect,
		AllowUDPAssociate:   opts.AllowUDPAssociate,
		ConnectConnTimeout:  opts.ConnectTimeout,
		UDPAssociateTimeout: opts.UDPAssociateTimeout,

		UDPAssociateAddrs: func(ctx context.Context, conn net.Conn, req *socks5.Request) (relayAddr *net.UDPAddr, outAddr *net.UDPAddr, advertiseAddr *net.UDPAddr, err error) {
			outAddr = &net.UDPAddr{
				IP: dialer.Generator.Next(),
			}

			if opts.UDPAssociateAdvertiseAddr != "" {
				advertiseAddr, err = net.ResolveUDPAddr("udp", opts.UDPAssociateAdvertiseAddr)
				if err != nil {
					return nil, nil, nil, err
				}
			}

			return
		},
	}

	if opts.Username != "" && opts.Password != "" {
		userPass4 := opts.Username + ":" + opts.Password

		socks4Handler.UserIDChecker = func(ctx context.Context, userID string) error {
			if userID == userPass4 {
				return nil
			}
			return ErrInvalidCredentials
		}

		socks5Handler.UserPassAuthenticator = func(ctx context.Context, u string, p string) error {
			if u == opts.Username && p == opts.Password {
				return nil
			}
			return ErrInvalidCredentials
		}
	}

	return &proxy.ServerHandler{
		Socks4: socks4Handler,
		Socks5: socks5Handler,
	}
}
