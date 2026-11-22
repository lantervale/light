package web

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"time"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/identity"
	"github.com/lantervale/light/pkg/netlog"
)

// importTimeout is how long one sheet an @import names may take.
const importTimeout = 30 * time.Second

// imports fetches the sheets the page's @import rules name as ui asks
// for them, after the page's load. They are recorded where ctx says, as
// asked by the sheet importing them, and outlive it.
func (b *builder) imports(ctx context.Context) ui.SheetFetcher {
	detached := context.WithoutCancel(ctx)
	return func(ref, sheet string) ([]byte, error) {
		address := b.resolve(ref)
		if address == "" {
			return nil, fmt.Errorf("%q is no http or https address", ref)
		}
		if sheet == "" {
			sheet = b.base.String()
		}
		timed, cancel := context.WithTimeout(detached, importTimeout)
		defer cancel()
		return b.loader.Imported(timed, address, sheet)
	}
}

// Imported gets the sheet at address that the sheet at sheet imports,
// which the request names as where it comes from: one served as
// text/css or as no type, as Chrome takes it, its text up to what is
// read of a sheet.
func (l *Loader) Imported(ctx context.Context, address, sheet string) ([]byte, error) {
	from, _ := url.Parse(sheet)
	ctx = fetch.Asking(netlog.As(ctx, bySheet, netlog.Stylesheet),
		identity.Request{Destination: identity.Style, Referrer: from})
	answer, err := l.Client.Get(ctx, address, identity.Accept(identity.Style))
	if err != nil {
		return nil, err
	}
	if !answer.OK() {
		return nil, fmt.Errorf("the server answered %d %s", answer.Status, http.StatusText(answer.Status))
	}
	if kind := answer.Header.Get("Content-Type"); kind != "" {
		if media, _, _ := mime.ParseMediaType(kind); media != "text/css" {
			return nil, fmt.Errorf("it is served as %s, not text/css", media)
		}
	}
	return []byte(cut(answer.Code())), nil
}
