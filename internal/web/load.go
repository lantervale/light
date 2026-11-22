package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // the pictures of the web, beside ui's PNG
	_ "image/jpeg"
	"net/http"
	"net/url"
	"slices"
	"strings"

	_ "golang.org/x/image/webp" // and WebP
	"golang.org/x/net/html"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/content"
	"github.com/lantervale/light/pkg/cookies"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/frames"
	"github.com/lantervale/light/pkg/identity"
	"github.com/lantervale/light/pkg/localnet"
	"github.com/lantervale/light/pkg/netlog"
)

// Who asks for what, as the network panel names it.
const (
	byRefresh  = "refresh"
	byDocument = "document"
	byFrame    = "frame"
	byPicture  = "picture"
)

// The schemes of the addresses light opens.
const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// asking is ctx for one request of the page ctx is for, by what it is
// for.
func asking(ctx context.Context, dest identity.Destination) context.Context {
	return fetch.Asking(ctx, identity.Request{Destination: dest})
}

// Loader loads pages through its client.
type Loader struct {
	Client *fetch.Client
	// NoPictures leaves a page's pictures unfetched, and NoRefresh a
	// page that refreshes elsewhere at once where it is.
	NoPictures, NoRefresh bool
	// Zoom is the zoom pages are built at, 1 when 0.
	Zoom float32
	// Scripts keeps a page's scripts, fetched with it, and builds it as
	// they need it (tree.go).
	Scripts bool
	// Frame is the frame the pages are loaded for, nil for a tab's own.
	Frame *Frame
	// Exact draws the pages' text at its exact coverage, without stem
	// darkening, unless a page says how its own is smoothed: for a light
	// a test runner compares the pictures of.
	Exact bool
}

// Inline is the document of a frame's srcdoc, built as a page of the
// web is: what it links to is read from base, the address of the
// document that holds the frame, whose space it shares.
func (l *Loader) Inline(ctx context.Context, base *url.URL, space localnet.Space, source string, links Links) *Page {
	page := &Page{Address: "about:srcdoc", Status: http.StatusOK, Kind: content.Page, Media: "text/html",
		Space: space}
	l.build(ctx, page, &fetch.Answer{Address: page.Address, URL: base, Body: []byte(source), Space: space}, links)
	return page
}

// by is who asks for what the documents the Loader loads need.
func (l *Loader) by() string { return l.named(byDocument) }

// named is who asks for something of a page, own for a tab's: what a
// frame's document needs is asked by the frame.
func (l *Loader) named(own string) string {
	if l.Frame != nil {
		return byFrame
	}
	return own
}

// destination is what the documents the Loader loads are for.
func (l *Loader) destination() identity.Destination {
	if l.Frame != nil {
		return identity.Iframe
	}
	return identity.Document
}

// within is ctx for the requests the document at page makes: a third
// party's by the tab's page.
func (l *Loader) within(ctx context.Context, page *url.URL) context.Context {
	switch f := l.Frame; {
	case f == nil || f.Top == nil:
		return cookies.Within(ctx, page)
	case f.Opaque:
		return cookies.Without(ctx)
	case frames.Partition(f.Top, append(slices.Clip(f.Chain), page)) != "":
		return cookies.Framed(ctx, f.Top, true)
	default:
		return cookies.Within(ctx, f.Top)
	}
}

// Local is the page of light's own at address, built from its HTML as
// one of the web is; what it links to is fetched.
func (l *Loader) Local(address, source string, links Links) *Page {
	at, err := url.Parse(address)
	if err != nil {
		return &Page{Address: address, Err: err}
	}
	page := &Page{Address: address, Status: http.StatusOK, Kind: content.Page, Media: "text/html"}
	if !opened(address) {
		page.Space = localnet.Loopback
	}
	l.build(context.Background(), page, &fetch.Answer{Address: address, URL: at, Body: []byte(source),
		Space: page.Space}, links)
	return page
}

// Load gets the page at address and what it needs, following its
// redirects and an immediate refresh, and builds it. It runs off the
// frame, until ctx ends it; links are called on the frame, by the links
// of the page once it is shown.
func (l *Loader) Load(ctx context.Context, address string, links Links) *Page {
	return l.load(ctx, address, nil, links)
}

// Post is Load for a form sent with a POST: the page is what answers
// it, or where that answer redirects to.
func (l *Loader) Post(ctx context.Context, sent Submission, links Links) *Page {
	return l.load(fetch.Referring(ctx, sent.Referrer), sent.Address, sent.Form, links)
}

