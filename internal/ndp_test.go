package internal

import (
	"encoding/binary"
	"net"
	"testing"
)

// buildNS crafts a minimal Ethernet+IPv6+ICMPv6 Neighbor Solicitation frame for
// `target`, sent from `srcMAC`/`srcIP` to the solicited-node multicast.
func buildNS(srcMAC net.HardwareAddr, srcIP, target net.IP) []byte {
	icmp := make([]byte, 0, 24)
	icmp = append(icmp, ndTypeNS, 0, 0, 0) // type, code, checksum(0 for test)
	icmp = append(icmp, 0, 0, 0, 0)        // reserved
	icmp = append(icmp, target.To16()...)  // target address

	ip := make([]byte, 40)
	binary.BigEndian.PutUint32(ip[0:4], 6<<28)
	binary.BigEndian.PutUint16(ip[4:6], uint16(len(icmp)))
	ip[6] = icmpv6Proto
	ip[7] = 255
	copy(ip[8:24], srcIP.To16())
	// solicited-node multicast ff02::1:ffXX:XXXX
	snm := net.IP{0xff, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0xff, target[13], target[14], target[15]}
	copy(ip[24:40], snm)

	eth := make([]byte, 0)
	eth = append(eth, 0x33, 0x33, 0xff, target[13], target[14], target[15]) // dst multicast MAC
	eth = append(eth, srcMAC...)
	eth = append(eth, 0x86, 0xdd)
	eth = append(eth, ip...)
	eth = append(eth, icmp...)
	return eth
}

func testResponder(t *testing.T, cidr string) *NDPResponder {
	t.Helper()
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("parse cidr: %v", err)
	}
	return &NDPResponder{
		ipnet: ipnet,
		mac:   net.HardwareAddr{0x00, 0x50, 0x56, 0x3d, 0x18, 0x07},
	}
}

func TestBuildResponse_AnswersInPrefix(t *testing.T) {
	r := testResponder(t, "2a02:c207:2011:9989::/64")
	gwMAC := net.HardwareAddr{0x00, 0xdc, 0x00, 0x00, 0x00, 0x02}
	gwIP := net.ParseIP("fe80::2dc:ff:fe00:2")
	target := net.ParseIP("2a02:c207:2011:9989::dead:beef")

	na, reqMAC, gotTarget, ok := r.buildResponse(buildNS(gwMAC, gwIP, target))
	if !ok {
		t.Fatal("expected responder to answer NS for an in-prefix target")
	}
	if !gotTarget.Equal(target) {
		t.Fatalf("target mismatch: got %s want %s", gotTarget, target)
	}
	if reqMAC.String() != gwMAC.String() {
		t.Fatalf("reqMAC mismatch: got %s want %s", reqMAC, gwMAC)
	}

	// Ethernet: dst = requester, src = our MAC, ethertype IPv6.
	if net.HardwareAddr(na[0:6]).String() != gwMAC.String() {
		t.Errorf("NA dst MAC = %s, want %s", net.HardwareAddr(na[0:6]), gwMAC)
	}
	if net.HardwareAddr(na[6:12]).String() != r.mac.String() {
		t.Errorf("NA src MAC = %s, want %s", net.HardwareAddr(na[6:12]), r.mac)
	}
	if binary.BigEndian.Uint16(na[12:14]) != ethTypeIPv6 {
		t.Errorf("NA ethertype = %#x, want %#x", binary.BigEndian.Uint16(na[12:14]), ethTypeIPv6)
	}

	ipv6 := na[14:]
	if ipv6[6] != icmpv6Proto || ipv6[7] != 255 {
		t.Errorf("NA IPv6 nexthdr/hoplimit = %d/%d, want %d/255", ipv6[6], ipv6[7], icmpv6Proto)
	}
	// Source address of the NA must be the advertised (target) address.
	if !net.IP(ipv6[8:24]).Equal(target) {
		t.Errorf("NA IPv6 src = %s, want %s", net.IP(ipv6[8:24]), target)
	}

	icmp := ipv6[40:]
	if icmp[0] != ndTypeNA {
		t.Errorf("ICMP type = %d, want %d (NA)", icmp[0], ndTypeNA)
	}
	// Override flag (0x20 in the top flag byte) must be set — the whole point.
	flags := binary.BigEndian.Uint32(icmp[4:8])
	if flags&0x20000000 == 0 {
		t.Errorf("Override flag not set in NA flags %#x", flags)
	}
	if flags&0x40000000 == 0 {
		t.Errorf("Solicited flag not set in NA flags %#x", flags)
	}
	if !net.IP(icmp[8:24]).Equal(target) {
		t.Errorf("NA target = %s, want %s", net.IP(icmp[8:24]), target)
	}
	// Target Link-Layer Address option: type 2, len 1, our MAC.
	if icmp[24] != 2 || icmp[25] != 1 {
		t.Errorf("TLLA option header = %d/%d, want 2/1", icmp[24], icmp[25])
	}
	if net.HardwareAddr(icmp[26:32]).String() != r.mac.String() {
		t.Errorf("TLLA MAC = %s, want %s", net.HardwareAddr(icmp[26:32]), r.mac)
	}

	// Checksum property: recomputing over the payload (with checksum in place) is 0.
	if got := icmpv6Checksum(ipv6[8:24], ipv6[24:40], icmp); got != 0 {
		t.Errorf("ICMPv6 checksum does not validate, got %#x want 0", got)
	}
}

func TestBuildResponse_IgnoresOutOfPrefix(t *testing.T) {
	r := testResponder(t, "2a02:c207:2011:9989::/64")
	gwMAC := net.HardwareAddr{0x00, 0xdc, 0x00, 0x00, 0x00, 0x02}
	gwIP := net.ParseIP("fe80::2dc:ff:fe00:2")
	other := net.ParseIP("2a02:c207:2011:9988::1") // different /64

	if _, _, _, ok := r.buildResponse(buildNS(gwMAC, gwIP, other)); ok {
		t.Fatal("responder answered NS for an address outside its prefix")
	}
}

func TestBuildResponse_SkipsAnycastZero(t *testing.T) {
	r := testResponder(t, "2a02:c207:2011:9989::/64")
	gwMAC := net.HardwareAddr{0x00, 0xdc, 0x00, 0x00, 0x00, 0x02}
	gwIP := net.ParseIP("fe80::2dc:ff:fe00:2")
	zero := net.ParseIP("2a02:c207:2011:9989::") // subnet-anycast

	if _, _, _, ok := r.buildResponse(buildNS(gwMAC, gwIP, zero)); ok {
		t.Fatal("responder answered NS for the ::0 subnet-anycast address")
	}
}

func TestBuildResponse_IgnoresNonNS(t *testing.T) {
	r := testResponder(t, "2a02:c207:2011:9989::/64")
	gwMAC := net.HardwareAddr{0x00, 0xdc, 0x00, 0x00, 0x00, 0x02}
	gwIP := net.ParseIP("fe80::2dc:ff:fe00:2")
	target := net.ParseIP("2a02:c207:2011:9989::1")
	frame := buildNS(gwMAC, gwIP, target)
	frame[14+40] = ndTypeNA // flip ICMP type to NA — must be ignored

	if _, _, _, ok := r.buildResponse(frame); ok {
		t.Fatal("responder answered a non-NS ICMPv6 message")
	}
}
