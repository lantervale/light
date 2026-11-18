// Package testnet is the network of a run driven by a test runner, whose
// servers are all of this machine under names of their own: names
// resolved to this machine where light dials, so the system's hosts
// file is left alone, and a certificate authority trusted by that run's
// transport only, every certificate still checked.
package testnet

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"
)

// Hosts are names with the address of this machine each one, and every
// name under it, is at.
type Hosts map[string]netip.Addr

// ParseHosts reads names and their addresses, which are of this machine
// (127.0.0.1, ::1): a name is mapped nowhere else.
func ParseHosts(list map[string]string) (Hosts, error) {
	hosts := make(Hosts, len(list))
	for name, address := range list {
		key := canonical(name)
		if _, numbers := netip.ParseAddr(key); key == "" || numbers == nil || strings.ContainsAny(key, " */:") {
			return nil, fmt.Errorf("%q is not a host's name", name)
		}
		at, err := netip.ParseAddr(address)
		if err != nil {
			return nil, fmt.Errorf("%s is mapped to %q, which is not an address", name, address)
		}
		if !at.IsLoopback() {
			return nil, fmt.Errorf("%s is mapped to %s, which is not this machine (127.0.0.1, ::1)", name, at)
		}
		hosts[key] = at
	}
	return hosts, nil
}

func canonical(name string) string {
	return strings.ToLower(strings.TrimSuffix(name, "."))
}

// Lookup is the address name is mapped to: its own, else the one of the
// nearest name above it.
func (h Hosts) Lookup(name string) (netip.Addr, bool) {
	for name = canonical(name); name != ""; {
		if at, mapped := h[name]; mapped {
			return at, true
		}
		_, above, under := strings.Cut(name, ".")
		if !under {
			break
		}
		name = above
	}
	return netip.Addr{}, false
}

// Transport is http.DefaultTransport's settings with the names of hosts
// dialled where they are mapped, past any proxy, and with the
// authorities of caFile, a PEM file, trusted beside the system's when
// one is given.
func Transport(hosts Hosts, caFile string) (*http.Transport, error) {
	base, cloned := http.DefaultTransport.(*http.Transport)
	if !cloned {
		return nil, errors.New("the default transport is not one to copy")
	}
	t := base.Clone()
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	t.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if host, port, err := net.SplitHostPort(address); err == nil {
			if at, mapped := hosts.Lookup(host); mapped {
				address = net.JoinHostPort(at.String(), port)
			}
		}
		return dialer.DialContext(ctx, network, address)
	}
	proxy := t.Proxy
	t.Proxy = func(req *http.Request) (*url.URL, error) {
		if _, mapped := hosts.Lookup(req.URL.Hostname()); mapped || proxy == nil {
			return nil, nil //nolint:nilnil // no proxy and no error is what http.Transport.Proxy answers
		}
		return proxy(req)
	}
	if caFile == "" {
		return t, nil
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	if !roots.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("%s holds no certificate (PEM)", caFile)
	}
	t.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	return t, nil
}
