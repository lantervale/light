package app

import (
	"strings"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/search"
	"github.com/lantervale/light/pkg/tabs"
)

// The names of light's own pages, each at tabs.Page(name).
const (
	guideName     = "guide"
	examplesName  = "examples"
	aboutName     = "about"
	bookmarksName = "bookmarks"
	historyName   = "history"
	downloadsName = "downloads"
	configName    = "config"
	scriptsName   = "scripts"
	// categoriesName is the page the categories are edited on: there
	// only while the model helps with them, so it is none of pages.
	categoriesName = "categories"
)

// own is what the address of a page of light's begins with.
const own = tabs.Scheme + "://"

// site is one page of light's own, found at tabs.Page(name).
type site struct {
	name, title, summary string
	body                 func(s *Screen) []*ui.Node
}

// made is a page of light's the model makes, with the feature it is
// of.
type made struct {
	site
	feature string
}

// pages are light's own pages, the start page first: what an address of
// its scheme opens, and what a search, the About page and the page of
// an unknown one list.
func pages() []site {
	return []site{
		{tabs.HomeName, "New Tab", "The start page: a search box and the pages light knows.", homePage},
		{guideName, "The ui guide", "Screens are HTML elements built from Go and styled by a CSS subset: flex, " +
			"grid, the cascade.", guidePage},
		{examplesName, "Examples", "Small programs, one per guide page: sprites, tile maps, lights, text, input, " +
			"sound.", examplesPage},
		{aboutName, "About", "What light is: tabs, a history and an address bar over pages built in Go and the " +
			"web.", aboutPage},
		{bookmarksName, bookmarked, "The pages kept by the star of the address field, by folder.", bookmarksBody},
		{historyName, visited, "Every page visited, newest first, kept between runs.", historyBody},
		{downloadsName, saves, "The files saved from the web, in progress and done.", downloadsBody},
		{configName, settings, "The start page, the downloads' folder, the history, the zoom, the network.",
			configBody},
		{scriptsName, "Scripts", "What light runs of a page's JavaScript: a counter, a list, tabs, a fetch.", nil},
	}
}

// assisted are the pages of light's the model makes, each with the
// feature it is of: they are listed nowhere while it does not help.
func assisted() []made {
	return []made{{site{translateName, translateTitle, "A text typed or pasted, translated by the model on this " +
		"device.", translateBody}, translateFeature}}
}

// lists are light's own pages as its lists have them: those the model
// makes among them only while it does.
func (s *Screen) lists() []site {
	all := pages()
	if s.helps(sortingFeature) {
		all = append(all, site{categoriesName, sortedTitle, "The categories pages are sorted into, in your words, " +
			"and what light does with the pages of each.", categoriesBody})
	}
	for _, page := range assisted() {
		if s.helps(page.feature) {
			all = append(all, page.site)
		}
	}
	return all
}

// pageNamed is light's page of that name.
func (s *Screen) pageNamed(name string) (site, bool) {
	for _, page := range s.lists() {
		if page.name == name {
			return page, true
		}
	}
	for _, page := range assisted() {
		if page.name == name {
			return page.site, true
		}
	}
	return site{}, false
}

// ownPages lists light's pages by their addresses.
func (s *Screen) ownPages() *ui.Node {
	list := ui.Div(ui.ID("own-pages"), ui.Class("list"))
	for _, page := range s.lists() {
		list.Append(ui.Div(s.link(tabs.Page(page.name), tabs.Page(page.name)), ui.Text(": "+page.summary)))
	}
	return list
}

// searches says what the start page's field does with what is typed.
func (s *Screen) searches() string {
	if e := s.engine(); e.ID != search.Own {
		return "Search with " + e.Name + " or type an address"
	}
	return "Search light's own pages or type an address"
}

// switcher is the start page's logo: a click switches the dark mode,
// and it is shown in the colours of the one that is on.
func (s *Screen) switcher() *ui.Node {
	picture := ui.Img(ui.ID("logo"), ui.Attr("src", s.logo()), ui.TabIndex(0), ui.Attr("alt",
		"light's logo: a disc with eight orange rays. It switches the dark mode"),
		ui.OnClick(func() { s.configure(func(c *config.Config) { c.DarkMode = !c.DarkMode }) }))
	s.fields = append(s.fields, func() { picture.SetAttr("src", s.logo()) })
	return picture
}

