package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/netlog"
)

const (
	bySheet = "stylesheet"
	// fontTimeout is how long one font of a page may take, and maxFont
	// the largest read.
	fontTimeout = 30 * time.Second
	maxFont     = 16 << 20
	// fontFiles are the fonts light reads, as it asks for them.
	fontFiles = "font/woff2, font/woff, font/ttf, font/otf, */*;q=0.1"
)

// fonts fetches the fonts of the page's @font-face rules as ui asks
// for them, when text first needs one, long after the page's load.
// They are recorded where ctx says, as asked by their sheet, and
// outlive it.
func (b *builder) fonts(ctx context.Context) ui.FontFetcher {
	detached := context.WithoutCancel(ctx)
	return func(ref, sheet string) ([]byte, error) {
		address := b.resolve(ref)
		if address == "" {
			return nil, fmt.Errorf("%q is no http or https address", ref)
		}
		if sheet == "" {
			sheet = b.base.String()
		}
		timed, cancel := context.WithTimeout(detached, fontTimeout)
		defer cancel()
		return b.loader.Font(timed, address, sheet)
	}
}

// Font gets the font file at address for the sheet at sheet, which the
// request names as where it comes from.
func (l *Loader) Font(ctx context.Context, address, sheet string) ([]byte, error) {
	from, _ := url.Parse(sheet)
	to, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	ctx = fetch.Referring(netlog.As(ctx, bySheet, netlog.Font), referrer(from, to))
	answer, err := l.Client.Get(ctx, address, fontFiles)
	switch {
	case err != nil:
		return nil, err
	case !answer.OK():
		return nil, fmt.Errorf("the server answered %d %s", answer.Status, http.StatusText(answer.Status))
	case len(answer.Body) > maxFont:
		return nil, errors.New("the font is over " + megabytes(maxFont))
	}
	return answer.Body, nil
}

// referrer is what a request for to says of from, the address it comes
// from, as browsers do by default: the whole address without its
// fragment and its user within a site, the site alone to another, and
// nothing from https to http.
func referrer(from, to *url.URL) string {
	if from == nil || to == nil || (from.Scheme != schemeHTTP && from.Scheme != schemeHTTPS) {
		return ""
	}
	if from.Scheme == schemeHTTPS && to.Scheme != schemeHTTPS {
		return ""
	}
	if from.Scheme != to.Scheme || from.Host != to.Host {
		return from.Scheme + "://" + from.Host + "/"
	}
	said := *from
	said.User, said.Fragment, said.RawFragment = nil, "", ""
	return said.String()
}
