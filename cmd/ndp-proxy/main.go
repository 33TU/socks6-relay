// Command ndp-proxy answers IPv6 Neighbor Solicitations for a whole prefix with
// Override-flag Neighbor Advertisements, so a routed/on-link prefix (e.g. on
// Contabo) delivers return traffic for addresses the host never configures.
//
// It is the standalone counterpart to the relay's --ndp-proxy flag: run it on
// its own (as a background service) when you want NDP handling independent of
// the SOCKS relay — alongside a different proxy, or always-on regardless of the
// relay's lifecycle.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"socks-ipv6-relay/internal/host"
	"socks-ipv6-relay/internal/ndp"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
	slog.Info("ndp-proxy stopped")
}

func run() error {
	prefix := flag.String("prefix", "", "IPv6 prefix to answer for (required, e.g. 2a01:4f8:...::/64)")
	iface := flag.String("iface", "", "network interface facing the gateway (required, e.g. eth0)")
	skipPreflight := flag.Bool("skip-preflight", false, "skip the host configuration checks")
	logLevel := flag.Int("log-level", 0, "log level (-4=DEBUG, 0=INFO, 4=WARN, 8=ERROR)")
	flag.Parse()

	slog.SetLogLoggerLevel(slog.Level(*logLevel))

	if *prefix == "" {
		return errors.New("missing required --prefix")
	}
	if *iface == "" {
		return errors.New("missing required --iface")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		slog.Info("received signal, shutting down...", "signal", sig)
		cancel()
	}()

	if *skipPreflight {
		slog.Warn("skipping host configuration checks")
	} else if err := host.Preflight(*prefix); err != nil {
		return err
	}

	responder, err := ndp.NewNDPResponder(*prefix, *iface)
	if err != nil {
		return err
	}

	// Runs until ctx is cancelled; restores allmulticast on exit.
	if err := responder.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
