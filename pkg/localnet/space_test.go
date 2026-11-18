package localnet_test

import (
	"net/netip"
	"testing"

	"github.com/lantervale/light/pkg/localnet"
)

func TestTheSpaceOfAnAddress(t *testing.T) {
	for _, c := range []struct {
		address string
		want    localnet.Space
	}{
		{"127.0.0.1", localnet.Loopback}, {"127.255.255.254", localnet.Loopback}, {"::1", localnet.Loopback},
		{"::ffff:127.0.0.1", localnet.Loopback}, {"0.0.0.0", localnet.Loopback}, {"0.1.2.3", localnet.Loopback},
		{"::", localnet.Loopback}, {"::ffff:0.0.0.0", localnet.Loopback}, {"64:ff9b::7f00:1", localnet.Loopback},
		{"2002:7f00:1::1", localnet.Loopback}, {"::127.0.0.1", localnet.Loopback},

		{"10.0.0.1", localnet.Private}, {"10.255.255.255", localnet.Private}, {"172.16.0.1", localnet.Private},
		{"172.31.255.255", localnet.Private}, {"192.168.0.1", localnet.Private}, {"192.168.255.255", localnet.Private},
		{"169.254.169.254", localnet.Private}, {"100.64.0.1", localnet.Private}, {"100.127.255.255", localnet.Private},
		{"fc00::1", localnet.Private}, {"fdff::1", localnet.Private}, {"fe80::1", localnet.Private},
		{"fe80::1%en0", localnet.Private}, {"febf::1", localnet.Private}, {"fec0::1", localnet.Private},
		{"::ffff:10.0.0.1", localnet.Private}, {"::ffff:192.168.1.1", localnet.Private},
		{"64:ff9b::c0a8:101", localnet.Private}, {"2002:c0a8:101::1", localnet.Private},
		{"192.0.0.8", localnet.Private}, {"192.0.2.1", localnet.Private}, {"198.18.0.1", localnet.Private},
		{"198.19.255.255", localnet.Private}, {"198.51.100.1", localnet.Private}, {"203.0.113.1", localnet.Private},
		{"224.0.0.1", localnet.Private}, {"239.255.255.250", localnet.Private}, {"240.0.0.1", localnet.Private},
		{"255.255.255.255", localnet.Private}, {"ff02::1", localnet.Private}, {"2001:db8::1", localnet.Private},
		{"::ffff:169.254.1.1", localnet.Private},

		{"1.1.1.1", localnet.Public}, {"8.8.8.8", localnet.Public}, {"9.255.255.255", localnet.Public},
		{"11.0.0.0", localnet.Public}, {"100.63.255.255", localnet.Public}, {"100.128.0.0", localnet.Public},
		{"126.255.255.255", localnet.Public}, {"128.0.0.1", localnet.Public}, {"169.253.255.255", localnet.Public},
		{"169.255.0.0", localnet.Public}, {"172.15.255.255", localnet.Public}, {"172.32.0.0", localnet.Public},
		{"192.167.255.255", localnet.Public}, {"192.169.0.0", localnet.Public}, {"198.17.255.255", localnet.Public},
		{"198.20.0.0", localnet.Public}, {"223.255.255.255", localnet.Public}, {"2606:4700:4700::1111", localnet.Public},
		{"::ffff:8.8.8.8", localnet.Public}, {"64:ff9b::808:808", localnet.Public}, {"2002:808:808::1", localnet.Public},
		{"fbff::1", localnet.Public}, {"2001:4860:4860::8888", localnet.Public},
	} {
		if got := localnet.Of(netip.MustParseAddr(c.address)); got != c.want {
			t.Errorf("%s is %s, want %s", c.address, got, c.want)
		}
	}
	if got := localnet.Of(netip.Addr{}); got != localnet.Loopback {
		t.Errorf("no address is %s, want the one nothing else reaches", got)
	}
}

func TestWhatASpaceReaches(t *testing.T) {
	spaces := []localnet.Space{localnet.Public, localnet.Private, localnet.Loopback}
	want := map[[2]localnet.Space]bool{
		{localnet.Public, localnet.Public}: true, {localnet.Private, localnet.Public}: true,
		{localnet.Private, localnet.Private}: true, {localnet.Loopback, localnet.Public}: true,
		{localnet.Loopback, localnet.Private}: true, {localnet.Loopback, localnet.Loopback}: true,
	}
	for _, from := range spaces {
		for _, to := range spaces {
			if got := from.Reaches(to); got != want[[2]localnet.Space{from, to}] {
				t.Errorf("%s reaches %s: %v", from, to, got)
			}
		}
	}
	if localnet.Public.String() != "public" || localnet.Private.String() != "local" ||
		localnet.Loopback.String() != "loopback" {
		t.Errorf("the spaces are named %s, %s and %s", localnet.Public, localnet.Private, localnet.Loopback)
	}
}
