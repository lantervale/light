package web

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
	// maxBytes is how much of a page's or a sheet's text is read.
	maxBytes = 4 << 20
	// maxPicture is the largest picture drawn.
	maxPicture  = 8 << 20
	maxSheets   = 64
	maxScripts  = 64
	maxElements = 8000
	// maxIcons is how many of a site's icons are tried.
	maxIcons = 3
)

// adaptation is one thing light does to a site's page for ui to
// show it: ui reads a CSS subset checked against Chrome and refuses the
// rest with a warning, and has no script engine, where a site is written
// for a browser. The page's sheets themselves go to ui as the site wrote them.
type adaptation struct {
	name, what string
	// option is set on the page's Document.
	option ui.DocumentOption
}

// Adaptation says one of them, for the About page.
type Adaptation struct{ Name, What string }

// Adaptations lists every adaptation of a site's page.
func Adaptations() []Adaptation {
	list := make([]Adaptation, len(adaptations))
	for i, a := range adaptations {
		list[i] = Adaptation{a.name, a.what}
	}
	return list
}

// adaptations is every one of them, each tested under its name
// (adaptations_test.go) and shown at light://about.
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
		what:   "The page scrolls in light's view as in a browser's window, html as high as the page.",
		option: ui.ScrollingViewport(),
	},
	{
		name: "overlay scrollbars",
		what: "The page's scrollbar lies over it and takes no width, so its lines are as wide as in a browser " +
			"whose bar takes no room.",
		option: ui.OverlayScrollbars(),
	},
	{
		name:   "system fonts",
		what:   "font-family names the machine's fonts, where a ui Document knows only the engine's faces.",
		option: ui.SystemFonts(),
	},
	{
		name: "page fonts",
		what: "The fonts of the page's @font-face rules are fetched through light's loader when text first " +
			"needs them (ui.FetchFonts), each asked by its sheet, " + megabytes(maxFont) + " at most; the text " +
			"shows in the next family until they land.",
	},

	{
		name: "svg pictures",
		what: "An SVG picture in an img or a url() is drawn, at the size it shows at (light imports " +
			"ui/svg); what it uses that ember's svg reader doesn't draw (text, clipping, masks, filters, " +
			"patterns…) is a warning naming the file.",
	},

	{
		name: "svg elements",
		what: "An svg element in the page is one picture, ui's svg element holding what it holds as SVG: sized " +
			"by its attributes, its viewBox and the rules naming it, currentColor the colour of its text, and " +
			"drawn again the frame after a script changed it. The rules of the page do not reach the shapes " +
			"inside it, and what ember's svg reader doesn't draw is a warning.",
	},

	{
		name: "chosen pictures",
		what: "An img shows one picture, chosen when it is built and when a script changes it, for the view's " +
			"width at one pixel a point: the candidate of its srcset that fills the slot its sizes gives, or of " +
			"the first source of its picture whose type light draws and whose media holds (min-width and " +
			"max-width; any other condition does not). It is not chosen again when the window is resized.",
	},

	// The page's elements: linked, dropped, build, link and skipped below.
	{
		name: "linked sheets",
		what: "A link is fetched as a sheet when its rel is stylesheet and its media isn't print, the first " +
			strconv.Itoa(maxSheets) + " of a page; one that can't be fetched is empty.",
	},
	{
		name: "elements left out",
		what: "ui has no element for some: " + strings.Join(droppedNames(), ", ") +
			" build nothing, their children neither.",
	},
	{
		name: "scripts",
		what: "No script is fetched or kept unless JavaScript is on in a build that has an engine. Then a " +
			"page's scripts are kept in the order they run, the first " + strconv.Itoa(maxScripts) +
			" of them, one with a src fetched as a sheet is. Scripts read the page as its parser made it: its " +
			"comments, the elements ui has none for (svg, noscript, template, script, style…) and every " +
			"attribute as written; ui is given only what it has an element for, as without scripts. A " +
			"module script is kept with its address alone: the page's scripts fetch it, and what it imports.",
	},
	{
		name: "noscript",
		what: "A page loaded without its scripts is read as a browser that runs none reads it: what a " +
			"noscript holds is the page's, its elements in its place and its sheets among the page's, in " +
			"the head too. With scripts it builds nothing.",
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
		name: "download links",
		what: "A click on an a with a download attribute saves what its href names, under the attribute's " +
			"name when it gives one, and leaves the tab where it is.",
	},
	{
		name: "forms",
		what: "A form is sent by a click on its submit button or by Enter in one of its fields, when its method " +
			"is GET: the tab opens its action, from the page's address, with the names and values of its controls " +
			"as the query, in the page's encoding. A form with another method is not sent.",
	},
	{
		name: "attributes left out",
		what: "on… handlers, autofocus, tabindex, srcset and sizes aren't copied, nor an href on anything but an a.",
	},
	{
		name: "refresh",
		what: "A page that sends elsewhere at once, by a Refresh header or a meta refresh of 0 seconds, is " +
			"followed there as a redirect is and counts as one; a refresh that waits is left alone, and one " +
			"in a noscript when the page's scripts run.",
	},
	{
		name: "limits",
		what: fmt.Sprintf("A page's or a sheet's text is read up to %d MB, a page built up to %d elements, and "+
			"a picture drawn up to %d MB.", maxBytes>>20, maxElements, maxPicture>>20),
	},
}

