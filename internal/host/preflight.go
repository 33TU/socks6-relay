package host

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"syscall"

	"github.com/vishvananda/netlink"
)

// The relay and ndp-proxy never change host network settings themselves; they
// only verify that the operator has configured them. Both checks below are
// read-only and need no privileges.

// nonLocalBindPath is a variable so tests can point it at a temporary file.
var nonLocalBindPath = "/proc/sys/net/ipv6/ip_nonlocal_bind"

// CheckIPv6NonLocalBind verifies net.ipv6.ip_nonlocal_bind is enabled, without
// which sockets cannot bind addresses that are not configured on an interface.
func CheckIPv6NonLocalBind() error {
	current, err := os.ReadFile(nonLocalBindPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", nonLocalBindPath, err)
	}

	if strings.TrimSpace(string(current)) == "1" {
		slog.Debug("IPv6 non-local bind is enabled")
		return nil
	}

	return fmt.Errorf("net.ipv6.ip_nonlocal_bind is disabled; enable it with: sudo sysctl -w net.ipv6.ip_nonlocal_bind=1")
}

// CheckLocalIPv6Route verifies a local route covering prefix exists in the
// local table, without which the kernel drops inbound packets for generated
// addresses. A route for a shorter prefix that covers this one satisfies it.
//
// iface is used only to make the remediation message concrete.
func CheckLocalIPv6Route(prefix, iface string) error {
	_, ipnet, err := net.ParseCIDR(prefix)
	if err != nil {
		return err
	}
	ones, _ := ipnet.Mask.Size()

	routes, err := netlink.RouteListFiltered(
		netlink.FAMILY_V6,
		&netlink.Route{Table: syscall.RT_TABLE_LOCAL},
		netlink.RT_FILTER_TABLE,
	)
	if err != nil {
		return fmt.Errorf("list local IPv6 routes: %w", err)
	}

	for _, route := range routes {
		if route.Dst == nil || route.Type != syscall.RTN_LOCAL {
			continue
		}

		// The route must cover the whole prefix, so it may not be more specific.
		dstOnes, _ := route.Dst.Mask.Size()
		if dstOnes <= ones && route.Dst.Contains(ipnet.IP) {
			slog.Debug("found covering local route", "prefix", prefix, "route", route.Dst.String())
			return nil
		}
	}

	if iface == "" {
		iface = "<iface>"
	}

	return fmt.Errorf(
		"no local route covering %s in the local table; add it with: sudo ip -6 route add local %s dev %s table local",
		prefix, ipnet.String(), iface,
	)
}

// Preflight runs the host checks the data path depends on.
func Preflight(prefix, iface string) error {
	if err := CheckIPv6NonLocalBind(); err != nil {
		return err
	}
	if err := CheckLocalIPv6Route(prefix, iface); err != nil {
		return err
	}

	slog.Info("preflight checks passed", "prefix", prefix)
	return nil
}
