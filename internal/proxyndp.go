package internal

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/vishvananda/netlink"
)

// ProxyNDP publishes proxy NDP entries so the host answers Neighbor
// Solicitations for addresses that are not configured on any interface.
//
// This is only needed when the prefix is on-link, i.e. the upstream gateway
// sits inside the prefix and resolves every destination address with a
// Neighbor Solicitation. Without an answer the router drops the return
// traffic and connections hang. On a routed prefix (the gateway is off-prefix,
// typically fe80::1) the router forwards the whole prefix without resolving
// individual addresses and this is unnecessary.
type ProxyNDP struct {
	iface     string
	linkIndex int

	// add and del are fields so the reference counting can be tested without
	// the NET_ADMIN privileges the netlink calls require.
	add func(*netlink.Neigh) error
	del func(*netlink.Neigh) error

	mu   sync.Mutex
	refs map[string]*proxyNDPEntry
}

type proxyNDPEntry struct {
	count int
	// owned is false when the entry already existed, in which case releasing
	// it must not remove an entry some other process is relying on.
	owned bool
}

// NewProxyNDP enables proxy NDP on iface and returns a publisher for it.
func NewProxyNDP(iface string) (*ProxyNDP, error) {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return nil, err
	}

	if err := enableProxyNDP(iface); err != nil {
		return nil, err
	}

	slog.Debug("proxy NDP ready", "interface", iface, "link_index", link.Attrs().Index)

	return &ProxyNDP{
		iface:     iface,
		linkIndex: link.Attrs().Index,
		add:       netlink.NeighAdd,
		del:       netlink.NeighDel,
		refs:      make(map[string]*proxyNDPEntry),
	}, nil
}

// Acquire publishes a proxy NDP entry for ip, reference counted so the same
// address may be in use by several concurrent connections.
func (p *ProxyNDP) Acquire(ip net.IP) error {
	key := ip.String()

	p.mu.Lock()
	defer p.mu.Unlock()

	if entry, ok := p.refs[key]; ok {
		entry.count++
		return nil
	}

	owned := true
	if err := p.add(p.neigh(ip)); err != nil {
		if !errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("add proxy NDP entry for %s: %w", key, err)
		}
		owned = false // pre-existing, leave it alone on release
	}

	p.refs[key] = &proxyNDPEntry{count: 1, owned: owned}
	slog.Debug("published proxy NDP entry", "ip", key, "interface", p.iface, "owned", owned)

	return nil
}

// Release drops a reference to ip, withdrawing the proxy NDP entry once the
// last connection using it is gone.
func (p *ProxyNDP) Release(ip net.IP) {
	key := ip.String()

	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.refs[key]
	if !ok {
		return
	}

	entry.count--
	if entry.count > 0 {
		return
	}

	delete(p.refs, key)
	p.withdraw(ip, entry)
}

// Close withdraws every entry still published.
func (p *ProxyNDP) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, entry := range p.refs {
		if ip := net.ParseIP(key); ip != nil {
			p.withdraw(ip, entry)
		}
		delete(p.refs, key)
	}
}

// withdraw removes an entry this process published. Callers must hold p.mu.
func (p *ProxyNDP) withdraw(ip net.IP, entry *proxyNDPEntry) {
	if !entry.owned {
		return
	}

	if err := p.del(p.neigh(ip)); err != nil && !errors.Is(err, syscall.ENOENT) {
		slog.Warn("failed to remove proxy NDP entry", "ip", ip.String(), "interface", p.iface, "error", err)
		return
	}

	slog.Debug("withdrew proxy NDP entry", "ip", ip.String(), "interface", p.iface)
}

func (p *ProxyNDP) neigh(ip net.IP) *netlink.Neigh {
	return &netlink.Neigh{
		LinkIndex: p.linkIndex,
		Family:    netlink.FAMILY_V6,
		Flags:     netlink.NTF_PROXY,
		IP:        ip,
	}
}

// enableProxyNDP sets net.ipv6.conf.{all,<iface>}.proxy_ndp.
func enableProxyNDP(iface string) error {
	for _, scope := range []string{"all", iface} {
		path := "/proc/sys/net/ipv6/conf/" + scope + "/proxy_ndp"

		current, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(current)) == "1" {
			continue
		}

		if err := os.WriteFile(path, []byte("1"), 0644); err != nil {
			return fmt.Errorf("enable proxy_ndp on %s: %w", scope, err)
		}

		slog.Debug("enabled proxy NDP", "scope", scope)
	}

	return nil
}
