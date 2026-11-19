// Package localnet keeps the pages of the web from this machine and
// from the network around it: an address is of a Space, and a request
// a page makes reaches only addresses as public as the one the page
// itself came from (the Private Network Access rule of browsers). A
// Guard holds requests to it where they connect, on the address a name
// resolved to.
package localnet

import (
	"net/netip"
)

// Space is how far an address is from the Internet, the further the
// more private.
type Space int

const (
	// Public is the Internet, and what nothing is known of.
	Public Space = iota
	// Private is the network around this machine, and every address the
	// Internet does not route.
	Private
	// Loopback is this machine.
	Loopback
)

func (s Space) String() string {
	switch s {
	case Private:
		return "local"
	case Loopback:
		return "loopback"
	case Public:
	}
	return "public"
}

// Reaches reports whether a page that came from s may ask an address of
// to: one as public as its own, or more.
func (s Space) Reaches(to Space) bool { return to <= s }

// unrouted are the addresses the Internet does not route, but this
// machine's.
var unrouted = prefixes( //nolint:gochecknoglobals // a constant table
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "100.64.0.0/10",
	"192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
	"fc00::/7", "fe80::/10", "fec0::/10", "ff00::/8", "2001:db8::/32",
)

// embedding are the IPv6 addresses that stand for an IPv4 one, each
// with where its four bytes are.
var embedding = []struct { //nolint:gochecknoglobals // a constant table
	prefix netip.Prefix
	at     int
}{{netip.MustParsePrefix("64:ff9b::/96"), 12}, {netip.MustParsePrefix("2002::/16"), 2},
	{netip.MustParsePrefix("::/96"), 12}}

func prefixes(list ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(list))
	for i, p := range list {
		out[i] = netip.MustParsePrefix(p)
	}
	return out
}

// Of is the space of an address. One that is none, and the addresses
// that name no machine (0.0.0.0/8, ::), are this machine's: nothing but
// it reaches them.
func Of(address netip.Addr) Space {
	address = address.Unmap().WithZone("")
	if !address.IsValid() {
		return Loopback
	}
	if address.Is6() && !address.IsUnspecified() && !address.IsLoopback() {
		raw := address.As16()
		for _, e := range embedding {
			if e.prefix.Contains(address) {
				return Of(netip.AddrFrom4([4]byte(raw[e.at : e.at+4])))
			}
		}
	}
	if address.IsLoopback() || address.IsUnspecified() || (address.Is4() && address.As4()[0] == 0) {
		return Loopback
	}
	for _, p := range unrouted {
		if p.Contains(address) {
			return Private
		}
	}
	return Public
}
