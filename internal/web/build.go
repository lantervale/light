package web

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/content"
	"github.com/lantervale/light/pkg/netlog"
)

// fetchers is how many of a page's sheets are fetched at once.
const fetchers = 8

// pictureTimeout is how long one picture of a page may take.
const pictureTimeout = 15 * time.Second

// icon is the site's icon: the first the page links to (rel icon) that
// light can draw, else /favicon.ico.
func (b *builder) icon(ctx context.Context, root *html.Node) []byte {
	var refs []string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Link &&
			slices.Contains(strings.Fields(strings.ToLower(attr(n, "rel"))), "icon") {
			refs = append(refs, attr(n, "href"))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	for _, ref := range append(refs, "/favicon.ico")[:min(len(refs)+1, maxIcons)] {
		address := b.resolve(ref)
		if address == "" {
			continue
		}
		answer, err := b.loader.Client.Get(netlog.As(ctx, byDocument, netlog.Image), address, "image/*")
		if err != nil || !answer.OK() {
			continue
		}
		if pic := favicon(address, answer.Header.Get("Content-Type"), answer.Body); pic != nil {
			return pic
		}
	}
	return nil
}

// favicon is a site's icon as a picture ui draws, nil for what is none:
// a PNG, JPEG, GIF or SVG as it is, and of an ICO file its widest picture
// stored as a PNG.
func favicon(address, contentType string, data []byte) []byte {
	if kind, _ := content.Of(contentType, data, address); kind != content.Picture {
		return nil
	}
	return content.Drawable(data)
}

func titleOf(n *html.Node) string {
	if n.Type == html.ElementNode && n.DataAtom == atom.Title && n.FirstChild != nil {
		return strings.Join(strings.Fields(n.FirstChild.Data), " ")
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if t := titleOf(c); t != "" {
			return t
		}
	}
	return ""
}

//nolint:gochecknoglobals // the builder's table
var elements = map[string]func(...ui.Arg) *ui.Node{
	"div": ui.Div, "section": ui.Section, "header": ui.Header, "footer": ui.Footer, "nav": ui.Nav,
	"main": ui.Main, "aside": ui.Aside, "article": ui.Article, "p": ui.P, "h1": ui.H1, "h2": ui.H2,
	"h3": ui.H3, "h4": ui.H4, "span": ui.Span, "strong": ui.Strong, "em": ui.Em, "code": ui.Code,
	"kbd": ui.Kbd, "samp": ui.Samp, "pre": ui.Pre, "label": ui.Label, "button": ui.Button,
	"input": ui.Input, "select": ui.Select, "option": ui.Option, "textarea": ui.Textarea,
	"details": ui.Details, "summary": ui.Summary, "table": ui.Table, "thead": ui.Thead,
	"tbody": ui.Tbody, "tfoot": ui.Tfoot, "caption": ui.Caption, "tr": ui.Tr, "th": ui.Th, "td": ui.Td,
	"progress": ui.Progress, "meter": ui.Meter, "img": ui.Img, "hr": ui.Hr, "ul": ui.Ul, "ol": ui.Ol,
	"li": ui.Li, "menu": ui.Menu, "dl": ui.Dl, "dt": ui.Dt, "dd": ui.Dd, "h5": ui.H5, "h6": ui.H6,
	"blockquote": ui.Blockquote, "figure": ui.Figure, "figcaption": ui.Figcaption, "form": ui.Form,
	"fieldset": ui.Fieldset, "legend": ui.Legend, "address": ui.Address, "center": ui.Center,
	"hgroup": ui.Hgroup, "search": ui.Search, "br": ui.Br, "a": ui.A, "b": ui.B, "i": ui.I, "u": ui.U,
	"s": ui.S, "small": ui.Small, "big": ui.Big, "sub": ui.Sub, "sup": ui.Sup, "abbr": ui.Abbr,
	"cite": ui.Cite, "q": ui.Q, "mark": ui.Mark, "time": ui.Time, "var": ui.Var, "font": ui.Font,
	"del": ui.Del, "ins": ui.Ins, "dfn": ui.Dfn, "bdi": ui.Bdi, "bdo": ui.Bdo, "data": ui.Data,
}

// stylesheet is a sheet's text and the address it came from.
type stylesheet struct {
	text string
	from *url.URL
}

// builder makes a page's Document: its sheets fetched first, then the
// HTML tree as ui elements.
type builder struct {
	loader *Loader
	base   *url.URL
	links  Links
	// sheets are the page's style elements and linked stylesheets in
	// document order.
	sheets []stylesheet
	count  int
}

func (b *builder) resolve(ref string) string {
	u, err := b.base.Parse(strings.TrimSpace(ref))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return u.String()
}

// load fetches the stylesheets root links to, a few at a time.
func (b *builder) load(ctx context.Context, root *html.Node) {
	var wg sync.WaitGroup
	limit := make(chan struct{}, fetchers)
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch {
			case n.DataAtom == atom.Style && n.FirstChild != nil:
				b.sheets = append(b.sheets, stylesheet{n.FirstChild.Data, b.base})
			case linked(n) && len(b.sheets) < maxSheets:
				if address := b.resolve(attr(n, "href")); address != "" {
					from, _ := url.Parse(address)
					b.sheets = append(b.sheets, stylesheet{from: from})
					into := &b.sheets[len(b.sheets)-1]
					wg.Add(1)
					go func() {
						defer wg.Done()
						limit <- struct{}{}
						defer func() { <-limit }()
						b.sheet(netlog.As(ctx, byDocument, netlog.Stylesheet), address, into)
					}()
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	// The slice must not grow while fetches write into it.
	b.sheets = make([]stylesheet, 0, maxSheets+countStyles(root))
	walk(root)
	wg.Wait()
}

// sheet fetches the sheet at address into the place kept for it: its
// text, and the address it came from after its redirects, which its
// url() are fetched from. One that cannot be fetched stays empty.
func (b *builder) sheet(ctx context.Context, address string, into *stylesheet) {
	answer, err := b.loader.Client.Get(ctx, address, "text/css, */*")
	if err != nil || !answer.OK() {
		return
	}
	into.text, into.from = cut(answer.Text()), answer.URL
}

func countStyles(n *html.Node) int {
	count := 0
	if n.Type == html.ElementNode && n.DataAtom == atom.Style {
		count = 1
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		count += countStyles(c)
	}
	return count
}

func (b *builder) document(ctx context.Context, root *html.Node) *ui.Document {
	opts := options()
	for _, sheet := range b.sheets {
		opts = append(opts, ui.CSS(sheet.text, ui.From(sheet.from.String())))
	}
	if b.loader.Zoom > 0 {
		opts = append(opts, ui.Zoom(b.loader.Zoom))
	}
	doc := ui.NewDocument(append(opts, ui.FetchImages(b.pictures(ctx)))...)
	var place func(n *html.Node)
	place = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch {
			case c.Type != html.ElementNode:
			case c.DataAtom == atom.Html:
				b.attributes(c, doc.HTML())
				place(c)
			case c.DataAtom == atom.Body:
				b.attributes(c, doc.Body())
				b.children(c, doc.Body())
			default:
				place(c)
			}
		}
	}
	place(root)
	return doc
}

var errNoPictures = errors.New("light is set not to load pictures")

// pictures fetches the page's pictures as ui asks for them, long after
// its load: an http or https address as it is, anything else as a
// reference from the page's address. They are recorded where ctx says,
// and outlive it.
func (b *builder) pictures(ctx context.Context) ui.ImageFetcher {
	detached := context.WithoutCancel(ctx)
	return func(ref string) ([]byte, error) {
		if b.loader.NoPictures {
			return nil, errNoPictures
		}
		address := b.resolve(ref)
		if address == "" {
			return nil, fmt.Errorf("%q is no http or https address", ref)
		}
		timed, cancel := context.WithTimeout(detached, pictureTimeout)
		defer cancel()
		return b.loader.Picture(timed, address)
	}
}

func (b *builder) children(n *html.Node, into *ui.Node) {
	for c := n.FirstChild; c != nil && b.count < maxElements; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode:
			ui.Text(c.Data).Apply(into)
		case c.Type != html.ElementNode || dropped[c.DataAtom]:
		default:
			b.count++
			into.Append(b.element(c))
		}
	}
}

func (b *builder) element(n *html.Node) *ui.Node {
	e := build(n.Data)
	b.attributes(n, e)
	if n.DataAtom == atom.A {
		b.link(e, n)
	}
	b.children(n, e)
	return e
}

func (b *builder) attributes(n *html.Node, e *ui.Node) {
	for _, a := range n.Attr {
		switch {
		case a.Key == "id":
			e.SetID(a.Val)
		case a.Key == "class":
			ui.Class(strings.Fields(a.Val)...).Apply(e)
		case a.Key == "style":
			ui.Style(a.Val).Apply(e)
		case skipped(a.Key):
		default:
			e.SetAttr(a.Key, a.Val)
		}
	}
}

func attr(n *html.Node, name string) string {
	value, _ := lookup(n, name)
	return value
}

// lookup is n's attribute name, and whether it has one.
func lookup(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}
