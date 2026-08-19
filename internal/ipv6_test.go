package internal

import (
	"net"
	"testing"
)

var prefixes = []string{
	"2a01:4f9:abcd:1234::/64",
	"2a01:4f9:abcd:1200::/56",
	"2a01:4f9:abcd:1230::/60", // not byte-aligned
	"2a01:4f9:abcd::/44",      // not byte-aligned
	"2a01:4f9:abcd:1234::/120",
	"2a01:4f9:abcd:1234::/124", // not byte-aligned
}

func TestNextStaysWithinPrefix(t *testing.T) {
	for _, p := range prefixes {
		for _, random := range []bool{true, false} {
			_, ipnet, err := net.ParseCIDR(p)
			if err != nil {
				t.Fatal(err)
			}
			g, err := NewIPv6Generator(p, random)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2000; i++ {
				ip := g.Next()
				if !ipnet.Contains(ip) {
					t.Fatalf("prefix %s random=%v: generated %s outside prefix", p, random, ip)
				}
			}
		}
	}
}

func TestHostBitsVary(t *testing.T) {
	for _, p := range prefixes {
		for _, random := range []bool{true, false} {
			g, _ := NewIPv6Generator(p, random)
			seen := map[string]bool{}
			for i := 0; i < 64; i++ {
				seen[g.Next().String()] = true
			}
			if len(seen) < 2 {
				t.Fatalf("prefix %s random=%v: generator produced only %d distinct address(es)", p, random, len(seen))
			}
		}
	}
}

func TestRejectsIPv4Prefix(t *testing.T) {
	if _, err := NewIPv6Generator("192.168.1.0/24", true); err != ErrNotIPv6Prefix {
		t.Fatalf("expected ErrNotIPv6Prefix, got %v", err)
	}
}

func TestIgnoresHostBitsInInput(t *testing.T) {
	g, err := NewIPv6Generator("2a01:4f9:abcd:1234::dead:beef/64", false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := g.Next().String(), "2a01:4f9:abcd:1234::1"; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
