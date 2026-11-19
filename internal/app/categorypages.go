package app

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/categories"
	"github.com/lantervale/light/pkg/config"
)

// categoriesPage is where the categories are edited.
const categoriesPage = own + categoriesName

// arrowUp, arrowDown and cross are the glyphs of the buttons that move
// a category and remove it.
const (
	arrowUp   = `<path d="M3 7.5 6 4.5 9 7.5" fill="none" stroke="currentColor" stroke-width="1.5"/>`
	arrowDown = `<path d="M3 4.5 6 7.5 9 4.5" fill="none" stroke="currentColor" stroke-width="1.5"/>`
	cross     = `<path d="M3 3l6 6M9 3l-6 6" fill="none" stroke="currentColor" stroke-width="1.5"/>`
)

// sortingSettings is the settings' part of the categories, there while
// the model helps with them: where they are edited, and how long a
// page keeps the category the model gave it.
func (s *Screen) sortingSettings() *ui.Node {
	g := &s.sorting
	g.lined = ui.Div(ui.ID("category-kept"))
	on := []*ui.Node{ui.H2(ui.Text(sortedTitle)),
		ui.P(ui.Class("hint"), ui.Text("Your categories, what light does with the pages of each and what you chose "+
			"for a site or a page are on "), s.anchor(categoriesPage, ui.ID("category-edit"), ui.Text(categoriesPage)),
			ui.Text(".")),
		s.numberSetting("set-category-hours", "Category kept", "Hours a page keeps the category the model gave it "+
			"before the model reads it again; 0 has it read each time. Kept by the page's address, with the history.",
			config.MinCategoryHours, config.MaxCategoryHours, func(c *config.Config) *int { return &c.CategoryHours })}
	g.reline = func() {
		if helps := s.helps(sortingFeature); helps != (len(g.lined.Children()) > 0) {
			empty(g.lined)
			if helps {
				g.lined.Append(on...)
			}
		}
	}
	s.fields = append(s.fields, g.reline)
	g.reline()
	return g.lined
}

// categoriesBody is the categories' page, one of light's own while the
// model helps with them: each category named, described, tagged, marked
// and given what light does with its pages and the banner it warns
// with, moved and removed; what adds one; and what was chosen for sites
// and pages.
func categoriesBody(s *Screen) []*ui.Node {
	g := &s.sorting
	g.held = ui.Div(ui.ID("category-settings"))
	g.relist = func() { s.refill(g.held, func() { g.held.Append(s.sortingRows()...) }) }
	g.relist()
	return []*ui.Node{ui.H1(ui.Text(sortedTitle)),
		ui.P(ui.Class("hint"), ui.Text("The model on this device reads a page once it has loaded and sorts it into "+
			"the first of these that fits, in this order. It can be wrong, and no mark never means a page is safe.")),
		g.held,
		ui.P(ui.Class("hint"), ui.Text("How long a page keeps its category is one of the "),
			s.anchor(configPage, ui.ID("category-settings-link"), ui.Text("settings")), ui.Text("."))}
}

// changeSorted changes the categories from their page: written, and
// listed again.
func (s *Screen) changeSorted(change func(l *categories.List)) {
	change(s.sorted.list)
	s.resorted()
	if g := &s.sorting; g.relist != nil && s.shows(g.held) {
		g.listed = s.sorted.list.Version()
		g.relist()
	}
}

