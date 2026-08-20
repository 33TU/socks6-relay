package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"socks-ipv6-relay/internal"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

func run() error {
	addr := flag.String("listen", ":1080", "SOCKS server listen address")
	network := flag.String("network", "tcp", "listen network")
	username := flag.String("user", "", "username (optional)")
	password := flag.String("pass", "", "password (optional)")

	allowConnect := flag.Bool("allow-connect", true, "allow SOCKS CONNECT")
	connectTimeout := flag.Duration("connect-timeout", 60*time.Second, "timeout for CONNECT operations")

	allowUDPAssociate := flag.Bool("allow-udp-associate", true, "allow SOCKS UDP ASSOCIATE")
	udpAssociateAdvertiseAddr := flag.String("udp-associate-advertise-addr", "", "advertised UDP relay address (optional)")
	udpAssociateTimeout := flag.Duration("udp-associate-timeout", 60*time.Second, "timeout for UDP ASSOCIATE operations")

	prefix := flag.String("prefix", "", "IPv6 prefix (required, e.g. 2a01:4f8:...::/64)")
	iface := flag.String("iface", "", "network interface, used only to make preflight messages concrete (e.g. enp0s31f6)")
	random := flag.Bool("random", true, "use random IPv6 (default true, false = incremental)")
	skipPreflight := flag.Bool("skip-preflight", false, "skip the host configuration checks")
	logLevel := flag.Int("log-level", 0, "log level (-4=DEBUG, 0=INFO, 4=WARN, 8=ERROR)")

	flag.Parse()

	slog.SetLogLoggerLevel(slog.Level(*logLevel))

	if *prefix == "" {
		return errors.New("missing required --prefix")
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
	} else if err := internal.Preflight(*prefix, *iface); err != nil {
		return err
	}

	gen, err := internal.NewIPv6Generator(*prefix, *random)
	if err != nil {
		return err
	}

	opts := internal.Options{
		Network:  *network,
		Addr:     *addr,
		Username: *username,
		Password: *password,

		AllowConnect:   *allowConnect,
		ConnectTimeout: *connectTimeout,

		AllowUDPAssociate:         *allowUDPAssociate,
		UDPAssociateAdvertiseAddr: *udpAssociateAdvertiseAddr,
		UDPAssociateTimeout:       *udpAssociateTimeout,

		IPv6Generator: gen,
	}

	slog.Info(
		"starting SOCKS server",
		"network", opts.Network,
		"addr", opts.Addr,
		"allow_connect", opts.AllowConnect,
		"allow_udp_associate", opts.AllowUDPAssociate,
		"udp_advertise_addr", opts.UDPAssociateAdvertiseAddr,
	)

	if err := internal.ListenAndServeSocks(ctx, opts); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}
