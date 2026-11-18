package app

import (
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/history"
	"github.com/lantervale/light/pkg/tabs"
)

// historyPage is where the history is shown.
const historyPage = own + historyName

// visit notes a move to a page of light's own; one of the web is noted
// when it lands, under the address that answered and its title.
func (s *Screen) visit(address string) {
	if _, _, lights := tabs.Own(address); lights && address != tabs.Home && address != historyPage {
		s.record(address, s.title(address))
	}
}

func (s *Screen) record(address, title string) {
	if !s.config.KeepHistory {
		return
	}
	s.visits.Add(address, title, s.opts.Now())
	s.history.touch()
}

func (s *Screen) clearHistory() {
	s.visits.Clear()
	s.history.touch()
}

// historyBody is the history page: a field that filters it, a button
// that clears it, and its visits by day under them.
func historyBody(s *Screen) []*ui.Node {
	list := ui.Div(ui.ID("history-list"))
	filter := ui.Input(ui.ID("history-filter"), ui.Type("search"), ui.Placeholder("Search the history"),
		ui.Attr("aria-label", "Search the history"))
	ui.On(filter, ui.EventInput, func() { s.listVisits(list, s.doc.Value(filter)) })
	forget := ui.Button(ui.ID("history-clear"), ui.Text("Clear history"), ui.OnClick(func() {
		s.clearHistory()
		s.listVisits(list, s.doc.Value(filter))
	}))
	body := []*ui.Node{ui.H1(ui.Text(visited)), ui.Div(ui.ID("history-tools"), filter, forget)}
	if s.unreadable != nil {
		body = append(body, ui.P(ui.ID("history-problem"), ui.Text("The history's file is not in use: "+
			s.unreadable.Error()+".")))
	}
	s.listVisits(list, "")
	return append(body, list)
}

// listVisits fills list with the visits filter keeps: a heading a day,
// then each visit's time, icon, title and address, and a button that
// removes it.
func (s *Screen) listVisits(list *ui.Node, filter string) {
	empty(list)
	days := s.visits.Days(filter)
	if len(days) == 0 {
		list.Append(ui.P(ui.ID("history-none"), ui.Text("No page visited.")))
	}
	for _, day := range days {
		list.Append(ui.H2(ui.Text(day.Date.Format("Monday 2 January 2006"))))
		for _, v := range day.Visits {
			list.Append(s.visitRow(v, func() {
				s.visits.Remove(v)
				s.history.touch()
				s.listVisits(list, filter)
			}))
		}
	}
}

func (s *Screen) visitRow(v history.Visit, remove func()) *ui.Node {
	title := v.Title
	if title == "" {
		title = shownAs(v.Address)
	}
	row := ui.Div(ui.Class("visit"), ui.Span(ui.Class("time"), ui.Text(v.At.Format("15:04"))))
	icon := s.icon(v.Address)
	if icon == nil {
		icon = ui.Span(ui.Class("icon"))
	}
	row.Append(icon)
	row.Append(s.link(title, v.Address), ui.Span(ui.Class("address"), ui.Text(shownAs(v.Address))),
		ui.Button(ui.Class("remove"), ui.Attr("aria-label", "Remove from the history"), ui.Text("×"),
			ui.OnClick(remove)))
	return row
}