func (s *Screen) sortingRows() []*ui.Node {
	list := s.sorted.list
	var rows []*ui.Node
	if s.sorted.unread != nil {
		rows = append(rows, ui.P(ui.ID("categories-problem"), ui.Text("The categories' file is not in use: "+
			s.sorted.unread.Error()+".")))
	}
	all := list.All()
	for i, c := range all[:len(all)-1] {
		rows = append(rows, s.sortingSetting(i, c, len(all)-1))
	}
	last := all[len(all)-1]
	rows = append(rows, ui.Div(ui.ID("category-set-default"), ui.Class("category-setting"), ui.Div(ui.Class("line"),
		ui.Span(ui.Class("swatch"), markOf(last.Mark)), ui.Span(ui.Class("fixed"), ui.Text(last.Name)),
		ui.Span(ui.Class("fixed", "what"), ui.Text(last.Description)),
		ui.Span(ui.Class("fixed"), ui.Text(last.Action.String()+", always last")))))
	field := ui.Input(ui.ID("category-set-new"), ui.Placeholder("recipes without a life story first"),
		ui.Attr("aria-label", "Describe the pages that belong in a new category"))
	add := func() {
		s.changeSorted(func(l *categories.List) { l.Add(s.doc.Value(field)) })
	}
	ui.OnSubmit(add).Apply(field)
	rows = append(rows, ui.Div(ui.Class("category-setting", "adding"), ui.Div(ui.Class("line"), field,
		ui.Button(ui.ID("category-set-add"), ui.Text("+ Add"), ui.OnClick(add)))))
	if choices := list.Choices(); len(choices) > 0 {
		rows = append(rows, ui.H3(ui.Text("What you chose, whatever the model makes of the page")))
		for i, choice := range choices {
			where := choice.Site
			if where == "" {
				where = shownAs(choice.Address)
			}
			rows = append(rows, ui.Div(ui.Class("visit", "category-choice"),
				ui.Span(ui.Class("address"), ui.Text(where)), ui.Span(ui.Class("chose"), ui.Text(choice.Category)),
				ui.Button(ui.ID("category-choice-"+strconv.Itoa(i)), ui.Class("remove"),
					ui.Attr("aria-label", "Forget this choice"), ui.Text("×"), ui.OnClick(func() {
						s.changeSorted(func(l *categories.List) { l.Forget(choice) })
					}))))
		}
	}
	return rows
}

// sortingSetting is one category of the page: a line of its mark, its
// name, what belongs in it, what light does with its pages and what
// moves and removes it, then one of the tag its label shows and, for
// one that warns, of its banner and the message it says.
func (s *Screen) sortingSetting(i int, c categories.Category, of int) *ui.Node {
	id := "category-set-" + strconv.Itoa(i)
	edit := func(change func(c *categories.Category)) {
		to := c
		change(&to)
		s.changeSorted(func(l *categories.List) { l.Edit(c.Name, to) })
	}
	marks := categories.Marks()
	swatch := ui.Button(ui.ID(id+"-mark"), ui.Class("swatch"),
		ui.Attr("aria-label", "The mark of "+c.Name+": a "+string(c.Shape)+". A click gives it the next"),
		markOf(c.Mark), ui.OnClick(func() {
			edit(func(c *categories.Category) { c.Mark = marks[(slices.Index(marks, c.Mark)+1)%len(marks)] })
		}))
	name := ui.Input(ui.ID(id+"-name"), ui.Value(c.Name), ui.Attr("aria-label", "Name"))
	what := ui.Input(ui.ID(id+"-what"), ui.Class("what"), ui.Value(c.Description),
		ui.Attr("aria-label", "The pages that belong in "+c.Name))
	renamed := func() { edit(func(c *categories.Category) { c.Name = s.doc.Value(name) }) }
	described := func() { edit(func(c *categories.Category) { c.Description = s.doc.Value(what) }) }
	ui.On(name, ui.EventChange, renamed)
	ui.On(name, ui.EventSubmit, renamed)
	ui.On(what, ui.EventChange, described)
	ui.On(what, ui.EventSubmit, described)
	tagged := ui.Input(ui.ID(id+"-tag"), ui.Class("tag"), ui.Value(c.Tag), ui.Placeholder(c.Name),
		ui.Attr("maxlength", "16"), ui.Attr("aria-label", "The tag the label of a page of "+c.Name+" shows, its "+
			"name when empty"))
	retagged := func() { edit(func(c *categories.Category) { c.Tag = s.doc.Value(tagged) }) }
	ui.On(tagged, ui.EventChange, retagged)
	ui.On(tagged, ui.EventSubmit, retagged)
	more := ui.Div(ui.Class("line", "more"), ui.Span(ui.Class("fixed"), ui.Text("Label")), tagged)
	if c.Action == categories.Warn {
		more.Append(s.bannerSetting(id, c, edit)...)
	}
	does := ui.Select(ui.ID(id+"-does"), ui.Attr("aria-label", "What light does with a page of "+c.Name))
	for _, a := range categories.Actions() {
		option := ui.Option(ui.Value(string(a)), ui.Text(a.String()))
		if a == c.Action {
			ui.Selected().Apply(option)
		}
		does.Append(option)
	}
	ui.On(does, ui.EventChange, func() {
		edit(func(c *categories.Category) { c.Action = categories.Action(s.doc.Value(does)) })
	})
	move := func(by int, label, drawn string) *ui.Node {
		b := ui.Button(ui.ID(fmt.Sprintf("%s-%s", id, label)), ui.Class("move"), ui.Attr("aria-label", "Move "+label),
			ui.Svg(drawn, ui.Attr("viewBox", "0 0 12 12"), ui.Attr("aria-hidden", "true")), ui.OnClick(func() {
				s.changeSorted(func(l *categories.List) { l.Move(c.Name, by) })
			}))
		enable(b, i+by >= 0 && i+by < of)
		return b
	}
	return ui.Div(ui.ID(id), ui.Class("category-setting"), ui.Div(ui.Class("line"), swatch, name, what, does,
		move(-1, "up", arrowUp), move(1, "down", arrowDown),
		ui.Button(ui.ID(id+"-remove"), ui.Class("move"), ui.Attr("aria-label", "Remove "+c.Name),
			ui.Svg(cross, ui.Attr("viewBox", "0 0 12 12"), ui.Attr("aria-hidden", "true")), ui.OnClick(func() {
				s.changeSorted(func(l *categories.List) { l.Remove(c.Name) })
			}))), more)
}