//nolint:nonamedreturns // a page that breaks the build is answered from the recover
func (l *Loader) load(ctx context.Context, address string, form *fetch.Form, links Links) (page *Page) {
	defer func() {
		if r := recover(); r != nil {
			page = &Page{Address: address, Err: fmt.Errorf("the page broke light: %v", r)}
		}
		page.Timing = timingOf(ctx)
	}()
	timing := timingOf(ctx)
	for hops := 0; ; hops++ {
		timing.Mark(Asked)
		stream, err := l.open(fetch.After(ctx, hops), address, form)
		if err != nil {
			return &Page{Address: address, Err: err}
		}
		timing.settle()
		form = nil
		if l.Frame != nil && l.Frame.Admit != nil {
			if err = l.Frame.Admit(stream.Header, stream.URL); err != nil {
				_ = stream.Close()
				return &Page{Address: stream.Address, Status: stream.Status, Err: err}
			}
		}
		if file := saved(stream); file != nil {
			return file
		}
		answer, err := stream.Answer()
		if err != nil {
			return &Page{Address: address, Err: err}
		}
		timing.Mark(Received)
		hops += answer.Hops
		to, refreshed := refresh(answer, l.Scripts)
		if !refreshed || l.NoRefresh {
			return l.read(ctx, answer, links)
		}
		if hops >= l.Client.MaxRedirects() {
			return &Page{Address: answer.Address, Err: fetch.ErrTooManyRedirects}
		}
		if !opened(to) {
			return &Page{Address: answer.Address,
				Err: fmt.Errorf("it refreshes to %s, and only http and https addresses are opened", to)}
		}
		ctx = localnet.From(netlog.As(ctx, byRefresh, netlog.Document), answer.Space)
		if l.Frame == nil {
			ctx = cookies.From(ctx, answer.URL)
		}
		address, ctx = to, l.refreshing(ctx, answer.URL)
	}
}

// refreshing is ctx for the navigation a page's refresh begins: the
// page's doing, not the user's, and from the page.
func (l *Loader) refreshing(ctx context.Context, page *url.URL) context.Context {
	return fetch.Asking(fetch.Referring(ctx, ""), identity.Request{Destination: l.destination(), Initiator: page})
}

// accepted is what a navigation says it accepts.
var accepted = identity.Accept(identity.Document) //nolint:gochecknoglobals // a constant a function gives

func (l *Loader) open(ctx context.Context, address string, form *fetch.Form) (*fetch.Stream, error) {
	if form != nil {
		return l.Client.Post(ctx, address, accepted, *form)
	}
	return l.Client.Open(ctx, address, accepted)
}

func opened(address string) bool {
	u, err := url.Parse(address)
	return err == nil && (u.Scheme == schemeHTTP || u.Scheme == schemeHTTPS)
}

// sniffed is how much of an answer says what it is.
const sniffed = 512

// saved is the page of an answer that is saved rather than shown: one
// its server sends as an attachment, or of a kind light neither reads
// nor draws. Its body is left unread, whatever its size: a download
// asks for it again. It is nil for an answer that is shown.
func saved(stream *fetch.Stream) *Page {
	disposition := stream.Header.Get("Content-Disposition")
	kind, media := content.Of(stream.Header.Get("Content-Type"), stream.Peek(sniffed), stream.Address)
	if !stream.OK() || len(stream.Peek(1)) == 0 || (kind != content.Download && !content.Attachment(disposition)) {
		return nil
	}
	defer stream.Close()
	name := content.SaveName(disposition, stream.Address, media)
	return &Page{Address: stream.Address, Status: stream.Status, Kind: content.Download, Media: media,
		FileName: name, Title: name}
}

// read makes the page of an answer, by its status, then by its kind.
func (l *Loader) read(ctx context.Context, answer *fetch.Answer, links Links) *Page {
	page := &Page{Address: answer.Address, Status: answer.Status, Space: answer.Space}
	status := fmt.Sprintf("%d %s", answer.Status, http.StatusText(answer.Status))
	switch {
	case answer.Status == http.StatusUnauthorized || answer.Status == http.StatusProxyAuthRequired:
		page.Err = fmt.Errorf("it asks for a sign-in, which light does not do (%s)", status)
		return page
	case !answer.OK() && len(answer.Body) == 0:
		page.Err = fmt.Errorf("the server answered %s", status)
		return page
	case len(answer.Body) == 0:
		page.Empty = true
		return page
	}
	page.Kind, page.Media = content.Of(answer.Header.Get("Content-Type"), answer.Body, answer.Address)
	page.FileName = content.FileName(answer.Header.Get("Content-Disposition"), answer.Address)
	page.Title = page.FileName
	switch page.Kind {
	case content.Page:
		l.build(ctx, page, answer, links)
	case content.Picture:
		page.Body = content.Drawable(answer.Body)
		if config, _, err := image.DecodeConfig(bytes.NewReader(page.Body)); err == nil {
			page.Size = image.Pt(config.Width, config.Height)
			page.Title = fmt.Sprintf("%s (%d×%d)", page.FileName, config.Width, config.Height)
		}
	case content.Text:
		page.Source = answer.Text()
		l.plain(page, answer, links)
	case content.Download:
	}
	return page
}

