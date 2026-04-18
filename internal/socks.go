package internal

import (
	"context"
	"log/slog"

	"github.com/33TU/socks/proxy"
)

// ListenAndServeSocks starts a SOCKS4a and SOCKS5 server with the given options.
func ListenAndServeSocks(ctx context.Context, opts Options) error {
	slog.Info(
		"creating SOCKS4a and SOCKS5 server",
		"network", opts.Network,
		"addr", opts.Addr,
		"connect_timeout", opts.ConnectTimeout,
		"udp_associate_timeout", opts.UDPAssociateTimeout,
		"allow_connect", opts.AllowConnect,
		"allow_udp_associate", opts.AllowUDPAssociate,
		"udp_associate_advertise_addr", opts.UDPAssociateAdvertiseAddr,
	)

	handler := NewServerHandler(opts)

	slog.Info(
		"starting SOCKS4a and SOCKS5 server",
		"network", opts.Network,
		"addr", opts.Addr,
	)

	errChan := make(chan error, 1)
	go func() {
		errChan <- proxy.ListenAndServe(ctx, opts.Network, opts.Addr, handler)
		close(errChan)
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		slog.Info("shutting down SOCKS4a and SOCKS5 server due to context cancellation")
		return ctx.Err()
	}
}
