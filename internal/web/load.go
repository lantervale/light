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
	"strings"

	_ "golang.org/x/image/webp" // and WebP
	"golang.org/x/net/html"

	"github.com/lantervale/light/pkg/content"
	"github.com/lantervale/light/pkg/cookies"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/netlog"
)

// Who asks for what, as the network panel names it.
const (
	byRefresh  = "refresh"
	byDocument = "document"
	byPicture  = "picture"
)

// The schemes of the addresses light opens.
const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// drawn are the pictures light draws, as it asks for them: no type it
// would be sent and could not read.
const drawn = "image/png, image/jpeg, image/gif, image/webp, image/svg+xml"

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
	// Viewport is the size of the view pages show in, which an img's
	// srcset is chosen for; a common one without it.
	Viewport func() (width, height float32)
}

// Local is the page of light's own at address, built from its HTML as
// one of the web is; what it links to is fetched.
func (l *Loader) Local(address, source string, links Links) *Page {
	at, err := url.Parse(address)
	if err != nil {
		return &Page{Address: address, Err: err}
	}
	page := &Page{Address: address, Status: http.StatusOK, Kind: content.Page, Media: "text/html"}
	l.build(context.Background(), page, &fetch.Answer{Address: address, URL: at, Body: []byte(source)}, links)
	return page
}

// Load gets the page at address and what it needs, following its
// redirects and an immediate refresh, and builds it. It runs off the
// frame, until ctx ends it; links are called on the frame, by the links
// of the page once it is shown.
//
//nolint:nonamedreturns // a page that breaks the build is answered from the recover
func (l *Loader) Load(ctx context.Context, address string, links Links) (page *Page) {
	defer func() {
		if r := recover(); r != nil {
			page = &Page{Address: address, Err: fmt.Errorf("the page broke light: %v", r)}
		}
	}()
	for hops := 0; ; hops++ {
		stream, err := l.Client.Open(fetch.After(ctx, hops), address, "text/html, */*")
		if err != nil {
			return &Page{Address: address, Err: err}
		}
		if file := saved(stream); file != nil {
			return file
		}
		answer, err := stream.Answer()
		if err != nil {
			return &Page{Address: address, Err: err}
		}
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
		address, ctx = to, cookies.From(netlog.As(ctx, byRefresh, netlog.Document), answer.URL)
	}
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
	page := &Page{Address: answer.Address, Status: answer.Status}
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
	case content.Download:
	}
	return page
}

// build reads an HTML answer and builds its Document, its sheets and its
// icon fetched meanwhile.
func (l *Loader) build(ctx context.Context, page *Page, answer *fetch.Answer, links Links) {
	page.Source = answer.Text()
	root, err := html.ParseWithOptions(strings.NewReader(cut(page.Source)), html.ParseOptionEnableScripting(l.Scripts))
	if err != nil {
		page.Err = fmt.Errorf("reading its HTML: %w", err)
		return
	}
	if l.Scripts {
		writtenIn(cut(page.Source)).restore(root)
	}
	b := newBuilder(l, answer.URL, links)
	b.encode = encoderOf(answer.Body, answer.Header.Get("Content-Type"))
	ctx = cookies.Within(netlog.As(ctx, byDocument, netlog.Other), answer.URL)
	icon := make(chan []byte, 1)
	go func() { icon <- b.icon(ctx, root) }()
	b.load(ctx, root)
	page.Title, page.Doc, page.Icon = titleOf(root), b.document(ctx, root), <-icon
	page.Scripts, page.Tree = b.ordered(), b.tree
	if err = ctx.Err(); err != nil {
		page.Err = err
	}
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
	answer, err := l.Client.Get(netlog.As(ctx, byPicture, netlog.Image), address,
		drawn)
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
