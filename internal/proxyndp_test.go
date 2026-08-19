package internal

import (
	"net"
	"sync"
	"syscall"
	"testing"

	"github.com/vishvananda/netlink"
)

// fakeNeigh records the proxy NDP entries a ProxyNDP would publish.
type fakeNeigh struct {
	mu      sync.Mutex
	live    map[string]int // net effect: +1 added, 0 removed
	adds    int
	dels    int
	addErr  error
	present map[string]bool // entries that already exist in the kernel
}

func newFakeNeigh() *fakeNeigh {
	return &fakeNeigh{live: map[string]int{}, present: map[string]bool{}}
}

func (f *fakeNeigh) add(n *netlink.Neigh) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if n.Flags != netlink.NTF_PROXY {
		return syscall.EINVAL
	}
	if f.present[n.IP.String()] {
		return syscall.EEXIST
	}
	if f.addErr != nil {
		return f.addErr
	}

	f.adds++
	f.live[n.IP.String()]++

	return nil
}

func (f *fakeNeigh) del(n *netlink.Neigh) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.dels++
	f.live[n.IP.String()]--

	return nil
}

func (f *fakeNeigh) liveCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	n := 0
	for _, v := range f.live {
		if v > 0 {
			n++
		}
	}
	return n
}

func newTestProxyNDP(f *fakeNeigh) *ProxyNDP {
	return &ProxyNDP{
		iface: "test0",
		add:   f.add,
		del:   f.del,
		refs:  map[string]*proxyNDPEntry{},
	}
}

func TestProxyNDPRefCounting(t *testing.T) {
	f := newFakeNeigh()
	p := newTestProxyNDP(f)
	ip := net.ParseIP("2a01:4f9:abcd:1234::1")

	for i := 0; i < 3; i++ {
		if err := p.Acquire(ip); err != nil {
			t.Fatal(err)
		}
	}
	if f.adds != 1 {
		t.Fatalf("expected 1 netlink add for 3 acquires, got %d", f.adds)
	}

	p.Release(ip)
	p.Release(ip)
	if f.dels != 0 {
		t.Fatalf("entry withdrawn while still referenced (%d dels)", f.dels)
	}

	p.Release(ip)
	if f.dels != 1 {
		t.Fatalf("expected 1 netlink del after last release, got %d", f.dels)
	}
	if f.liveCount() != 0 {
		t.Fatalf("expected no live entries, got %d", f.liveCount())
	}
}

func TestProxyNDPDistinctAddresses(t *testing.T) {
	f := newFakeNeigh()
	p := newTestProxyNDP(f)

	ips := []net.IP{
		net.ParseIP("2a01:4f9:abcd:1234::1"),
		net.ParseIP("2a01:4f9:abcd:1234::2"),
		net.ParseIP("2a01:4f9:abcd:1234::3"),
	}
	for _, ip := range ips {
		if err := p.Acquire(ip); err != nil {
			t.Fatal(err)
		}
	}
	if f.liveCount() != 3 {
		t.Fatalf("expected 3 live entries, got %d", f.liveCount())
	}

	p.Close()
	if f.liveCount() != 0 {
		t.Fatalf("Close left %d entries behind", f.liveCount())
	}
}

// A pre-existing kernel entry must not be removed on release: something else
// on the host may depend on it.
func TestProxyNDPLeavesPreExistingEntries(t *testing.T) {
	f := newFakeNeigh()
	ip := net.ParseIP("2a01:4f9:abcd:1234::1")
	f.present[ip.String()] = true

	p := newTestProxyNDP(f)
	if err := p.Acquire(ip); err != nil {
		t.Fatalf("EEXIST should not be an error: %v", err)
	}

	p.Release(ip)
	if f.dels != 0 {
		t.Fatalf("removed a pre-existing entry (%d dels)", f.dels)
	}
}

func TestProxyNDPAcquireErrorPropagates(t *testing.T) {
	f := newFakeNeigh()
	f.addErr = syscall.EPERM

	p := newTestProxyNDP(f)
	if err := p.Acquire(net.ParseIP("2a01:4f9:abcd:1234::1")); err == nil {
		t.Fatal("expected error from failing netlink add")
	}
	if len(p.refs) != 0 {
		t.Fatalf("failed acquire left %d refs behind", len(p.refs))
	}
}

func TestProxyNDPReleaseUnknownIsNoop(t *testing.T) {
	f := newFakeNeigh()
	p := newTestProxyNDP(f)

	p.Release(net.ParseIP("2a01:4f9:abcd:1234::9"))
	if f.dels != 0 {
		t.Fatalf("releasing an unheld address issued %d dels", f.dels)
	}
}

func TestProxyNDPConcurrentAcquireRelease(t *testing.T) {
	f := newFakeNeigh()
	p := newTestProxyNDP(f)
	ip := net.ParseIP("2a01:4f9:abcd:1234::1")

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.Acquire(ip); err != nil {
				t.Error(err)
				return
			}
			p.Release(ip)
		}()
	}
	wg.Wait()

	if f.liveCount() != 0 {
		t.Fatalf("expected no live entries after balanced acquire/release, got %d", f.liveCount())
	}
	if len(p.refs) != 0 {
		t.Fatalf("expected empty ref table, got %d", len(p.refs))
	}
}

func TestReleaseConnReleasesOnceOnClose(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()

	released := 0
	c := &releaseConn{Conn: a, release: func() { released++ }}

	if released != 0 {
		t.Fatal("released before Close")
	}
	_ = c.Close()
	_ = c.Close()

	if released != 1 {
		t.Fatalf("expected exactly 1 release across 2 Closes, got %d", released)
	}
}