// plain gives a document sent as XML that is no page (a feed, a
// sitemap, any vocabulary of its own) the tree scripts read it by,
// when it is well-formed and loaded for scripts: light shows its text
// as it was sent, and nothing a script changes in the tree.
func (l *Loader) plain(page *Page, answer *fetch.Answer, links Links) {
	if !l.Scripts || !IsXML(page.Media) {
		return
	}
	root, prefixes, err := ParseXML(cut(DecodeXML(answer.Body, answer.Header.Get("Content-Type"))))
	if err != nil {
		return
	}
	b := newBuilder(l, answer.URL, links)
	b.doc = ui.NewDocument(options()...)
	b.tree.xml, b.tree.prefixes, b.tree.unshown = true, prefixes, true
	b.tree.root, b.tree.doc = root, b.doc
	page.Tree, page.Frames, page.Base = b.tree, b.frames, answer.URL
}

// build reads an HTML answer and builds its Document, its sheets and its
// icon fetched meanwhile: a frame's document has no icon.
func (l *Loader) build(ctx context.Context, page *Page, answer *fetch.Answer, links Links) {
	root, prefixes, err := l.parse(page, answer)
	if err != nil {
		page.Err = err
		return
	}
	timing := timingOf(ctx)
	timing.Mark(Parsed)
	b := newBuilder(l, answer.URL, links)
	if b.tree != nil {
		b.tree.xml, b.tree.prefixes = IsXML(page.Media), prefixes
	}
	b.encode = encoderOf(answer.Body, answer.Header.Get("Content-Type"))
	if !IsXML(page.Media) {
		page.Encoding = EncodingOf(answer.Body, answer.Header.Get("Content-Type"))
	}
	ctx = fetch.By(l.within(netlog.As(ctx, l.by(), netlog.Other), answer.URL), answer.URL)
	ctx = localnet.From(ctx, answer.Space)
	icon := make(chan []byte, 1)
	if l.Frame != nil {
		icon <- nil
	} else {
		at := b.icons(root)
		go func() { icon <- b.icon(ctx, at) }()
	}
	b.load(ctx, root)
	timing.Mark(Gathered)
	page.Title, page.Doc = titleOf(root), b.document(ctx, root)
	timing.Mark(Built)
	page.Icon = <-icon
	page.Scripts, page.Tree, page.Frames, page.Base = b.ordered(), b.tree, b.frames, b.base
	if err = ctx.Err(); err != nil {
		page.Err = err
	}
}

// parse reads an answer into its tree: by XML's rules for a document
// sent as XML, which is shown whole or not at all, else as HTML.
func (l *Loader) parse(page *Page, answer *fetch.Answer) (*html.Node, Prefixes, error) {
	if IsXML(page.Media) {
		page.Source = DecodeXML(answer.Body, answer.Header.Get("Content-Type"))
		root, prefixes, err := ParseXML(cut(page.Source))
		if err != nil {
			return nil, nil, fmt.Errorf("its XML is not well-formed, and nothing of it is shown: %w", err)
		}
		return root, prefixes, nil
	}
	page.Source = answer.Text()
	root, err := html.ParseWithOptions(strings.NewReader(cut(page.Source)), html.ParseOptionEnableScripting(l.Scripts))
	if err != nil {
		return nil, nil, fmt.Errorf("reading its HTML: %w", err)
	}
	if l.Scripts {
		writtenIn(cut(page.Source)).restore(root)
	}
	return root, nil, nil
}

// refresh is where an answer says to go at once, in a Refresh header or,
// for HTML, a meta refresh: one that waits is left alone, and so is one
// in a noscript when the page's scripts run.
func refresh(answer *fetch.Answer, scripts bool) (string, bool) {
	value := answer.Header.Get("Refresh")
	if kind, _ := content.Of(answer.Header.Get("Content-Type"), answer.Body, answer.Address); value == "" &&
		kind == content.Page {
		value = metaRefresh(answer.Text(), scripts)
	}
	seconds, to, ok := fetch.Refresh(value, answer.URL)
	return to, ok && seconds == 0
}

// Picture gets the picture at address for a page to draw or the player
// to save.
func (l *Loader) Picture(ctx context.Context, address string) ([]byte, error) {
	answer, err := l.Client.Get(asking(netlog.As(ctx, l.named(byPicture), netlog.Image), identity.Image), address,
		identity.Accept(identity.Image))
	switch {
	case err != nil:
		return nil, err
	case !answer.OK():
		return nil, fmt.Errorf("the server answered %d %s", answer.Status, http.StatusText(answer.Status))
	case len(answer.Body) > maxPicture:
		return nil, errors.New("the picture is over " + megabytes(maxPicture))
	}
	return answer.Body, nil
}

func megabytes(n int) string { return fmt.Sprintf("%d MB", n>>20) }
