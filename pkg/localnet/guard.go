package localnet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type (
	fromKey   struct{}
	servedKey struct{}
)

// From is ctx for the requests a page that came from space makes: its
// pictures, its sheets, its scripts' requests, the navigations it
// begins unasked. A context that is not From is the user's own: an
// address typed, a link clicked, a page of light's.
func From(ctx context.Context, space Space) context.Context {
	return context.WithValue(ctx, fromKey{}, space)
}

// Served is where the answer of a request came from.
type Served struct {
	space atomic.Int64
}

// Space is the space of the address that answered last, Public while
// none did or when a proxy stood between.
func (s *Served) Space() Space { return Space(s.space.Load()) }

// Watch is ctx for a request whose answer's space is kept, each hop of
// a redirect in its turn.
func Watch(ctx context.Context) (context.Context, *Served) {
	served := &Served{}
	return context.WithValue(ctx, servedKey{}, served), served
}

// BlockedError is a request a page may not make: to an address more
// private than the page's own.
type BlockedError struct {
	From, To Space
	// Address is the address the request was for, with its port.
	Address string
}

func (e *BlockedError) Error() string {
	return fmt.Sprintf("a page of the %s network may not reach the %s network (%s)", e.From, e.To, e.Address)
}

// Blocked is why the request was not sent, as a browser says it.
func (e *BlockedError) Blocked() string {
	return fmt.Sprintf("The request client is in the `%s` address space and the resource is in more-private "+
		"address space `%s`.", e.From, e.To)
}

// Guard is a transport that makes the requests of pages (From) through
// connections of their own, which open only to addresses the page may
// reach: each address a name resolves to is checked as it is dialled,
// so a public name that turns to this machine is refused too, at every
// hop of a redirect. The other requests go through Base as they are.
//
// Through a proxy the address that answers is the proxy's, and nothing
// can be checked but the address the request names: one that is written
// as a private address, or as localhost, is refused.
type Guard struct {
	// Base makes the requests, http.DefaultTransport when nil. One that
	// is no *http.Transport dials where the Guard cannot see: its
	// requests are checked by the address they name, and by the one
	// that answered before anything is read of it.
	Base http.RoundTripper
	// Space is the space of an address, Of when nil: for tests, whose
	// servers are all of this machine.
	Space func(netip.AddrPort) Space

	open atomic.Bool
	mu   sync.Mutex
	// held are the transports of the pages of a space, by that space.
	held map[Space]*http.Transport
}

// Open lets pages reach every address when on: the protection is off.
func (g *Guard) Open(on bool) { g.open.Store(on) }

func (g *Guard) base() http.RoundTripper {
	if g.Base != nil {
		return g.Base
	}
	return http.DefaultTransport
}

func (g *Guard) space(at netip.AddrPort) Space {
	if g.Space != nil {
		return g.Space(netip.AddrPortFrom(at.Addr().Unmap(), at.Port()))
	}
	return Of(at.Addr())
}

// check is why a page from a space may not connect to address, nil when
// it may.
func (g *Guard) check(from Space, address string) error {
	at, err := netip.ParseAddrPort(address)
	to := Loopback
	if err == nil {
		to = g.space(at)
	}
	if from.Reaches(to) {
		return nil
	}
	return &BlockedError{From: from, To: to, Address: address}
}

func (g *Guard) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if served, watched := ctx.Value(servedKey{}).(*Served); watched {
		served.space.Store(int64(Public))
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
			if at, err := netip.ParseAddrPort(info.Conn.RemoteAddr().String()); err == nil && !g.proxied(req) {
				served.space.Store(int64(g.space(at)))
			}
		}})
		req = req.WithContext(ctx)
	}
	from, asked := ctx.Value(fromKey{}).(Space)
	if !asked || from == Loopback || g.open.Load() {
		return g.base().RoundTrip(req)
	}
	if err := g.named(from, req); err != nil {
		return nil, err
	}
	base, dials := g.base().(*http.Transport)
	switch {
	case !dials:
		return g.after(from, req)
	case g.proxied(req):
		return base.RoundTrip(req)
	}
	return g.of(from, base).RoundTrip(req)
}

// proxied reports whether req goes through a proxy.
func (g *Guard) proxied(req *http.Request) bool {
	base, dials := g.base().(*http.Transport)
	if !dials || base.Proxy == nil {
		return false
	}
	proxy, err := base.Proxy(req)
	return err == nil && proxy != nil
}

// named is why a page from a space may not ask for the address req
// names, when it names one in numbers or this machine by its name.
func (g *Guard) named(from Space, req *http.Request) error {
	host, port := req.URL.Hostname(), req.URL.Port()
	if port == "" {
		port = map[string]string{"https": "443"}[req.URL.Scheme]
	}
	if port == "" {
		port = "80"
	}
	if host == "localhost" || len(host) > len(".localhost") && host[len(host)-len(".localhost"):] == ".localhost" {
		host = "127.0.0.1"
	}
	if at, _ := netip.ParseAddr(host); !at.IsValid() {
		return nil
	}
	return g.check(from, net.JoinHostPort(host, port))
}

// after makes req through a base that dials by itself, and gives no
// answer that came from where the page may not reach.
func (g *Guard) after(from Space, req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancelCause(req.Context())
	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		if err := g.check(from, info.Conn.RemoteAddr().String()); err != nil {
			cancel(err)
		}
	}}
	resp, err := g.base().RoundTrip(req.WithContext(httptrace.WithClientTrace(ctx, trace)))
	var blocked *BlockedError
	if errors.As(context.Cause(ctx), &blocked) {
		if err == nil {
			_ = resp.Body.Close()
		}
		return nil, blocked
	}
	if err != nil {
		cancel(nil)
		return nil, err
	}
	resp.Body = &ended{ReadCloser: resp.Body, end: func() { cancel(nil) }}
	return resp, nil
}

// ended is a body that tells when it is closed.
type ended struct {
	io.ReadCloser
	once sync.Once
	end  func()
}

func (e *ended) Close() error {
	err := e.ReadCloser.Close()
	e.once.Do(e.end)
	return err
}

// of is the transport of the pages of a space: base's own settings,
// with connections no other request shares.
func (g *Guard) of(from Space, base *http.Transport) *http.Transport {
	g.mu.Lock()
	defer g.mu.Unlock()
	if t := g.held[from]; t != nil {
		return t
	}
	t := base.Clone()
	dial := base.DialContext
	if dial == nil || g.Base == nil {
		dial = (&net.Dialer{
			Timeout: 30 * time.Second, KeepAlive: 30 * time.Second,
			ControlContext: func(_ context.Context, _, address string, _ syscall.RawConn) error {
				return g.check(from, address)
			},
		}).DialContext
	}
	t.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		if err = g.check(from, conn.RemoteAddr().String()); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return conn, nil
	}
	if g.held == nil {
		g.held = make(map[Space]*http.Transport)
	}
	g.held[from] = t
	return t
}

// CloseIdleConnections closes the connections nothing uses.
func (g *Guard) CloseIdleConnections() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, t := range g.held {
		t.CloseIdleConnections()
	}
	if closer, closes := g.base().(interface{ CloseIdleConnections() }); closes {
		closer.CloseIdleConnections()
	}
}
