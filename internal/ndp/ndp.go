package ndp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"socks-ipv6-relay/internal/host"

	"golang.org/x/sys/unix"
)

// NDPResponder answers IPv6 Neighbor Solicitations for every address in a prefix
// with a Neighbor Advertisement that has the Override flag set.
//
// This is needed on "on-link" providers (e.g. Contabo) whose gateway sits inside
// your prefix and resolves each destination address via NDP. The kernel's
// proxy_ndp and the ndppd daemon both emit NAs with Override CLEARED, which some
// gateways silently ignore (they keep re-soliciting forever), so return traffic
// for a generated address is dropped. Advertising with Override set — exactly as a
// natively-assigned address does — makes the gateway accept the mapping.
//
// It complements the host setup the preflight checks require: the local route
// lets the kernel accept inbound packets for the prefix, and this makes the
// upstream gateway actually deliver them.
type NDPResponder struct {
	iface string
	ipnet *net.IPNet
	mac   net.HardwareAddr
	ifIdx int
	fd    int
}

const (
	ethTypeIPv6  = 0x86DD
	icmpv6Proto  = 58
	ndTypeNS     = 135
	ndTypeNA     = 136
	naFlagsRSO   = 0xE0000000 // Router + Solicited + Override
	rcvTimeoutMs = 1000
)

// NewNDPResponder resolves the interface and prepares (but does not start) the responder.
func NewNDPResponder(prefix, iface string) (*NDPResponder, error) {
	_, ipnet, err := host.ParseIPv6Prefix(prefix)
	if err != nil {
		return nil, err
	}

	link, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, err
	}
	if len(link.HardwareAddr) != 6 {
		return nil, fmt.Errorf("interface %s has no usable MAC address", iface)
	}

	return &NDPResponder{
		iface: iface,
		ipnet: ipnet,
		mac:   link.HardwareAddr,
		ifIdx: link.Index,
		fd:    -1,
	}, nil
}

// Start runs the responder until ctx is cancelled. It blocks, so run it in a
// goroutine. It requests allmulticast on its own socket so solicitations for
// unowned addresses (sent to solicited-node multicast groups) reach us; the
// kernel drops that request when the socket closes, so nothing is left behind.
func (r *NDPResponder) Start(ctx context.Context) error {
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(ethTypeIPv6)))
	if err != nil {
		return fmt.Errorf("open AF_PACKET socket: %w", err)
	}
	r.fd = fd
	var closeOnce sync.Once
	closeFD := func() { closeOnce.Do(func() { unix.Close(fd) }) }
	defer closeFD()

	if err := unix.Bind(fd, &unix.SockaddrLinklayer{
		Protocol: htons(ethTypeIPv6),
		Ifindex:  r.ifIdx,
	}); err != nil {
		return fmt.Errorf("bind AF_PACKET to %s: %w", r.iface, err)
	}

	// Solicitations for addresses we do not own are sent to their solicited-node
	// multicast groups, which the NIC filters out because the kernel never joined
	// them. Ask for allmulticast on this socket rather than setting the interface
	// flag: the kernel reference counts it against the socket and drops it when
	// the socket closes, so it cannot outlive the process even on SIGKILL.
	if err := unix.SetsockoptPacketMreq(fd, unix.SOL_PACKET, unix.PACKET_ADD_MEMBERSHIP,
		&unix.PacketMreq{
			Ifindex: int32(r.ifIdx),
			Type:    unix.PACKET_MR_ALLMULTI,
		}); err != nil {
		return fmt.Errorf("enable allmulticast on %s: %w", r.iface, err)
	}
	slog.Debug("enabled allmulticast for socket", "iface", r.iface)

	// Periodic read timeout so the loop can observe ctx cancellation. Usec must be
	// in [0, 1e6); split milliseconds into whole seconds + remainder microseconds.
	tv := unix.Timeval{Sec: rcvTimeoutMs / 1000, Usec: (rcvTimeoutMs % 1000) * 1000}
	if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv); err != nil {
		slog.Warn("failed to set recv timeout on NDP socket", "error", err)
	}

	// Close the fd when ctx is cancelled to unblock any in-flight read.
	go func() {
		<-ctx.Done()
		closeFD()
	}()

	slog.Info("NDP responder started", "iface", r.iface, "prefix", r.ipnet.String(), "mac", r.mac.String())

	buf := make([]byte, 2048)
	for {
		if ctx.Err() != nil {
			return nil
		}
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
				continue
			}
			if ctx.Err() != nil || errors.Is(err, unix.EBADF) {
				return nil // socket closed on shutdown
			}
			return fmt.Errorf("recv on NDP socket: %w", err)
		}
		r.handle(buf[:n])
	}
}

