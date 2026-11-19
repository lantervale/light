package localnet_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/localnet"
)

// network is servers of this machine taken for servers of the three
// spaces, each by its port, and how many requests each got.
type network struct {
	t      *testing.T
	guard  *localnet.Guard
	client *http.Client
	of     map[uint16]localnet.Space
	got    map[localnet.Space]*atomic.Int32
	at     map[localnet.Space]string
}

func spaces(t *testing.T) *network {
	t.Helper()
	n := &network{t: t, of: map[uint16]localnet.Space{}, got: make(map[localnet.Space]*atomic.Int32),
		at: make(map[localnet.Space]string)}
	for _, space := range []localnet.Space{localnet.Public, localnet.Private, localnet.Loopback} {
		count := &atomic.Int32{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count.Add(1)
			if to := r.URL.Query().Get("to"); to != "" {
				http.Redirect(w, r, to, http.StatusFound)
				return
			}
			_, _ = io.WriteString(w, space.String())
		}))
		t.Cleanup(srv.Close)
		at := netip.MustParseAddrPort(srv.Listener.Addr().String())
		n.of[at.Port()], n.got[space], n.at[space] = space, count, srv.URL
	}
	n.guard = &localnet.Guard{Space: func(at netip.AddrPort) localnet.Space { return n.of[at.Port()] }}
	n.client = &http.Client{Transport: n.guard}
	t.Cleanup(n.guard.CloseIdleConnections)
	return n
}

// get is what a request in ctx for address was answered, and why not.
func (n *network) get(ctx context.Context, address string) (string, error) {
	n.t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, http.NoBody)
	if err != nil {
		n.t.Fatal(err)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

func blocked(t *testing.T, err error, from, to localnet.Space) {
	t.Helper()
	var refused *localnet.BlockedError
	if !errors.As(err, &refused) || refused.From != from || refused.To != to {
		t.Errorf("a page of the %s network asking the %s one: %v", from, to, err)
	}
}

func TestAPageReachesOnlyAddressesAsPublicAsItsOwn(t *testing.T) {
	n := spaces(t)
	all := []localnet.Space{localnet.Public, localnet.Private, localnet.Loopback}
	for _, from := range all {
		for _, to := range all {
			before := n.got[to].Load()
			got, err := n.get(localnet.From(t.Context(), from), n.at[to])
			if from.Reaches(to) {
				if err != nil || got != to.String() {
					t.Errorf("a page of the %s network asking the %s one got %q, %v", from, to, got, err)
				}
				continue
			}
			blocked(t, err, from, to)
			if n.got[to].Load() != before {
				t.Errorf("the %s server heard of a page of the %s network", to, from)
			}
		}
	}
	for _, to := range all {
		if got, err := n.get(t.Context(), n.at[to]); err != nil || got != to.String() {
			t.Errorf("the user asking the %s network got %q, %v", to, got, err)
		}
	}
}

func TestAConnectionOpenedForTheUserIsNotAPages(t *testing.T) {
	n := spaces(t)
	for _, ctx := range []context.Context{t.Context(), localnet.From(t.Context(), localnet.Loopback)} {
		if _, err := n.get(ctx, n.at[localnet.Loopback]); err != nil {
			t.Fatal(err)
		}
	}
	before := n.got[localnet.Loopback].Load()
	_, err := n.get(localnet.From(t.Context(), localnet.Public), n.at[localnet.Loopback])
	blocked(t, err, localnet.Public, localnet.Loopback)
	_, err = n.get(localnet.From(t.Context(), localnet.Private), n.at[localnet.Loopback])
	blocked(t, err, localnet.Private, localnet.Loopback)
	if n.got[localnet.Loopback].Load() != before {
		t.Error("a page's request went through a connection opened for the user")
	}
}

func TestARedirectLeadsAPageNoFurtherThanItReaches(t *testing.T) {
	n := spaces(t)
	public := localnet.From(t.Context(), localnet.Public)
	for _, to := range []localnet.Space{localnet.Private, localnet.Loopback} {
		before := n.got[to].Load()
		_, err := n.get(public, n.at[localnet.Public]+"/?to="+url.QueryEscape(n.at[to]))
		blocked(t, err, localnet.Public, to)
		if n.got[to].Load() != before {
			t.Errorf("a redirect led a public page to the %s server", to)
		}
	}
	if got, err := n.get(public, n.at[localnet.Public]+"/?to="+url.QueryEscape(n.at[localnet.Public])); err != nil ||
		got != "public" {
		t.Errorf("a redirect within the public network got %q, %v", got, err)
	}
	turned := n.at[localnet.Public] + "/?to=" + url.QueryEscape(n.at[localnet.Loopback])
	if got, err := n.get(t.Context(), turned); err != nil || got != "loopback" {
		t.Errorf("the user redirected to this machine got %q, %v", got, err)
	}
}

func TestANameThatResolvesToThisMachineIsRefusedAsItsAddress(t *testing.T) {
	n := spaces(t)
	named := strings.Replace(n.at[localnet.Loopback], "127.0.0.1", "localhost", 1)
	_, err := n.get(localnet.From(t.Context(), localnet.Public), named)
	blocked(t, err, localnet.Public, localnet.Loopback)

	resolved := spaces(t)
	rebound, dialled := "http://rebound.test:80/", ""
	base, _ := http.DefaultTransport.(*http.Transport)
	turned := base.Clone()
	turned.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		dialled = address
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(resolved.at[localnet.Loopback], "http://"))
	}
	resolved.guard.Base = turned
	before := resolved.got[localnet.Loopback].Load()
	_, err = resolved.get(localnet.From(t.Context(), localnet.Public), rebound)
	blocked(t, err, localnet.Public, localnet.Loopback)
	if dialled != "rebound.test:80" || resolved.got[localnet.Loopback].Load() != before {
		t.Errorf("%s was dialled, and the server heard %d requests", dialled, resolved.got[localnet.Loopback].Load()-before)
	}
	if got, failed := resolved.get(t.Context(), rebound); failed != nil || got != "loopback" {
		t.Errorf("the user asking the name got %q, %v", got, failed)
	}
}

