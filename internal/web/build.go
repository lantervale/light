package web

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/content"
	"github.com/lantervale/light/pkg/identity"
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
		answer, err := b.loader.Client.Get(asking(netlog.As(ctx, b.loader.by(), netlog.Image), identity.Image), address,
			identity.Accept(identity.Image))
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
// a PNG, JPEG, GIF, WebP or SVG as it is, and of an ICO file its widest
// picture stored as a PNG.
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
	// node is the style element or the link it is of.
	node *html.Node
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
	// styled is what the Document has of each, for a page with a tree.
	styled map[*html.Node]Styled
	count  int
	// tree is the page as scripts read it, nil for a page loaded
	// without them, and scripts the page's.
	tree    *Tree
	scripts []*Script
	// doc is the Document built, and encode writes text in the page's
	// encoding, which its forms are sent in.
	doc    *ui.Document
	encode func(text string) string
	// frames are the iframe elements built.
	frames *Frames
}

func newBuilder(l *Loader, base *url.URL, links Links) *builder {
	b := &builder{loader: l, base: base, links: links, frames: &Frames{}}
	if l.Scripts {
		b.tree = &Tree{b: b, shown: map[*html.Node]*ui.Node{}, of: map[*ui.Node]*html.Node{},
			contents: map[*html.Node]*html.Node{}, head: ui.Div()}
	}
	return b
}

func (b *builder) resolve(ref string) string {
	u, err := b.base.Parse(strings.TrimSpace(ref))
	if err != nil || (u.Scheme != schemeHTTP && u.Scheme != schemeHTTPS) {
		return ""
	}
	return u.String()
}