// handle parses one frame and, if it is a Neighbor Solicitation for an address in
// our prefix, sends a matching Neighbor Advertisement.
func (r *NDPResponder) handle(frame []byte) {
	na, reqMAC, target, ok := r.buildResponse(frame)
	if !ok {
		return
	}
	if err := unix.Sendto(r.fd, na, 0, &unix.SockaddrLinklayer{
		Protocol: htons(ethTypeIPv6),
		Ifindex:  r.ifIdx,
		Halen:    6,
		Addr:     [8]byte{reqMAC[0], reqMAC[1], reqMAC[2], reqMAC[3], reqMAC[4], reqMAC[5]},
	}); err != nil {
		slog.Debug("failed to send NA", "target", target.String(), "error", err)
		return
	}
	slog.Debug("answered NS", "target", target.String(), "to", reqMAC.String())
}

// buildResponse is the pure parse-and-build core of handle: given a raw Ethernet
// frame it returns the NA frame to send, the requester's MAC, and the solicited
// target — or ok=false if the frame is not a Neighbor Solicitation we should answer.
func (r *NDPResponder) buildResponse(frame []byte) (na []byte, reqMAC net.HardwareAddr, target net.IP, ok bool) {
	// Ethernet(14) + IPv6(40) + ICMPv6 NS(>=24)
	if len(frame) < 14+40+24 {
		return nil, nil, nil, false
	}
	if binary.BigEndian.Uint16(frame[12:14]) != ethTypeIPv6 {
		return nil, nil, nil, false
	}
	ipv6 := frame[14:]
	if ipv6[6] != icmpv6Proto { // Next Header
		return nil, nil, nil, false
	}
	icmp := ipv6[40:]
	if icmp[0] != ndTypeNS {
		return nil, nil, nil, false
	}

	target = net.IP(append([]byte(nil), icmp[8:24]...)) // NS target address
	if !r.ipnet.Contains(target) {
		return nil, nil, nil, false
	}
	if target.Equal(r.ipnet.IP) { // skip the ::0 (subnet anycast) address
		return nil, nil, nil, false
	}

	nsSrc := net.IP(ipv6[8:24])                                    // solicitor's source address (may be ::)
	reqMAC = net.HardwareAddr(append([]byte(nil), frame[6:12]...)) // solicitor's link-layer address

	naDst := nsSrc
	if naDst.Equal(net.IPv6unspecified) {
		naDst = net.IP{0xff, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1} // ff02::1
	}

	return r.buildNA(target, naDst, reqMAC), reqMAC, target, true
}

// buildNA constructs a full Ethernet + IPv6 + ICMPv6 Neighbor Advertisement frame
// advertising `target` as owned by our MAC, with the Override flag set.
func (r *NDPResponder) buildNA(target, dst net.IP, reqMAC net.HardwareAddr) []byte {
	src := target.To16()
	dst = dst.To16()

	// ICMPv6 NA body: type, code, checksum(0), flags, target(16), TLLA option(8)
	icmp := make([]byte, 0, 32)
	icmp = append(icmp, ndTypeNA, 0, 0, 0) // type, code, checksum placeholder
	var flags [4]byte
	binary.BigEndian.PutUint32(flags[:], naFlagsRSO)
	icmp = append(icmp, flags[:]...)
	icmp = append(icmp, src...)   // target address (== advertised address)
	icmp = append(icmp, 2, 1)     // option: Target Link-Layer Address, len 1 (8 bytes)
	icmp = append(icmp, r.mac...) // our MAC

	csum := icmpv6Checksum(src, dst, icmp)
	binary.BigEndian.PutUint16(icmp[2:4], csum)

	// IPv6 header
	ip := make([]byte, 40)
	binary.BigEndian.PutUint32(ip[0:4], 6<<28) // version 6, TC/flow 0
	binary.BigEndian.PutUint16(ip[4:6], uint16(len(icmp)))
	ip[6] = icmpv6Proto
	ip[7] = 255 // hop limit
	copy(ip[8:24], src)
	copy(ip[24:40], dst)

	// Ethernet header
	eth := make([]byte, 0, 14+40+len(icmp))
	eth = append(eth, reqMAC...) // dst MAC
	eth = append(eth, r.mac...)  // src MAC
	eth = append(eth, 0x86, 0xdd)
	eth = append(eth, ip...)
	eth = append(eth, icmp...)
	return eth
}

// icmpv6Checksum computes the ICMPv6 checksum over the IPv6 pseudo-header + payload.
func icmpv6Checksum(src, dst, payload []byte) uint16 {
	var sum uint32
	addBytes := func(b []byte) {
		for i := 0; i+1 < len(b); i += 2 {
			sum += uint32(b[i])<<8 | uint32(b[i+1])
		}
		if len(b)%2 == 1 {
			sum += uint32(b[len(b)-1]) << 8
		}
	}
	addBytes(src)
	addBytes(dst)
	var meta [8]byte
	binary.BigEndian.PutUint32(meta[0:4], uint32(len(payload)))
	meta[7] = icmpv6Proto
	addBytes(meta[:])
	addBytes(payload)
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func htons(v uint16) uint16 {
	return v<<8 | v>>8
}
