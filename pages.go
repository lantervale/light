package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/lantervale/ember/twod"
	"github.com/lantervale/ember/ui"
)

// site is one page the explorer knows, found at ember://<its name>.
type site struct {
	title, summary string
	body           func(e *explorer) []*ui.Node
}

var names = []string{"home", "guide", "examples", "about"}

var sites map[string]site

// The pages link to each other through the explorer, which shows them
// from sites, so the map is filled once both exist.
func init() {
	sites = map[string]site{
		"home": {"New Tab", "The start page: a search box and the pages this explorer knows.", homePage},
		"guide": {"The ui guide", "Screens are HTML elements built from Go and styled by a CSS subset: flex, grid, the cascade.",
			func(e *explorer) []*ui.Node {
				return []*ui.Node{
					ui.H1(ui.Text("The ui guide")),
					ui.P(ui.Text("A screen is a Document: HTML elements built from Go, styled by a CSS subset checked " +
						"against headless Chrome, and added to a window as an entity.")),
					ui.H2(ui.Text("Elements")),
					ui.P(ui.Text("Every element is a function of arguments: attributes, classes, styles, callbacks " +
						"and children. This page is a column of h1, h2 and p inside a scrolled div.")),
					ui.H2(ui.Text("Styles")),
					ui.P(ui.Text("Lay screens out with flex or grid. Selectors, the cascade, var() and calc() work; " +
						"@media and animation are refused with a warning, never a crash.")),
					ui.H2(ui.Text("Next")),
					ui.P(e.link("See the examples", "ember://examples"), ui.Text(" or read "),
						e.link("about this explorer", "ember://about"), ui.Text(".")),
				}
			}},
		"examples": {"Examples", "Small programs, one per guide page: sprites, tile maps, lights, text, input, sound.",
			func(e *explorer) []*ui.Node {
				list := ui.Div(ui.ID("list"))
				for _, row := range [][2]string{
					{"19-sprites", "sprites from an embedded PNG"},
					{"26-tilemap", "a tile map drawn from a picture of the level"},
					{"36-point-lights", "a flickering torch and soft shadows"},
					{"44-text", "the engine's faces and a wrapped paragraph"},
					{"94-search-page", "a search home page from plain elements"},
				} {
					list.Append(ui.Div(ui.Strong(ui.Text(row[0])), ui.Text(": "+row[1])))
				}
				return []*ui.Node{
					ui.H1(ui.Text("Examples")),
					ui.P(ui.Text("Run any of them with go run ./examples/<name>.")),
					list,
					ui.P(e.link("Back to the guide", "ember://guide")),
				}
			}},
		"about": {"About", "What this explorer is: tabs, history and an address bar over pages built in Go and the web.",
			func(e *explorer) []*ui.Node {
				return []*ui.Node{
					ui.H1(ui.Text("About this explorer")),
					ui.P(ui.Text("Tabs, an address bar and a history over pages built in Go, and over the web: " +
						"an http or https address is fetched with its stylesheets and pictures and built as " +
						"a Document of its own. No script runs, and the CSS is the subset ui knows.")),
					ui.P(ui.Text("The tabs are the window's title bar (platform.OwnTitleBar): the empty part of " +
						"the strip drags the window, and a double click there zooms it. The address of a link " +
						"under the pointer, and the site a page is waited for, show in a bubble at the bottom-left.")),
					ui.P(ui.Text("Try one: "), e.link("https://example.com", "https://example.com"), ui.Text(".")),
					ui.H2(ui.Text("What it changes in a page")),
					ui.P(ui.Text("A site is written for a browser, so the explorer adapts its page before ui has it:")),
					adapted(),
				}
			}},
	}
}

// adapted lists the adaptations of a site's page.
func adapted() *ui.Node {
	list := ui.Div(ui.ID("list"))
	for _, a := range adaptations {
		list.Append(ui.Div(ui.Strong(ui.Text(a.name)), ui.Text(": "+a.what)))
	}
	return list
}

// locate splits address into the site it names and its search query.
func locate(address string) (name, query string) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "ember" {
		return "", ""
	}
	return u.Host, u.Query().Get("q")
}

func (e *explorer) title(address string) string {
	name, query := locate(address)
	if name == "search" {
		return query + " - Search"
	}
	if s, ok := sites[name]; ok {
		return s.title
	}
	if page := e.remotes[address]; page != nil && page.title != "" {
		return page.title
	}
	return shownAs(address)
}

