package fetch

import (
	"context"
	"net/http"
	"net/url"
	"sync"

	"github.com/lantervale/light/pkg/identity"
)

// Languages is the Accept-Language requests are sent with unless they
// say their own.
func Languages(header string) Option { return func(c *Client) { c.languages = header } }

type (
	pageKey      struct{}
	describedKey struct{}
)

// described is one request as its context describes it, and the
// addresses it went through so far.
type described struct {
	q identity.Request
	sync.Mutex
	trail []*url.URL
}

// By is ctx for the requests the page at address makes, each then
// described on its own (Asking): none is taken for the navigation that
// led to the page.
func By(ctx context.Context, page *url.URL) context.Context {
	return context.WithValue(context.WithValue(Referring(ctx, ""), pageKey{}, page), describedKey{}, (*described)(nil))
}

// Asking is ctx for one request, described by q: it then carries the
// headers a browser gives such a request, at every hop of its
// redirects (identity.Apply). A request that names no initiator is
// asked by the page ctx is By, if any.
func Asking(ctx context.Context, q identity.Request) context.Context {
	if q.Initiator == nil {
		q.Initiator, _ = ctx.Value(pageKey{}).(*url.URL)
	}
	return context.WithValue(ctx, describedKey{}, &described{q: q})
}

// present gives req the headers light sends by default, for the address
// it now goes to: called once for each hop. mode is how a script asked
// for it, "" for what its context says.
func (c *Client) present(req *http.Request, mode Mode) {
	who := identity.Identity{Agent: c.agent, Languages: c.languages}
	d, _ := req.Context().Value(describedKey{}).(*described)
	if d == nil {
		who.Apply(req.Header, nil, nil)
		return
	}
	d.Lock()
	defer d.Unlock()
	d.trail = append(d.trail, req.URL)
	q := d.q
	if mode != "" {
		q.Mode = identity.Mode(mode)
	}
	if said := req.Header.Get("Referer"); len(d.trail) == 1 && q.Referrer == nil && said != "" &&
		said != identity.Referrer(q.Initiator, req.URL) {
		if from, err := url.Parse(said); err == nil {
			d.q.Referrer, q.Referrer = from, from
		}
	}
	who.Apply(req.Header, &q, d.trail)
}