// bannerSetting is the banner of a category that warns: its kind, and
// the message of its reader's it says in place of the model's reason.
func (s *Screen) bannerSetting(id string, c categories.Category,
	edit func(change func(c *categories.Category))) []*ui.Node {
	kind := ui.Select(ui.ID(id+"-banner"), ui.Attr("aria-label", "The banner over a page of "+c.Name))
	for _, b := range categories.Banners() {
		option := ui.Option(ui.Value(string(b)), ui.Text(b.String()))
		if b == c.Banner {
			ui.Selected().Apply(option)
		}
		kind.Append(option)
	}
	ui.On(kind, ui.EventChange, func() {
		edit(func(c *categories.Category) { c.Banner = categories.Banner(s.doc.Value(kind)) })
	})
	message := ui.Input(ui.ID(id+"-message"), ui.Class("what"), ui.Value(c.Message),
		ui.Placeholder("the model's reason"), ui.Attr("maxlength", "160"),
		ui.Attr("aria-label", "What the banner says over a page of "+c.Name+", the model's reason when empty"))
	said := func() { edit(func(c *categories.Category) { c.Message = s.doc.Value(message) }) }
	ui.On(message, ui.EventChange, said)
	ui.On(message, ui.EventSubmit, said)
	return []*ui.Node{ui.Span(ui.Class("fixed", "banner"), ui.Text("Banner")), kind, message}
}

// homeSorted is the start page's count of the pages read this week in
// each category but Default, from what was counted as they were
// judged: nothing while the model does not help with the categories,
// and nothing when no page was.
func (s *Screen) homeSorted() []*ui.Node {
	if !s.helps(sortingFeature) {
		return nil
	}
	counts, total := s.sorted.list.Week(s.opts.Now())
	if len(counts) == 0 {
		return nil
	}
	week := ui.Div(ui.Class("week"))
	for _, c := range counts {
		read := strconv.Itoa(c.Pages) + " pages"
		switch {
		case c.Action == categories.Warn:
			read = strconv.Itoa(c.Pages) + " warned"
		case c.Pages == 1:
			read = "1 page"
		}
		week.Append(ui.Div(ui.Class("row"), ui.Attr("data-category", c.Name),
			ui.Span(ui.Class("name"), markOf(c.Mark), ui.Span(ui.Text(c.Name))),
			ui.Span(ui.Class("track"), ui.Span(ui.Class("share"),
				ui.Style(fmt.Sprintf("width: %d%%; background: %s", max(c.Pages*100/total, 2), c.Colour)))),
			ui.Span(ui.Class("count"), ui.Text(read))))
	}
	return []*ui.Node{ui.Section(ui.ID("home-categories"),
		ui.Div(ui.Class("head"), ui.H2(ui.Text("This week, by your categories"))), week)}
}