// load fetches the stylesheets root links to, a few at a time.
func (b *builder) load(ctx context.Context, root *html.Node) {
	var wg sync.WaitGroup
	limit := make(chan struct{}, fetchers)
	links := 0
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch {
			case n.DataAtom == atom.Style && n.FirstChild != nil:
				b.sheets = append(b.sheets, stylesheet{n.FirstChild.Data, b.base, n})
			case n.DataAtom == atom.Script && b.tree != nil:
				b.script(ctx, n, &wg, limit)
			case linked(n) && links < maxSheets:
				if address := b.resolve(attr(n, "href")); address != "" {
					links++
					from, _ := url.Parse(address)
					b.sheets = append(b.sheets, stylesheet{from: from, node: n})
					into := &b.sheets[len(b.sheets)-1]
					wg.Add(1)
					go func() {
						defer wg.Done()
						limit <- struct{}{}
						defer func() { <-limit }()
						b.sheet(netlog.As(ctx, b.loader.by(), netlog.Stylesheet), address, into)
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

// script keeps the script element n, its source fetched meanwhile when
// it names one.
func (b *builder) script(ctx context.Context, n *html.Node, wg *sync.WaitGroup, limit chan struct{}) {
	kind := scriptKind(n)
	if kind == "" {
		return
	}
	s := &Script{Address: b.base.String(), Node: n, Module: kind == moduleScript, ImportMap: kind == importMap}
	_, deferred := lookup(n, "defer")
	_, async := lookup(n, "async")
	src, external := lookup(n, "src")
	s.Deferred, s.Async = s.Module || external && (deferred || async), s.Module && async
	if n.FirstChild != nil && !external {
		s.Source = cut(n.FirstChild.Data)
	}
	b.scripts = append(b.scripts, s)
	switch {
	case !external:
		return
	case s.ImportMap:
		s.Skipped = "it is an import map with a src, and one is read only from the page itself"
		return
	}
	if len(b.scripts) > maxScripts {
		s.Skipped = "it is past the first " + strconv.Itoa(maxScripts) + " scripts of the page"
		return
	}
	address := b.resolve(src)
	if address == "" {
		s.Skipped = fmt.Sprintf("its src %q is no http or https address", src)
		return
	}
	s.Address = address
	if s.Module {
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		limit <- struct{}{}
		defer func() { <-limit }()
		s.Source, s.Address, s.Skipped = b.loader.Script(netlog.As(ctx, b.loader.by(), netlog.Script), address)
	}()
}

// ordered are the page's scripts in the order they run: as the page
// has them, those that wait for it (defer, async) after the others.
func (b *builder) ordered() []Script {
	var now, later []Script
	for _, s := range b.scripts {
		if s.Deferred {
			later = append(later, *s)
		} else {
			now = append(now, *s)
		}
	}
	return append(now, later...)
}

// sheet fetches the sheet at address into the place kept for it: its
// text, and the address it came from after its redirects, which its
// url() are fetched from. One that cannot be fetched stays empty.
func (b *builder) sheet(ctx context.Context, address string, into *stylesheet) {
	answer, err := b.loader.Client.Get(asking(ctx, identity.Style), address, identity.Accept(identity.Style))
	if err != nil || !answer.OK() {
		return
	}
	into.text, into.from = cut(answer.Code()), answer.URL
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
		if b.tree == nil {
			opts = append(opts, ui.CSS(sheet.text, ui.From(sheet.from.String())))
		}
	}
	if b.loader.Zoom > 0 {
		opts = append(opts, ui.Zoom(b.loader.Zoom))
	}
	doc := ui.NewDocument(append(opts, ui.FetchImages(b.pictures(ctx)), ui.FetchFonts(b.fonts(ctx)))...)
	b.doc = doc
	if b.tree != nil {
		b.style(doc)
		b.tree.bind(root, doc)
		return doc
	}
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
		case b.tree != nil:
			if u := b.tree.made(c); u != nil {
				into.Append(u)
			}
		case c.Type == html.TextNode:
			ui.Text(c.Data).Apply(into)
		case c.Type != html.ElementNode:
		case c.DataAtom == atom.Noscript:
			b.children(c, into)
		case dropped[c.DataAtom]:
		default:
			b.count++
			into.Append(b.element(c))
		}
	}
}

// element is the ui element of n with what n holds: the one the page's
// tree has of it from then on, when the page has a tree.
func (b *builder) element(n *html.Node) *ui.Node {
	if pictured(n) {
		e := ui.Svg(drawing(n))
		if b.tree != nil {
			b.count++
			b.tree.keep(n, e)
		}
		b.attributes(n, e)
		return e
	}
	if n.DataAtom == atom.Iframe && n.Namespace == "" {
		return b.frame(n)
	}
	e := b.make(n.Data)
	if b.tree != nil {
		b.count++
		b.tree.keep(n, e)
	}
	b.attributes(n, e)
	if n.DataAtom == atom.A && b.tree == nil {
		b.link(e, n)
	}
	b.children(n, e)
	return e
}

// frame is the iframe element of n, empty until whoever shows the page
// gives it a document: what n holds is for browsers that show none. A
// document that panics in it empties it, for whoever shows it to say.
func (b *builder) frame(n *html.Node) *ui.Node {
	e := ui.Iframe(nil, ui.Guarded())
	if b.tree != nil {
		b.count++
		b.tree.keep(n, e)
	}
	b.attributes(n, e)
	b.frames.add(e)
	return e
}

// open opens address where target says, in the page's own tab or frame
// when it says nothing.
func (b *builder) open(address, target string) {
	if target != "" && b.links.Aimed != nil && b.links.Aimed(address, target) {
		return
	}
	b.links.Open(address)
}

func (b *builder) attributes(n *html.Node, e *ui.Node) {
	for _, a := range n.Attr {
		b.attribute(n, e, a.Key, a.Val)
	}
}

// attribute sets on e, the element built for n, an attribute of n's,
// unless it is one that stays off it.
func (b *builder) attribute(n *html.Node, e *ui.Node, key, value string) {
	if n.DataAtom == atom.Img && (key == "src" || key == attrSrcset || key == attrSizes) {
		if address, ok := b.chosen(n); ok {
			e.SetAttr("src", address)
		} else {
			e.RemoveAttr("src")
		}
		return
	}
	if key == "id" || key == "class" || key == "style" || !b.skips(n, key) {
		e.SetAttr(key, value)
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
