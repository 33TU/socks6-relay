package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"socks-ipv6-relay/internal"
)

func main() {
	// CLI flags
	addr := flag.String("listen", ":1080", "SOCKS4a and SOCKS5 server listen address")
	username := flag.String("user", "", "username (optional)")
	password := flag.String("pass", "", "password (optional)")
	prefix := flag.String("prefix", "", "IPv6 prefix (required, e.g. 2a01:4f8:...::/64)")
	iface := flag.String("iface", "", "Network interface (required) (e.g. enp0s31f6)")
	random := flag.Bool("random", true, "use random IPv6 (default true, false = incremental)")
	setupIPv6Routes := flag.Bool("setup-ipv6-routes", true, "automatically setup IPv6 route (default true)")
	setupIPv6LocalBind := flag.Bool("setup-ipv6-local-bind", true, "automatically setup IPv6 local bind (default true)")
	logLevel := flag.Int("log-level", 0, "log level (-4=DEBUG, 0=INFO, 4=ERROR, 8=WARN)")
	tcpTimeout := flag.Duration("tcp-timeout", 60*time.Second, "TCP timeout for connect operations")

	flag.Parse()

	// set log level
	slog.SetLogLoggerLevel(slog.Level(*logLevel))

	// validation
	if *prefix == "" {
		slog.Error("missing required --prefix")
		os.Exit(1)
	}

	// context + shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		slog.Info("received signal, shutting down...", "signal", sig)
		cancel()
	}()

	// setup IPv6 route
	if *setupIPv6Routes {
		routeAdded, err := internal.AddLocalIPv6Route(*prefix, *iface)
		if err != nil {
			slog.Error("failed to add local IPv6 route", "error", err)
			os.Exit(1)
		}
		if routeAdded {
			defer func() {
				removed, err := internal.RemoveLocalIPv6Route(*prefix, *iface)
				if err != nil {
					slog.Error("failed to remove local IPv6 route", "error", err)
				} else if removed {
					slog.Info("IPv6 route removed", "prefix", *prefix)
				}
			}()
		}
		slog.Info("IPv6 route ready", "prefix", *prefix)
	}

	// setup non-local bind
	if *setupIPv6LocalBind {
		if err := internal.EnableIPv6NonLocalBind(); err != nil {
			slog.Error("failed to enable non local bind", "error", err)
			os.Exit(1)
		}
		slog.Info("enabled non local bind")
	}

	// generator
	gen, err := internal.NewIPv6Generator(*prefix, *random)
	if err != nil {
		slog.Error("failed to create IPv6 generator", "error", err)
		os.Exit(1)
	}

	slog.Info("IPv6 generator initialized", "random", *random)

	// start server
	slog.Info("SOCKS4a and SOCKS5 server listening on", "address", *addr)

	err = internal.ListenAndServeSocks(
		ctx,
		"tcp",
		*addr,
		*username,
		*password,
		*tcpTimeout,
		gen,
	)

	if err != nil && err != context.Canceled {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}
