package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/search"
)

const (
	timetable = "https://ferry.test/times"
	votes     = "https://votes.test/ferry"
)

// known gives this device three pages: one visited twice, one visited
// once and later, one kept and never visited.
func (s *shown) known() {
	day := time.Date(2026, time.March, 1, 9, 0, 0, 0, time.UTC)
	for i, v := range []struct{ address, title string }{
		{timetable, "Ferry times"}, {"https://crypt.test/", "A crypt"}, {timetable, "Ferry times"},
		{votes, "Council votes for a ferry"},
	} {
		s.Profile.visits.Add(v.address, v.title, day.Add(time.Duration(i)*time.Hour))
	}
	s.marks.Add("https://shipyard.test/", "The ferry's shipyard", "", day)
}

// retypes gives the address field the focus and types text over what
// it holds.
func (s *shown) retypes(text string) {
	s.t.Helper()
	s.click("address")
	s.command(input.KeyA)
	s.types(text)
}

// steps moves through the list as Alt with the arrow k does.
func (s *shown) steps(k input.Key) {
	s.chord(input.KeyLeftAlt, k)
	s.frame()
	s.frame()
}

func (s *shown) completes() bool { return s.completeOpen() }

// offered are the rows of the list as they are announced, and the id of
// the one that is highlighted, "" for none.
func (s *shown) offered() ([]string, string) {
	s.t.Helper()
	if !s.completes() {
		return nil, ""
	}
	var rows []string
	chosen := ""
	for _, row := range s.completing.root.Children() {
		label, _ := row.Attr("aria-label")
		rows = append(rows, row.ID()+" "+label)
		if on, _ := row.Attr("aria-selected"); on == "true" {
			chosen = row.ID()
		}
	}
	active, _ := s.address.Attr("aria-activedescendant")
	if active != chosen {
		s.t.Fatalf("the field names %q as the highlighted row, which is %q", active, chosen)
	}
	return rows, chosen
}

