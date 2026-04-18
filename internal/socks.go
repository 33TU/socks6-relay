package internal

import (
	"context"
	"log/slog"
	"time"

	"github.com/33TU/socks/proxy"
)

// ListenAndServeSocks starts a SOCKS4a and SOCKS5 server with the given parameters and handler.
func ListenAndServeSocks(
	ctx context.Context,
	network string, addr string,
	username string, password string,
	tcpTimeout time.Duration,
	generator *IPv6Generator,
) error {
	slog.Info("creating SOCKS4a and SOCKS5 server", "addr", addr, "tcp_timeout", tcpTimeout)

	handler := NewServerHandler(
		ctx,
		network, addr,
		username, password,
		tcpTimeout,
		generator,
	)

	slog.Info("starting SOCKS4a and SOCKS5 server", "addr", addr)
	errChan := make(chan error, 1)
	go func() {
		errChan <- proxy.ListenAndServe(ctx, network, addr, handler)
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