// options are the Document options of a page: its adaptations', and a
// drag of the right button selecting its text, which is light's own
// and asks ui for what no browser does.
func options() []ui.DocumentOption {
	opts := []ui.DocumentOption{ui.RightDragSelects()}
	for _, a := range adaptations {
		if a.option != nil {
			opts = append(opts, a.option)
		}
	}
	return opts
}

// linked reports whether n is a link to a sheet the page is shown with.
func linked(n *html.Node) bool {
	return n.DataAtom == atom.Link && strings.EqualFold(attr(n, "rel"), "stylesheet") && attr(n, "media") != "print"
}

//nolint:gochecknoglobals,exhaustive // the elements left out, not every element there is
var dropped = map[atom.Atom]bool{
	atom.Head: true, atom.Script: true, atom.Style: true, atom.Link: true, atom.Meta: true,
	atom.Template: true, atom.Object: true,
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

// skips reports whether an attribute of n stays off the element built
// for it: those skipped, but a script's handlers and a link's href once
// scripts run, and none on an element listed out of sight for them.
func (b *builder) skips(n *html.Node, key string) bool {
	switch {
	case b.tree == nil:
		return skipped(key)
	case strings.HasPrefix(key, "on"):
		b.tree.handlers = true
		return false
	case kept[n.DataAtom]:
		return false
	case key == "href":
		return n.DataAtom != atom.A
	}
	return skipped(key)
}

// link makes a, an a element built from n, a link to its href: the href
// :link matches by, and a click that opens it from the page's address,
// or saves it when n has a download attribute, unless it is a place in
// the page itself or nothing light opens.
func (b *builder) link(a *ui.Node, n *html.Node) {
	href := attr(n, "href")
	if href == "" {
		return
	}
	a.SetAttr("href", href)
	address := b.resolve(href)
	if address == "" || strings.HasPrefix(href, "#") {
		return
	}
	follow := func() { b.open(address, attr(n, "target")) }
	if name, saved := lookup(n, "download"); saved {
		follow = func() { b.links.Save(address, name) }
	}
	ui.OnClick(follow).Apply(a)
	ui.OnPointerEnter(func() { b.links.Over(address) }).Apply(a)
	ui.OnPointerLeave(func() { b.links.Over("") }).Apply(a)
}

// skipped reports whether an attribute stays off the element built: a
// script's handler, what would move light's focus, the pictures an img
// chooses among (chosen), and href, which link sets on an a alone.
func skipped(key string) bool {
	return strings.HasPrefix(key, "on") || key == "autofocus" || key == attrSrcset || key == attrSizes ||
		key == "tabindex" || key == "href"
}

// cut is text up to what is read of a page or a sheet.
func cut(text string) string {
	return text[:min(len(text), maxBytes)]
}

// metaRefresh is the content of the first meta refresh of an HTML page,
// "" for none: what a noscript holds is the page's only when its scripts
// do not run.
func metaRefresh(page string, scripts bool) string {
	root, err := html.ParseWithOptions(strings.NewReader(cut(page)), html.ParseOptionEnableScripting(scripts))
	if err != nil {
		return ""
	}
	var find func(n *html.Node) string
	find = func(n *html.Node) string {
		if n.DataAtom == atom.Meta && strings.EqualFold(attr(n, "http-equiv"), "refresh") {
			return attr(n, "content")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if content := find(c); content != "" {
				return content
			}
		}
		return ""
	}
	return find(root)
}
