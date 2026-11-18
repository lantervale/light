package cookies

import (
	"context"
	"net/http"
	"net/url"
)

type key struct{}

// asked is what a context carries for the Transport.
type asked struct{ top, from *url.URL }

// Within is ctx for the requests the page at top makes: its sheets, its
// pictures, its scripts and what they fetch. A request to another site
// than the page's is a third party's.
func Within(ctx context.Context, top *url.URL) context.Context {
	return context.WithValue(ctx, key{}, asked{top: top})
}

// From is ctx for a navigation the page at from began, a link followed
// or a script's: one to another site carries no Strict cookie. A
// context that is neither Within nor From is a navigation the player
// asked for.
func From(ctx context.Context, from *url.URL) context.Context {
	return context.WithValue(ctx, key{}, asked{from: from})
}

type withoutKey struct{}

// Without is ctx for a request that carries no cookie and whose answer
// sets none: a script's that asks for none, or asks another origin
// without saying it wants them.
func Without(ctx context.Context) context.Context {
	return context.WithValue(ctx, withoutKey{}, true)
}

// Transport makes requests through Base, http.DefaultTransport when
// nil, each sent with the cookies Jar holds for it, and takes those its
// answer sets: every hop of a redirect on its own. With no Jar, and for
// a request Without cookies, it is Base.
type Transport struct {
	Jar  *Jar
	Base http.RoundTripper
}

func (t *Transport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Jar == nil || req.Context().Value(withoutKey{}) != nil {
		return t.base().RoundTrip(req)
	}
	a, _ := req.Context().Value(key{}).(asked)
	q := Request{URL: req.URL, Top: a.top, From: a.from, Unsafe: unsafe(req.Method)}
	if header := t.Jar.Header(q); header != "" {
		req = req.Clone(req.Context())
		req.Header.Set("Cookie", header)
	}
	resp, err := t.base().RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if lines := resp.Header.Values("Set-Cookie"); len(lines) > 0 {
		t.Jar.Store(q, lines)
	}
	return resp, nil
}

// unsafe reports a method that is not one of those that only read.
func unsafe(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	}
	return true
}

// Page is the jar as the scripts of one page see it, through
// document.cookie.
type Page struct {
	jar *Jar
	at  func() *url.URL
}

// Page is the jar for the scripts of the page at the address at gives,
// asked each time, since a page's own moves change it.
func (j *Jar) Page(at func() *url.URL) *Page { return &Page{jar: j, at: at} }

func (p *Page) Get() string { return p.jar.Script(p.at()) }

func (p *Page) Set(cookie string) { p.jar.SetScript(p.at(), cookie) }

// Enabled reports whether the page's scripts may keep cookies.
func (p *Page) Enabled() bool { return p.jar.Enabled() }
