package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/lantervale/ember/ui"
	_ "github.com/lantervale/ember/ui/svg" // the "svg pictures" adaptation
)

const (
	maxBytes    = 4 << 20
	maxSheets   = 16
	maxElements = 8000
	// maxIcons is how many of a site's icons are tried.
	maxIcons = 3
)

// adaptation is one thing the explorer does to a site's page for ui to
// show it: ui reads a CSS subset checked against Chrome and refuses the
// rest with a warning, and runs no script, where a site is written for a
// browser. The page's sheets themselves go to ui as the site wrote them.
type adaptation struct {
	name, what string
	// option is set on the page's Document, on a plain-text page's too
	// when plain.
	option ui.DocumentOption
	plain  bool
}

// adaptations is every one of them, each tested under its name
// (adaptations_test.go) and shown at ember://about.
//
//nolint:gochecknoglobals // the list
var adaptations = []adaptation{
	// The page's Document.
	{
		name: "browser sheets",
		what: "The page's sheets and style attributes are read as a browser reads them (ui.BrowserSheets): " +
			"-webkit- names Chrome reads as plain ones are, what is another browser's goes without a warning, " +
			"@supports is answered and @layer ordered, a width is its content's, the controls a browser " +
			"sizes by their border keeping it, and the page's text is selected and copied as in a browser.",
		option: ui.BrowserSheets(),
	},
	{
		name: "sheet addresses",
		what: "Each sheet goes to ui with the address it was fetched from (ui.From), so its url() are " +
			"fetched from there; a style attribute's are from the page's.",
	},
	{
		name:   "scrolling viewport",
		what:   "The page scrolls in the explorer's view as in a browser's window, html as high as the page.",
		option: ui.ScrollingViewport(), plain: true,
	},
	{
		name: "overlay scrollbars",
		what: "The page's scrollbar lies over it and takes no width, so its lines are as wide as in a browser " +
			"whose bar takes no room.",
		option: ui.OverlayScrollbars(), plain: true,
	},
	{
		name:   "system fonts",
		what:   "font-family names the machine's fonts, where a ui Document knows only the engine's faces.",
		option: ui.SystemFonts(),
	},

	{
		name: "svg pictures",
		what: "An SVG picture in an img or a url() is drawn, at the size it shows at (the explorer imports " +
			"ui/svg); what it uses that ember's svg reader doesn't draw (text, clipping, masks, filters, " +
			"patterns…) is a warning naming the file.",
	},

	// The page's elements: linked, dropped, build, link and skipped below.
	{
		name: "linked sheets",
		what: "A link is fetched as a sheet when its rel is stylesheet and its media isn't print, the first " +
			strconv.Itoa(maxSheets) + " of a page; one that can't be fetched is empty.",
	},
	{
		name: "elements left out",
		what: "No script runs, and ui has no element for some: " + strings.Join(droppedNames(), ", ") +
			" build nothing, their children neither.",
	},
	{
		name: "unknown elements",
		what: "An element ui has none for (a custom element, picture, nobr…) is a div with its attributes " +
			"and children.",
	},
	{
		name: "links",
		what: "An a keeps its href, so :link matches it; a click opens it from the page's address when that " +
			"is http or https and not a place in the page itself (#…), the status bubble naming it under the pointer.",
	},
	{
		name: "attributes left out",
		what: "on… handlers, autofocus, tabindex and srcset aren't copied, nor an href on anything but an a.",
	},
	{
		name: "limits",
		what: fmt.Sprintf("A page or a sheet is read up to %d MB, and a page built up to %d elements.",
			maxBytes>>20, maxElements),
	},
}

// options are the Document options of a page, of a plain-text one when
// plain.
func options(plain bool) []ui.DocumentOption {
	var opts []ui.DocumentOption
	for _, a := range adaptations {
		if a.option != nil && (a.plain || !plain) {
			opts = append(opts, a.option)
		}
	}
	return opts
}

// linked reports whether n is a link to a sheet the page is shown with.
func linked(n *html.Node) bool {
	return n.DataAtom == atom.Link && strings.EqualFold(attr(n, "rel"), "stylesheet") && attr(n, "media") != "print"
}

//nolint:gochecknoglobals // the elements left out
var dropped = map[atom.Atom]bool{
	atom.Head: true, atom.Script: true, atom.Style: true, atom.Link: true, atom.Meta: true,
	atom.Noscript: true, atom.Template: true, atom.Svg: true, atom.Iframe: true, atom.Object: true,
	atom.Embed: true, atom.Video: true, atom.Audio: true, atom.Canvas: true, atom.Dialog: true,
	atom.Source: true, atom.Picture: false,
}

func droppedNames() []string {
	var names []string
	for tag, out := range dropped {
		if out {
			names = append(names, tag.String())
		}
	}
	slices.Sort(names)
	return names
}

// build is the ui element of an HTML tag, a div for one ui has none for.
func build(tag string) *ui.Node {
	if element, ok := elements[tag]; ok {
		return element()
	}
	return ui.Div()
}

// link makes a, an a element, a link to href: the href :link matches by,
// and a click that opens it from the page's address, unless it is a place
// in the page itself or nothing the explorer opens.
func (b *builder) link(a *ui.Node, href string) {
	if href == "" {
		return
	}
	a.SetAttr("href", href)
	if address := b.resolve(href); address != "" && !strings.HasPrefix(href, "#") {
		ui.OnClick(func() { b.links.open(address) }).Apply(a)
		ui.OnPointerEnter(func() { b.links.over(address) }).Apply(a)
		ui.OnPointerLeave(func() { b.links.over("") }).Apply(a)
	}
}

// skipped reports whether an attribute stays off the element built: a
// script's handler, what would move the explorer's focus, pictures ui
// doesn't choose among, and href, which link sets on an a alone.
func skipped(key string) bool {
	return strings.HasPrefix(key, "on") || key == "autofocus" || key == "srcset" || key == "tabindex" ||
		key == "href"
}