func (e *explorer) page(address string) *ui.Node {
	name, query := locate(address)
	var body []*ui.Node
	switch s, ok := sites[name]; {
	case name == "search":
		body = e.results(query)
	case ok:
		body = s.body(e)
	case strings.HasPrefix(address, "http://"), strings.HasPrefix(address, "https://"):
		if body = e.web(address); e.host != nil {
			name = "hosted"
		}
	default:
		body = e.unreachable(address, "this explorer opens ember://, http:// and https:// addresses")
	}
	page := ui.Article(ui.ID("page"), ui.Class(name))
	page.Append(body...)
	return page
}

// web is the page at an address on the web: fetched off the frame the
// first time it is shown, an empty page until Update has it, then its
// own Document drawn in a canvas, so its sheets style nothing of the
// explorer's.
func (e *explorer) web(address string) []*ui.Node {
	page := e.remotes[address]
	if page == nil && e.loads[address] == nil {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		l := &load{cancel}
		e.loads[address] = l
		links := linker{open: e.open, over: e.point}
		go func() {
			defer cancel()
			e.loaded <- fetched{address, fetch(ctx, address, links), l}
		}()
	}
	switch {
	case page == nil:
		return []*ui.Node{ui.P(ui.ID("loading"))}
	case page.err != nil:
		return e.unreachable(address, page.err.Error())
	}
	e.host = ui.Canvas(func(c *twod.Canvas) {
		defer func() {
			if r := recover(); r != nil {
				page.err = fmt.Errorf("the page broke the explorer: %v", r)
			}
		}()
		page.doc.Draw(c)
	}, ui.ID("web"))
	return []*ui.Node{e.host}
}

func (e *explorer) unreachable(address, why string) []*ui.Node {
	return []*ui.Node{
		ui.H1(ui.Text("This page can't be reached")),
		ui.P(ui.Code(ui.Text(address)), ui.Text(": "+why+".")),
		ui.P(e.link("Go to the start page", home)),
	}
}

func homePage(e *explorer) []*ui.Node {
	search := ui.Input(ui.ID("search"), ui.Type("search"), ui.Placeholder("Search the pages or type an address"),
		ui.Attr("autofocus", ""))
	ui.On(search, ui.EventSubmit, func() { e.open(Resolve(e.doc.Value(search))) })
	tiles := ui.Div(ui.ID("tiles"))
	for _, name := range names[1:] {
		s := sites[name]
		tiles.Append(ui.Button(ui.ID("go-"+name), ui.Class("tile"),
			ui.Strong(ui.Text(s.title)), ui.Span(ui.Text(s.summary)),
			ui.OnClick(func() { e.open("ember://" + name) })))
	}
	tiles.Append(ui.Button(ui.ID("go-demo"), ui.Class("tile"),
		ui.Strong(ui.Text("CSS Zen Garden")), ui.Span(ui.Text("A page of the web, to see what the explorer draws: csszengarden.com")),
		ui.OnClick(func() { e.open(demo) })))
	return []*ui.Node{ui.Div(ui.ID("wordmark"), ui.Text("explorer")), search, tiles}
}

func (e *explorer) results(query string) []*ui.Node {
	body := []*ui.Node{ui.H1(ui.Text("Results for “" + query + "”"))}
	words := strings.Fields(strings.ToLower(query))
	for _, name := range names {
		s := sites[name]
		text := strings.ToLower(s.title + " " + s.summary)
		if len(words) == 0 || !containsAll(text, words) {
			continue
		}
		address := "ember://" + name
		body = append(body, ui.Div(ui.Class("result"),
			ui.Div(ui.Class("address"), ui.Text(address)), e.link(s.title, address), ui.P(ui.Text(s.summary))))
	}
	if len(body) == 1 {
		body = append(body, ui.P(ui.ID("none"), ui.Text("No page matches.")))
	}
	return body
}

func containsAll(text string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

// link is an anchor to address reading text.
func (e *explorer) link(text, address string) *ui.Node {
	return e.anchor(address, ui.Text(text))
}

// anchor opens address in the shown tab, and names it in the status
// bubble while the pointer is over it.
func (e *explorer) anchor(address string, args ...ui.Arg) *ui.Node {
	a := ui.Span(ui.Class("link"), ui.TabIndex(0),
		ui.OnClick(func() { e.open(address) }),
		ui.OnPointerEnter(func() { e.point(address) }),
		ui.OnPointerLeave(func() { e.point("") }))
	for _, arg := range args {
		arg.Apply(a)
	}
	return a
}