func TestTheAddressesOfThisMachineAreItsOwnWithNothingSaidOfThem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "here")
	}))
	t.Cleanup(srv.Close)
	n := &network{t: t, guard: &localnet.Guard{}}
	n.client = &http.Client{Transport: n.guard}
	t.Cleanup(n.guard.CloseIdleConnections)
	for _, from := range []localnet.Space{localnet.Public, localnet.Private} {
		_, err := n.get(localnet.From(t.Context(), from), srv.URL)
		blocked(t, err, from, localnet.Loopback)
		for _, address := range []string{"http://0.0.0.0:1/", "http://[::1]:1/", "http://[::ffff:127.0.0.1]:1/",
			"http://app.localhost:1/", "http://127.1:1/"} {
			if _, err = n.get(localnet.From(t.Context(), from), address); err == nil {
				t.Errorf("a page of the %s network reached %s", from, address)
			}
		}
	}
	_, err := n.get(localnet.From(t.Context(), localnet.Public), "http://10.255.255.1:1/")
	blocked(t, err, localnet.Public, localnet.Private)
	if got, failed := n.get(localnet.From(t.Context(), localnet.Loopback), srv.URL); failed != nil || got != "here" {
		t.Errorf("a page of this machine asking it got %q, %v", got, failed)
	}
	n.guard.Open(true)
	if got, failed := n.get(localnet.From(t.Context(), localnet.Public), srv.URL); failed != nil || got != "here" {
		t.Errorf("with the guard open a public page got %q, %v", got, failed)
	}
	n.guard.Open(false)
	_, err = n.get(localnet.From(t.Context(), localnet.Public), srv.URL)
	blocked(t, err, localnet.Public, localnet.Loopback)
	var refused *localnet.BlockedError
	if errors.As(err, &refused) && (refused.Blocked() != "The request client is in the `public` address space and the "+
		"resource is in more-private address space `loopback`." || !strings.Contains(refused.Error(), refused.Address) ||
		!strings.HasPrefix(refused.Address, "127.0.0.1:")) {
		t.Errorf("the refusal says %q and %q", refused.Blocked(), refused.Error())
	}
}

