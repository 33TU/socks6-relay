package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withNonLocalBind(t *testing.T, contents string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ip_nonlocal_bind")
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}

	previous := nonLocalBindPath
	nonLocalBindPath = path
	t.Cleanup(func() { nonLocalBindPath = previous })
}

func TestCheckIPv6NonLocalBindEnabled(t *testing.T) {
	withNonLocalBind(t, "1\n")

	if err := CheckIPv6NonLocalBind(); err != nil {
		t.Fatalf("expected enabled, got %v", err)
	}
}

func TestCheckIPv6NonLocalBindDisabled(t *testing.T) {
	withNonLocalBind(t, "0\n")

	err := CheckIPv6NonLocalBind()
	if err == nil {
		t.Fatal("expected an error when disabled")
	}
	// The message has to tell the operator what to run.
	if !strings.Contains(err.Error(), "sysctl -w net.ipv6.ip_nonlocal_bind=1") {
		t.Fatalf("error is not actionable: %v", err)
	}
}

func TestCheckIPv6NonLocalBindMissingFile(t *testing.T) {
	previous := nonLocalBindPath
	nonLocalBindPath = filepath.Join(t.TempDir(), "absent")
	t.Cleanup(func() { nonLocalBindPath = previous })

	if err := CheckIPv6NonLocalBind(); err == nil {
		t.Fatal("expected an error when the sysctl cannot be read")
	}
}

func TestCheckLocalIPv6RouteRejectsBadPrefix(t *testing.T) {
	if err := CheckLocalIPv6Route("not-a-prefix", "eth0"); err == nil {
		t.Fatal("expected a parse error")
	}
}

// ::1 is in the local table on any IPv6-capable host, so it exercises the
// match path against the real kernel without needing privileges.
func TestCheckLocalIPv6RouteAgainstKernel(t *testing.T) {
	if err := CheckLocalIPv6Route("::1/128", "lo"); err != nil {
		t.Skipf("host has no local ::1 route, skipping: %v", err)
	}

	missing := "2a01:4f9:abcd:1234::/64"
	err := CheckLocalIPv6Route(missing, "eth0")
	if err == nil {
		t.Fatalf("expected %s to be missing from the local table", missing)
	}
	if !strings.Contains(err.Error(), "ip -6 route add local "+missing) {
		t.Fatalf("error is not actionable: %v", err)
	}
}

// A route for a shorter prefix covers a longer one inside it, so ::1/128 being
// present must not be read as covering an unrelated /64.
func TestCheckLocalIPv6RouteDoesNotMatchMoreSpecific(t *testing.T) {
	if err := CheckLocalIPv6Route("::/0", "eth0"); err == nil {
		t.Fatal("a /128 local route must not satisfy a ::/0 requirement")
	}
}
