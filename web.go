package main

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/html/charset"

	"github.com/lantervale/ember/ui"
	"github.com/lantervale/ember/ui/web"
)

const fetchers = 8

// remote is a page fetched from the web as a Document of its own, built
// from its HTML and its stylesheets, its pictures fetched by ui as they
// show, or why there is none.
type remote struct {
	title string
	doc   *ui.Document
	// icon is the site's icon as a picture ui draws, nil for none.
	icon []byte
	err  error
}

type fetched struct {
	address string
	page    *remote
	load    *load
}

// loadTimeout is how long a page and its sheets may take.
const loadTimeout = 30 * time.Second

//nolint:gochecknoglobals // one client for every fetch
var client = &http.Client{Timeout: 15 * time.Second}

func get(ctx context.Context, address, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ember-web-explorer/1.0")
	req.Header.Set("Accept", accept)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		resp.Body.Close()
		return nil, fmt.Errorf("the server answered %s", resp.Status)
	}
	return resp, nil
}

func getBytes(ctx context.Context, address, accept string) ([]byte, error) {
	resp, err := get(ctx, address, accept)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
}

// fetch gets the page at address and what it needs, and builds its
// Document. It runs off the frame, until ctx ends it; open is called on
// it, by the links of the page once it is shown.
func fetch(ctx context.Context, address string, links linker) (page *remote) {
	defer func() {
		if r := recover(); r != nil {
			page = &remote{err: fmt.Errorf("the page broke the explorer: %v", r)}
		}
	}()
	resp, err := get(ctx, address, "text/html, text/plain")
	if err != nil {
		return &remote{err: err}
	}
	defer resp.Body.Close()
	kind := resp.Header.Get("Content-Type")
	media, _, _ := mime.ParseMediaType(kind)
	if media != "" && media != "text/plain" && media != "text/html" && media != "application/xhtml+xml" {
		return &remote{err: fmt.Errorf("it is %s, and this explorer shows only HTML and text", media)}
	}
	body, err := charset.NewReader(io.LimitReader(resp.Body, maxBytes), kind)
	if err != nil {
		return &remote{err: err}
	}
	if media == "text/plain" {
		data, err := io.ReadAll(body)
		doc := ui.NewDocument(options(true)...)
		doc.Body().Append(ui.Pre(ui.Text(string(data))))
		return &remote{doc: doc, err: err}
	}
	root, err := html.Parse(body)
	if err != nil {
		return &remote{err: err}
	}
	b := &builder{base: resp.Request.URL, links: links}
	icon := make(chan []byte, 1)
	go func() { icon <- b.icon(ctx, root) }()
	b.load(ctx, root)
	page = &remote{title: titleOf(root), doc: b.document(root), icon: <-icon}
	if err = ctx.Err(); err != nil {
		return &remote{err: err}
	}
	return page
}

// icon is the site's icon: the first the page links to (rel icon) that
// the explorer can draw, else /favicon.ico.
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
		resp, err := get(ctx, address, "image/*")
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
		resp.Body.Close()
		media, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if pic := favicon(address, media, data); err == nil && pic != nil {
			return pic
		}
	}
	return nil
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

// linker is what a page's links do: open an address, and name it while
// the pointer is over one.
type linker struct {
	open, over func(address string)
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
	base  *url.URL
	links linker
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
						if data, err := getBytes(ctx, address, "text/css"); err == nil {
							into.text = string(data)
						}
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

func (b *builder) document(root *html.Node) *ui.Document {
	opts := options(false)
	for _, sheet := range b.sheets {
		opts = append(opts, ui.CSS(sheet.text, ui.From(sheet.from.String())))
	}
	doc := ui.NewDocument(append(opts, ui.FetchImages(web.Images(b.base.String())))...)
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
		b.link(e, attr(n, "href"))
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
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}