func TestTheSpaceAnAnswerCameFromIsKept(t *testing.T) {
	n := spaces(t)
	for _, c := range []struct {
		address string
		want    localnet.Space
	}{
		{n.at[localnet.Loopback], localnet.Loopback}, {n.at[localnet.Private], localnet.Private},
		{n.at[localnet.Public], localnet.Public},
		{n.at[localnet.Public] + "/?to=" + url.QueryEscape(n.at[localnet.Private]), localnet.Private},
		{n.at[localnet.Loopback] + "/?to=" + url.QueryEscape(n.at[localnet.Public]), localnet.Public},
	} {
		ctx, served := localnet.Watch(t.Context())
		if _, err := n.get(ctx, c.address); err != nil || served.Space() != c.want {
			t.Errorf("%s was answered from the %s network, want %s (%v)", c.address, served.Space(), c.want, err)
		}
	}
	ctx, served := localnet.Watch(t.Context())
	if _, err := n.get(ctx, "http://127.0.0.1:1/"); err == nil || served.Space() != localnet.Public {
		t.Errorf("a request nothing answered came from the %s network (%v)", served.Space(), err)
	}
}

func TestThroughAProxyOnlyTheAddressNamedIsChecked(t *testing.T) {
	var asked []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.String())
		_, _ = io.WriteString(w, "proxied")
	}))
	t.Cleanup(proxy.Close)
	through, _ := url.Parse(proxy.URL)
	base, _ := http.DefaultTransport.(*http.Transport)
	proxied := base.Clone()
	proxied.Proxy = http.ProxyURL(through)
	n := &network{t: t, guard: &localnet.Guard{Base: proxied}}
	n.client = &http.Client{Transport: n.guard}
	t.Cleanup(proxied.CloseIdleConnections)
	public := localnet.From(t.Context(), localnet.Public)
	ctx, served := localnet.Watch(public)
	if got, err := n.get(ctx, "http://site.test/page"); err != nil || got != "proxied" ||
		served.Space() != localnet.Public {
		t.Errorf("through the proxy a public page got %q from the %s network, %v", got, served.Space(), err)
	}
	for address, to := range map[string]localnet.Space{
		"http://127.0.0.1:8080/": localnet.Loopback, "http://localhost/": localnet.Loopback,
		"http://[::1]/": localnet.Loopback, "http://192.168.1.1/": localnet.Private,
	} {
		_, err := n.get(public, address)
		blocked(t, err, localnet.Public, to)
	}
	if len(asked) != 1 || asked[0] != "http://site.test/page" {
		t.Errorf("the proxy was asked for %q", asked)
	}
}

// opaque is a transport that shows nothing of how it dials.
type opaque struct{ base http.RoundTripper }

func (o opaque) RoundTrip(req *http.Request) (*http.Response, error) { return o.base.RoundTrip(req) }

func TestATransportThatDialsByItselfGivesNoAnswerAPageMayNotHave(t *testing.T) {
	n := spaces(t)
	base, _ := http.DefaultTransport.(*http.Transport)
	own := base.Clone()
	t.Cleanup(own.CloseIdleConnections)
	n.guard.Base = opaque{own}
	public := localnet.From(t.Context(), localnet.Public)
	_, err := n.get(public, n.at[localnet.Loopback])
	blocked(t, err, localnet.Public, localnet.Loopback)
	named := strings.Replace(n.at[localnet.Private], "127.0.0.1", "localhost", 1)
	if _, err = n.get(public, named); err == nil {
		t.Error("a public page read the local network through a transport that dials by itself")
	}
	if got, failed := n.get(public, n.at[localnet.Public]); failed != nil || got != "public" {
		t.Errorf("a public page asking the public network got %q, %v", got, failed)
	}
	if got, failed := n.get(t.Context(), n.at[localnet.Loopback]); failed != nil || got != "loopback" {
		t.Errorf("the user got %q, %v", got, failed)
	}
	own.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(n.at[localnet.Loopback], "http://"))
	}
	got, err := n.get(public, "http://rebound.test/")
	blocked(t, err, localnet.Public, localnet.Loopback)
	if got != "" {
		t.Errorf("a public page read %q of this machine", got)
	}
	if got, err = n.get(localnet.From(t.Context(), localnet.Loopback), "http://rebound.test/"); err != nil ||
		got != "loopback" {
		t.Errorf("a page of this machine got %q, %v", got, err)
	}
}

