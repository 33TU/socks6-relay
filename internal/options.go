package internal

import "time"

// Options holds the configuration for the SOCKS server.
type Options struct {
	Network string
	Addr    string

	Username string
	Password string

	AllowConnect   bool
	ConnectTimeout time.Duration

	AllowUDPAssociate         bool
	UDPAssociateAdvertiseAddr string
	UDPAssociateTimeout       time.Duration

	IPv6Generator *IPv6Generator
}