func (s *shown) lists(want ...string) {
	s.t.Helper()
	if got, _ := s.offered(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		s.t.Fatalf("the list offers\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestTypingInTheAddressFieldListsWhatEnterDoesThenThePagesOfThisDeviceAndAsksNothing(t *testing.T) {
	s, o := searchingWith(t, Options{})
	s.known()
	s.click("address")
	s.command(input.KeyA)
	if s.completes() {
		t.Fatal("the list shows for a field that has the focus and was not typed in")
	}
	s.types("ferry")
	s.lists("complete-enter Search DuckDuckGo: ferry",
		"complete-page-0 Ferry times ferry.test/times",
		"complete-page-1 Council votes for a ferry votes.test/ferry",
		"complete-page-2 The ferry's shipyard shipyard.test/")
	if role, _ := s.completing.root.Attr("role"); role != "listbox" || s.doc.Value(s.address) != "ferry" ||
		!s.doc.Focused(s.address) {
		t.Fatalf("the list is a %q, the field holds %q, focused %v", role, s.doc.Value(s.address),
			s.doc.Focused(s.address))
	}
	if got := src(s.node("complete-enter").Children()[0].Children()[0]); got != searchMark {
		t.Fatalf("the search is marked %q", got)
	}
	list, field := s.box("complete"), s.box("field")
	if list.X != field.X || list.W != field.W || list.Y < field.Y+field.H {
		t.Fatalf("the list stands at %v under a field at %v", list, field)
	}
	s.clean()
	s.types(".test")
	s.lists("complete-enter Go to ferry.test", "complete-page-0 Ferry times ferry.test/times")
	s.command(input.KeyA)
	s.types(timetable)
	s.lists("complete-enter Go to ferry.test/times")
	s.command(input.KeyA)
	s.press(input.KeyBackspace)
	if s.completes() {
		t.Fatal("the list shows for an empty field")
	}
	s.configure(func(c *config.Config) { c.SearchEngine = search.Own })
	s.types("crypt")
	s.lists("complete-enter Search light's own pages: crypt", "complete-page-0 A crypt crypt.test/")
	if got := src(s.node("complete-enter").Children()[0].Children()[0]); got != logo {
		t.Fatalf("a search of light's own pages is marked %q", got)
	}
	for range 30 {
		s.frame()
	}
	if len(o.asked) != 0 {
		t.Fatalf("typing asked for %v: nothing is to go before Enter", o.asked)
	}
}

func TestAltWithAnArrowMovesThroughTheAddressFieldsListAndEnterTakesTheHighlightedRow(t *testing.T) {
	s, o := searchingWith(t, Options{})
	s.known()
	s.retypes("ferry")
	for i, want := range []string{"complete-enter", "complete-page-0", "complete-page-1", "complete-page-2", "",
		"complete-enter"} {
		s.steps(input.KeyDown)
		if _, got := s.offered(); got != want || !s.doc.Focused(s.address) || s.doc.Value(s.address) != "ferry" {
			t.Fatalf("Alt+Down %d highlights %q, want %q; the field focused %v, holding %q", i+1, got, want,
				s.doc.Focused(s.address), s.doc.Value(s.address))
		}
	}
	s.clean()
	s.steps(input.KeyUp)
	s.steps(input.KeyUp)
	if _, got := s.offered(); got != "complete-page-2" {
		t.Fatalf("Alt+Up from no row highlights %q", got)
	}
	s.steps(input.KeyUp)
	s.types("y")
	if _, got := s.offered(); got != "" || s.doc.Value(s.address) != "ferryy" {
		t.Fatalf("typing leaves %q highlighted, the field holding %q", got, s.doc.Value(s.address))
	}
	s.press(input.KeyBackspace)
	s.steps(input.KeyDown)
	s.steps(input.KeyDown)
	s.steps(input.KeyDown)
	if len(o.asked) != 0 {
		t.Fatalf("moving through the list asked for %v", o.asked)
	}
	s.press(input.KeyEnter)
	s.loaded(votes)
	if s.completes() {
		t.Fatal("Enter leaves the list open")
	}
	if got := o.addresses(); len(got) != 1 || got[0] != "GET "+votes {
		t.Fatalf("the row asked for %v", got)
	}
	s.at(votes)
}

func TestEnterInTheAddressFieldWithNoRowHighlightedDoesWhatItDidAndAClickTakesARow(t *testing.T) {
	s, o := searchingWith(t, Options{})
	s.known()
	s.retypes("ferry")
	s.press(input.KeyEnter)
	s.loaded(found("ferry"))
	if s.completes() {
		t.Fatal("Enter leaves the list open")
	}
	for range 10 {
		s.frame()
	}
	if s.completes() {
		t.Fatal("the list comes back once the page shows")
	}
	s.retypes("ferry t")
	s.click("complete-page-0")
	s.loaded(timetable)
	if s.completes() {
		t.Fatal("a click on a row leaves the list open")
	}
	s.at(shownAs(timetable))
	s.retypes("crypt")
	s.click("complete-enter")
	s.loaded(found("crypt"))
	want := []string{"GET " + found("ferry"), "GET " + timetable, "GET " + found("crypt")}
	if got := o.addresses(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("the rows asked for %v, want %v", got, want)
	}
}

func TestTheAddressFieldsListClosesOnEscapeWithTheTextKeptAndWhenTheFieldIsLeft(t *testing.T) {
	s, o := searchingWith(t, Options{})
	s.known()
	s.retypes("ferry")
	s.steps(input.KeyDown)
	s.press(input.KeyEscape)
	if s.completes() || s.doc.Value(s.address) != "ferry" {
		t.Fatalf("Escape leaves the list open %v, the field holding %q", s.completes(), s.doc.Value(s.address))
	}
	if active, has := s.address.Attr("aria-activedescendant"); has {
		t.Fatalf("closed, the field still names the row %q", active)
	}
	for range 5 {
		s.frame()
	}
	if s.completes() {
		t.Fatal("the list comes back by itself after Escape")
	}
	s.types("s")
	if _, chosen := s.offered(); !s.completes() || chosen != "" || s.doc.Value(s.address) != "ferrys" {
		t.Fatalf("typing again shows the list %v, the field holding %q", s.completes(), s.doc.Value(s.address))
	}
	s.clickAt(geom.V2(300, 600))
	if s.completes() {
		t.Fatal("a press on the page leaves the list open")
	}
	s.click("address")
	if s.completes() {
		t.Fatal("the focus alone brings the list back")
	}
	if len(o.asked) != 0 {
		t.Fatalf("the list asked for %v", o.asked)
	}
}

func TestWithTheModelTheAddressFieldsListAsksThePageFirstAsAQuestionMarkDoes(t *testing.T) {
	s, fake, _ := reading(t)
	address := s.browser.Tab().Current()
	s.retypes(ferryQuestion)
	rows, _ := s.offered()
	if len(rows) < 2 || rows[0] != "complete-ask Ask this page: "+ferryQuestion ||
		rows[1] != "complete-enter Search DuckDuckGo: "+ferryQuestion {
		t.Fatalf("the list offers\n%s", strings.Join(rows, "\n"))
	}
	if got := src(s.node("complete-ask").Children()[0].Children()[0]); got != "row-model.svg" {
		t.Fatalf("the question is marked %q", got)
	}
	s.clean()
	if len(fake.Asked()) != 0 {
		t.Fatal("the model is asked before a row is taken")
	}
	s.click("complete-ask")
	s.until("the model is asked", func() bool { return len(fake.Asked()) == 1 })
	s.frame()
	if !s.answerShown() || s.completes() || s.browser.Tab().Current() != address ||
		s.doc.Value(s.address) != "? "+ferryQuestion || s.node("answer-title").Text() != ferryQuestion {
		t.Fatalf("the row left the card open %v, the list %v, the field %q", s.answerShown(), s.completes(),
			s.doc.Value(s.address))
	}
	if text := fake.Asked()[0].Messages[0].Text; !strings.Contains(text, ferryQuestion) ||
		!strings.Contains(text, ferrySource) {
		t.Fatalf("the model is asked %q", text)
	}
	s.replied(fake)
	s.retypes("ferry")
	if s.answerShown() || !s.completes() || s.doc.Value(s.address) != "ferry" {
		t.Fatalf("typing under an answer leaves its card open %v, the list %v, the field holding %q",
			s.answerShown(), s.completes(), s.doc.Value(s.address))
	}
	s.clean()
	s.steps(input.KeyDown)
	s.press(input.KeyEnter)
	s.until("the model is asked again", func() bool { return len(fake.Asked()) == 2 })
	if !s.answerShown() || s.completes() {
		t.Fatalf("Enter on the row left the card open %v, the list %v", s.answerShown(), s.completes())
	}
	s.press(input.KeyEscape)
	s.retypes("? " + ferryQuestion)
	s.lists("complete-ask Ask this page: "+ferryQuestion, "complete-enter Search DuckDuckGo: "+ferryQuestion)
	s.command(input.KeyA)
	s.types("votes.test")
	s.lists("complete-enter Go to votes.test")
}

func TestTheAddressFieldsListShowsNothingOfTheModelWhereItDoesNotAnswer(t *testing.T) {
	srv := harbour(t)
	for name, prepare := range map[string]func(*shown){
		"the model off":   func(s *shown) { s.configure(func(c *config.Config) { c.Model = false }) },
		"the answers off": func(s *shown) { s.assists.Set(answersFeature, false) },
		"a page of light": func(s *shown) { s.Open(configPage); s.frame() },
	} {
		s, fake := fencedOn(t)
		s.visit(srv.URL + "/")
		prepare(s)
		for _, typed := range []string{ferryQuestion, "? " + ferryQuestion} {
			s.retypes(typed)
			s.lists("complete-enter Search DuckDuckGo: " + typed)
			if s.doc.ByID("complete-ask") != nil {
				t.Errorf("with %s the list asks the page", name)
			}
		}
		s.press(input.KeyEnter)
		s.loaded(s.Resolve("? " + ferryQuestion))
		if len(fake.Asked()) != 0 {
			t.Errorf("with %s the model was asked", name)
		}
	}
}

func TestNothingLeavesThisDeviceWhileTheAddressFieldsListShowsWithTheModelOn(t *testing.T) {
	srv := harbour(t)
	fence := &fenced{}
	s, fake := modelled(t, Options{Transport: fence})
	for _, f := range s.features {
		s.assists.Set(f.id, f.id == answersFeature)
	}
	s.configure(func(c *config.Config) { c.Model = true })
	s.frame()
	s.frame()
	s.known()
	s.visit(srv.URL + "/")
	s.retypes("ferry")
	rows, _ := s.offered()
	if len(rows) != 6 || !strings.HasPrefix(rows[0], "complete-ask ") {
		t.Fatalf("the list offers\n%s", strings.Join(rows, "\n"))
	}
	for _, k := range []input.Key{input.KeyDown, input.KeyDown, input.KeyDown, input.KeyUp} {
		s.steps(k)
	}
	s.types(" times")
	s.command(input.KeyA)
	s.types("votes.test/fer")
	for range 30 {
		s.frame()
	}
	s.press(input.KeyEscape)
	fence.Lock()
	asked := len(fence.asked)
	fence.Unlock()
	if asked != 0 || len(fake.Asked()) != 0 {
		t.Fatalf("the list sent %d requests and asked the model %d times before Enter", asked, len(fake.Asked()))
	}
	s.clean()
}

func TestTheAddressFieldsListFitsANarrowWindowAndADarkPageAndGivesWayToTheOtherCards(t *testing.T) {
	s, fake, _ := reading(t)
	s.known()
	s.Profile.visits.Add("https://long.test/"+strings.Repeat("deep/", 40), strings.Repeat("A long ferry title ", 12),
		time.Date(2026, time.March, 2, 9, 0, 0, 0, time.UTC))
	s.configure(func(c *config.Config) { c.Appearance = config.AppearanceDark })
	s.win.SetSize(geom.V2(760, 480))
	s.frame()
	s.frame()
	s.retypes("ferry")
	list, field := s.box("complete"), s.box("field")
	if rows, _ := s.offered(); len(rows) != 6 || list.X != field.X || list.W != field.W || list.Y+list.H > 480 {
		t.Fatalf("in a narrow window the list of %d rows stands at %v under a field at %v", len(rows), list, field)
	}
	s.clean()
	s.command(input.KeyD)
	s.frame()
	if !s.panelShown() || s.completes() {
		t.Fatalf("the bookmark's panel is open %v with the list %v", s.panelShown(), s.completes())
	}
	s.press(input.KeyEscape)
	s.retypes("ferry")
	s.click("robot")
	if !s.cardShown() || s.completes() {
		t.Fatalf("the model's card is open %v with the list %v", s.cardShown(), s.completes())
	}
	s.press(input.KeyEscape)
	s.wonders(fake, ferryQuestion)
	s.retypes("vaults")
	if s.answerShown() || !s.completes() || s.sheets() != 1 {
		t.Fatalf("typing leaves the answer open %v, the list %v, %d cards", s.answerShown(), s.completes(), s.sheets())
	}
	s.clean()
}