// counted is a server of this machine and how many connections were
// opened to it.
func counted(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	opened := &atomic.Int32{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, _ := io.ReadAll(r.Body)
		_, _ = w.Write(sent)
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, opened
}

func TestAPagesRequestsGoThroughTheConnectionItsDocumentCameThrough(t *testing.T) {
	for _, space := range []localnet.Space{localnet.Public, localnet.Private, localnet.Loopback} {
		srv, opened := counted(t)
		guard := &localnet.Guard{Space: func(netip.AddrPort) localnet.Space { return space }}
		t.Cleanup(guard.CloseIdleConnections)
		client := &http.Client{Transport: guard}
		for _, ctx := range []context.Context{t.Context(), localnet.From(t.Context(), space), t.Context()} {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, strings.NewReader("sent"))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if string(body) != "sent" {
				t.Errorf("a request to the %s server carried %q", space, body)
			}
		}
		if got := opened.Load(); got != 1 {
			t.Errorf("the user's request and a page's to one %s server opened %d connections", space, got)
		}
	}
}

// held is a server that answers a request only once told to, how many
// wait for it, and how many connections were opened to it.
type held struct {
	*httptest.Server
	waiting, opened atomic.Int32
	free            chan struct{}
}

func holding(t *testing.T, secure bool) *held {
	t.Helper()
	h := &held{free: make(chan struct{})}
	h.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.waiting.Add(1)
		defer h.waiting.Add(-1)
		select {
		case <-h.free:
		case <-r.Context().Done():
		}
		_, _ = io.WriteString(w, r.Proto)
	}))
	h.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			h.opened.Add(1)
		}
	}
	if secure {
		h.EnableHTTP2 = true
		h.StartTLS()
	} else {
		h.Start()
	}
	t.Cleanup(h.Close)
	return h
}

// ask makes count requests of a public page at once, and gives what
// each was answered once the server, told to when wait of them wait,
// answered them all.
func (h *held) ask(t *testing.T, client *http.Client, count, wait int) []string {
	t.Helper()
	answers := make(chan string, count)
	for range count {
		go func() {
			req, _ := http.NewRequestWithContext(localnet.From(t.Context(), localnet.Public), http.MethodGet, h.URL,
				http.NoBody)
			resp, err := client.Do(req)
			if err != nil {
				answers <- err.Error()
				return
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			answers <- string(body)
		}()
	}
	for deadline := time.Now().Add(10 * time.Second); int(h.waiting.Load()) < wait; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d requests reached the server at once", h.waiting.Load(), count)
		}
	}
	close(h.free)
	got := make([]string, count)
	for i := range got {
		got[i] = <-answers
	}
	h.free = make(chan struct{})
	return got
}

func TestThePagesOfASpaceOpenSixConnectionsToAHostAndKeepThem(t *testing.T) {
	h := holding(t, false)
	guard := &localnet.Guard{Space: func(netip.AddrPort) localnet.Space { return localnet.Public }}
	t.Cleanup(guard.CloseIdleConnections)
	client := &http.Client{Transport: guard}
	for _, answer := range h.ask(t, client, 14, 6) {
		if answer != "HTTP/1.1" {
			t.Fatalf("a request was answered %q", answer)
		}
	}
	if got := h.opened.Load(); got != 6 {
		t.Errorf("14 requests at once to one host opened %d connections", got)
	}
	h.ask(t, client, 6, 6)
	if got := h.opened.Load(); got != 6 {
		t.Errorf("6 more requests opened %d more connections: those opened were not kept", got-6)
	}
}

func TestHTTP2IsSpokenWhereTheServerOffersItAndItsOneConnectionShared(t *testing.T) {
	h := holding(t, true)
	guard := &localnet.Guard{Base: h.Client().Transport, Space: func(netip.AddrPort) localnet.Space {
		return localnet.Public
	}}
	t.Cleanup(guard.CloseIdleConnections)
	client := &http.Client{Transport: guard}
	close(h.free)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, h.URL, http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	h.free = make(chan struct{})
	for _, answer := range h.ask(t, client, 14, 14) {
		if answer != "HTTP/2.0" {
			t.Fatalf("a request was answered %q", answer)
		}
	}
	if got := h.opened.Load(); resp.Proto != "HTTP/2.0" || got != 1 {
		t.Errorf("the user's request, answered by %s, and 14 of a page at once opened %d connections", resp.Proto, got)
	}
}
