package internal

import (
	"errors"
	"log/slog"
	"net"
	"os"
	"strings"
	"syscall"

	"github.com/vishvananda/netlink"
)

// EnableIPv6NonLocalBind enables binding to non-local IPv6 addresses
func EnableIPv6NonLocalBind() error {
	slog.Debug("checking IPv6 non-local bind status")

	// Check if already enabled
	current, err := os.ReadFile("/proc/sys/net/ipv6/ip_nonlocal_bind")
	if err != nil {
		slog.Warn("failed to read IPv6 non-local bind status", "error", err)
	} else if strings.Contains(string(current), "1") {
		slog.Debug("IPv6 non-local bind already enabled")
		return nil
	}

	slog.Debug("enabling IPv6 non-local bind")
	return os.WriteFile(
		"/proc/sys/net/ipv6/ip_nonlocal_bind",
		[]byte("1"),
		0644,
	)
}

// AddLocalIPv6Route ensures: local <prefix> dev lo exists.
// Returns (created, error); created is false if the route already existed.
func AddLocalIPv6Route(cidr, iface string) (bool, error) {
	slog.Debug("adding local IPv6 route", "cidr", cidr, "interface", iface)

	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, err
	}

	link, err := netlink.LinkByName(iface)
	if err != nil {
		return false, err
	}

	route := &netlink.Route{
		Dst:       ipnet,
		LinkIndex: link.Attrs().Index,
		Type:      syscall.RTN_LOCAL,
		Table:     255,
	}

	err = netlink.RouteAdd(route)
	if err == nil {
		return true, nil // created
	}
	if errors.Is(err, syscall.EEXIST) {
		return false, nil // pre-existing, not ours to remove
	}
	return false, err
}

// RemoveLocalIPv6Route removes: local <prefix> dev lo.
// Returns (removed, error)
func RemoveLocalIPv6Route(cidr, iface string) (bool, error) {
	slog.Debug("removing local IPv6 route", "cidr", cidr, "interface", iface)

	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, err
	}

	link, err := netlink.LinkByName(iface)
	if err != nil {
		return false, err
	}

	route := &netlink.Route{
		Dst:       ipnet,
		LinkIndex: link.Attrs().Index,
		Type:      syscall.RTN_LOCAL,
		Table:     255,
	}

	err = netlink.RouteDel(route)
	if err == nil {
		return true, nil // removed
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil // already gone
	}
	return false, err
}