func guidePage(s *Screen) []*ui.Node {
	return []*ui.Node{
		ui.H1(ui.Text("The ui guide")),
		ui.P(ui.Text("A screen is a Document: HTML elements built from Go, styled by a CSS subset checked " +
			"against headless Chrome, and added to a window as an entity.")),
		ui.H2(ui.Text("Elements")),
		ui.P(ui.Text("Every element is a function of arguments: attributes, classes, styles, callbacks " +
			"and children. This page is a column of h1, h2 and p inside a scrolled div.")),
		ui.H2(ui.Text("Styles")),
		ui.P(ui.Text("Lay screens out with flex or grid. Selectors, the cascade, var() and calc() work; " +
			"animation is refused with a warning, never a crash.")),
		ui.H2(ui.Text("Next")),
		ui.P(s.link("See the examples", own+examplesName), ui.Text(" or read "),
			s.link("about light", own+aboutName), ui.Text(".")),
	}
}

func examplesPage(s *Screen) []*ui.Node {
	list := ui.Div(ui.ID("examples"), ui.Class("list"))
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
		ui.P(s.link("Back to the guide", own+guideName)),
	}
}

func aboutPage(s *Screen) []*ui.Node {
	return []*ui.Node{
		ui.H1(ui.Text("About light")),
		ui.P(ui.Text("light is a small web browser made as a demo of ember: tabs, an address bar and a "+
			"history over pages built in Go, and over the web. An http or https address is fetched with its "+
			"stylesheets and pictures and built as a Document of its own; a picture, a text or a file is "+
			"shown or saved by its type. The CSS is the subset ui knows; a page's scripts run in a build that "+
			"has an engine for them, as far as "), s.link(scriptsPage, scriptsPage), ui.Text(" shows.")),
		ui.P(ui.Text("The tabs are the window's title bar (platform.OwnTitleBar): the empty part of " +
			"the strip drags the window, and a double click there zooms it. The address of a link " +
			"under the pointer, and the site a page is waited for, show in a bubble at the bottom-left.")),
		ui.P(ui.Text("A right click opens a menu for what is under the pointer; from it, or by their keys, "+
			"come the page's source (view-source:), the "), s.link("history", historyPage),
			ui.Text(" and the developer tools, whose Network tab lists every request of the tab.")),
		ui.P(ui.Text("Try one: "), s.link("https://example.com", "https://example.com"), ui.Text(".")),
		ui.H2(ui.Text("What it keeps")),
		s.kept(),
		ui.H2(ui.Text("Its own pages")),
		s.ownPages(),
		ui.H2(ui.Text("What it changes in a page")),
		ui.P(ui.Text("A site is written for a browser, so light adapts its page before ui has it:")),
		adapted(),
	}
}

// kept lists the files light keeps between runs, where they are.
func (s *Screen) kept() *ui.Node {
	list := ui.Div(ui.ID("kept"), ui.Class("list"))
	if s.opts.Dir == "" {
		list.Append(ui.Div(ui.Text("Nothing is kept: this run was given no folder.")))
		return list
	}
	for _, file := range []struct {
		what  string
		store *store
	}{{visited, s.history}, {bookmarked, s.markFile}, {saves + " list", s.saved}, {settings, s.settings}} {
		list.Append(ui.Div(ui.Strong(ui.Text(file.what)), ui.Text(": "+file.store.path(s.opts.Dir))))
	}
	return list
}

// adapted lists the adaptations of a site's page.
func adapted() *ui.Node {
	list := ui.Div(ui.ID("adaptations"), ui.Class("list"))
	for _, a := range web.Adaptations() {
		list.Append(ui.Div(ui.Strong(ui.Text(a.Name)), ui.Text(": "+a.What)))
	}
	return list
}

func (s *Screen) results(query string) []*ui.Node {
	body := []*ui.Node{ui.H1(ui.Text("Results for “" + query + "”"))}
	words := strings.Fields(strings.ToLower(query))
	for _, found := range s.lists() {
		text := strings.ToLower(found.title + " " + found.summary)
		if len(words) == 0 || !containsAll(text, words) {
			continue
		}
		address := tabs.Page(found.name)
		body = append(body, ui.Div(ui.Class("result"),
			ui.Div(ui.Class("address"), ui.Text(address)), s.link(found.title, address), ui.P(ui.Text(found.summary))))
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
