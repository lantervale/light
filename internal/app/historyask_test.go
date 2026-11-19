package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/model"
)

const (
	batteryQuestion = "which arm tops up the batteries"
	byMeaningSub    = "Matched by meaning against pages you visited, on this device."
	searchedSaid    = "Searched what the pages said, not only their titles."
)

// harboured is a light that remembers, the quay's three articles
// read, on the history page.
func harboured(t *testing.T) (*shown, *model.Fake, string) {
	t.Helper()
	srv := quay(t)
	s, fake := remembering(t, Options{
		Now: func() time.Time { return time.Date(2026, time.November, 16, 10, 0, 0, 0, time.UTC) },
	})
	for _, path := range []string{"/ferry", "/charging", "/knots"} {
		s.reads(srv.URL + path)
	}
	s.Open(historyPage)
	s.frame()
	return s, fake, srv.URL
}

// questions types a question in the history's field and enters it,
// then waits for the model to have embedded it.
func (s *shown) questions(question string) {
	s.t.Helper()
	s.click("history-filter")
	s.doc.SetValue(s.node("history-filter"), question)
	s.press(input.KeyEnter)
	s.until("the question is answered", func() bool { return s.asking == nil || !s.asking.pending })
	s.frame()
}

// recalledRows are the matches the history page lists, each as its
// title, its site, its day and its passage.
func (s *shown) recalledRows() []string {
	s.t.Helper()
	var out []string
	for _, row := range s.node("history-list").Children() {
		if !row.HasClass("recalled") {
			continue
		}
		text := row.Children()[1].Children()
		first, second := text[0].Children(), text[1].Children()
		out = append(out, strings.Join([]string{first[0].Text(), second[0].Text(), first[1].Text(), text[2].Text()},
			" | "))
	}
	return out
}

// strong are the words the matches' passages mark.
func (s *shown) strong() string {
	var out []string
	s.node("history-list").Walk(func(n *ui.Node) bool {
		if n.Tag().String() == "strong" {
			out = append(out, n.Text())
		}
		return true
	})
	return strings.Join(out, " ")
}

func (s *shown) tooltip(id string) string {
	s.t.Helper()
	for _, c := range s.node(id).Children() {
		if role, _ := c.Attr("role"); role == "tooltip" {
			return c.Text()
		}
	}
	s.t.Fatalf("#%s has no tooltip", id)
	return ""
}

func TestWithTheModelOrTheFeatureOffTheHistoryIsAsItIsWithoutAModel(t *testing.T) {
	srv := quay(t)
	plain := showWith(t, athome(Options{Downloads: t.TempDir(), Now: clock()}))
	off, _ := modelled(t, athome(Options{Now: clock()}))
	on, _ := modelled(t, athome(Options{Now: clock()}))
	on.configure(func(c *config.Config) { c.Model = true })
	var want []string
	for name, s := range map[string]*shown{"no model": plain, "the model off": off, "the feature off": on} {
		s.memory.settle = 0
		s.visit(srv.URL + "/ferry")
		s.visit(srv.URL + "/knots")
		s.Open(historyPage)
		s.frame()
		s.questions("ferry")
		got := s.ids()
		for _, row := range s.visits() {
			_, titled, _ := strings.Cut(row, " | ")
			got = append(got, titled)
		}
		if want == nil {
			want = got
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("with %s the page is\n%s\nwant\n%s", name, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
		text := strings.ToLower(s.node("page").Text())
		for _, word := range []string{"remember", "meaning", "forget", "said"} {
			if strings.Contains(text, word) {
				t.Errorf("with %s the page speaks of %q", name, word)
			}
		}
		s.node("page").Walk(func(n *ui.Node) bool {
			if n.HasClass("robot-toggle") || n.HasClass("recalled") || strings.HasPrefix(n.ID(), "history-ask") {
				t.Errorf("with %s the page has %s.%v", name, n.ID(), n.Classes())
			}
			return true
		})
		if s.memory.pages.Len() != 0 {
			t.Errorf("with %s %d pages are remembered", name, s.memory.pages.Len())
		}
		s.clean()
	}
	if len(want) != 7 || strings.Join(want[:5], " ") != "page history-tools history-filter history-clear history-list" {
		t.Fatalf("the page is %q", want)
	}
}

func TestAQuestionIsAnsweredByWhatThePagesSaid(t *testing.T) {
	s, _, site := harboured(t)
	if s.doc.ByID("history-ask") != nil || len(s.visits()) != 3 {
		t.Fatalf("before a question the page shows a card or %v", s.visits())
	}
	if s.node("history-unremembered").Text() != unremembered {
		t.Fatalf("the page says %q", s.node("history-unremembered").Text())
	}
	s.clean()
	s.questions(batteryQuestion)
	if got := s.node("history-ask-title").Text(); got != batteryQuestion {
		t.Fatalf("the card is titled %q", got)
	}
	if got := s.node("history-ask-sub").Text(); got != byMeaningSub {
		t.Fatalf("the card says %q", got)
	}
	robot := s.node("history-robot")
	if on, _ := robot.Attr("aria-pressed"); on != "true" || s.tooltip("history-robot") != searchedSaid {
		t.Fatalf("the robot is pressed %q and says %q", on, s.tooltip("history-robot"))
	}
	ticked := false
	robot.Walk(func(n *ui.Node) bool {
		ticked = ticked || src(n) == "tick.svg"
		return true
	})
	if !ticked {
		t.Fatal("the robot has no tick")
	}
	want := "How shore charging works | 127.0.0.1 | Mon 16 Nov | " +
		"An automatic arm tops up the batteries at each berth <b>quickly</b>."
	if got := s.recalledRows(); len(got) != 1 || got[0] != want {
		t.Fatalf("the matches are\n%s\nwant\n%s", strings.Join(got, "\n"), want)
	}
	if got := s.strong(); got != "tops batteries" {
		t.Fatalf("the words marked are %q", got)
	}
	if len(s.visits()) != 0 {
		t.Fatalf("the visits are listed under the matches: %v", s.visits())
	}
	if !s.doc.Focused(s.node("history-filter")) {
		t.Fatal("asking took the focus off the field")
	}
	s.clean()
	s.configure(func(c *config.Config) { c.DarkMode = true })
	s.frame()
	if !s.dark() || len(s.recalledRows()) != 1 {
		t.Fatalf("dark, the matches are %v", s.recalledRows())
	}
	s.clean()
	var link *ui.Node
	s.node("history-list").Walk(func(n *ui.Node) bool {
		if n.HasClass("link") {
			link = n
		}
		return link == nil
	})
	s.clickAt(link.Box().Center())
	s.until("the match opens", func() bool { return s.browser.Tab().Current() == site+"/charging" })
}

func TestWhatAPageSaidIsShownAsPlainText(t *testing.T) {
	s, _, _ := harboured(t)
	s.questions(batteryQuestion)
	s.node("history-list").Walk(func(n *ui.Node) bool {
		if !n.HasClass("said") {
			return true
		}
		for _, c := range n.Children() {
			if !c.IsText() && c.Tag().String() != "strong" {
				t.Errorf("the passage holds a %s", c.Tag())
			}
		}
		return false
	})
	s.questions("a token no reader sees")
	if got := s.recalledRows(); len(got) != 0 {
		t.Fatalf("a page's script is remembered: %v", got)
	}
	if s.node("history-unsaid").Text() != "No page remembered says something like it." {
		t.Fatalf("with no match the page says %q", s.node("history-unsaid").Text())
	}
	s.clean()
}

func TestTheRobotFallsBackToTitlesAndBringsMeaningBack(t *testing.T) {
	s, _, site := harboured(t)
	s.questions("diesel ferry")
	if got := s.recalledRows(); len(got) != 1 || !strings.HasPrefix(got[0], "Council votes") {
		t.Fatalf("by meaning the matches are %v", got)
	}
	s.click("history-robot")
	if got := s.visits(); len(got) != 1 ||
		!strings.HasSuffix(got[0], "Council votes to retire the diesel ferry | "+site+"/ferry") {
		t.Fatalf("by titles the visits are %v", got)
	}
	if len(s.recalledRows()) != 0 || s.doc.ByID("history-when") != nil {
		t.Fatal("the robot off, the page still shows what pages said")
	}
	if on, _ := s.node("history-robot").Attr("aria-pressed"); on != "false" ||
		s.node("history-ask-sub").Text() != "Matched against the titles and addresses of pages you visited." {
		t.Fatalf("the robot is pressed %q, the card says %q", on, s.node("history-ask-sub").Text())
	}
	if s.node("history-ask-title").Text() != "diesel ferry" {
		t.Fatal("the robot off, the question went")
	}
	s.clean()
	s.click("history-robot")
	if got := s.recalledRows(); len(got) != 1 || len(s.visits()) != 0 || s.node("history-ask-sub").Text() != byMeaningSub {
		t.Fatalf("the robot on again, the matches are %v", got)
	}
	s.clean()
}

func TestTypingAgainLeavesTheQuestionForTheFilter(t *testing.T) {
	s, _, _ := harboured(t)
	s.questions(batteryQuestion)
	s.click("history-filter")
	s.press(input.KeyBackspace)
	if s.doc.ByID("history-ask") != nil || len(s.recalledRows()) != 0 {
		t.Fatal("typing left the question's card")
	}
	s.Open(historyPage)
	s.frame()
	s.click("history-filter")
	s.in.AddText('k')
	s.in.AddText('n')
	s.frame()
	s.frame()
	if got := s.visits(); len(got) != 1 || !strings.Contains(got[0], "Knots") {
		t.Fatalf("the filter %q keeps %v", s.doc.Value(s.node("history-filter")), got)
	}
	s.questions(batteryQuestion)
	s.questions("")
	if s.doc.ByID("history-ask") != nil || len(s.visits()) != 3 {
		t.Fatalf("an empty question leaves the card or %v", s.visits())
	}
	s.clean()
}

func TestLastWeekKeepsThePagesVisitedInTheLastSevenDays(t *testing.T) {
	srv := quay(t)
	now := time.Date(2026, time.November, 2, 10, 0, 0, 0, time.UTC)
	s, _ := remembering(t, Options{Now: func() time.Time { return now }})
	s.reads(srv.URL + "/ferry")
	now = now.AddDate(0, 0, 10)
	s.reads(srv.URL + "/charging")
	s.Open(historyPage)
	s.frame()
	s.questions("the council voted to replace the batteries at each berth")
	if got := s.recalledRows(); len(got) != 2 || s.pressed("history-when") != "Any time" {
		t.Fatalf("at any time the matches are %v, the chip %q", got, s.pressed("history-when"))
	}
	s.click("history-when-week")
	got := s.recalledRows()
	if len(got) != 1 || !strings.HasPrefix(got[0], "How shore charging works | 127.0.0.1 | Thu 12 Nov") ||
		s.pressed("history-when") != "Last week" {
		t.Fatalf("in the last week the matches are %v, the chip %q", got, s.pressed("history-when"))
	}
	s.clean()
	s.click("history-when-any")
	if got = s.recalledRows(); len(got) != 2 {
		t.Fatalf("at any time again the matches are %v", got)
	}
}

func TestForgettingWhatPagesSaidIsConfirmedInTheCard(t *testing.T) {
	s, _, _ := harboured(t)
	s.click("history-forget")
	if got := s.node("history-forget-sure").Text(); !strings.Contains(got, "Forget what 3 pages said?") {
		t.Fatalf("the card asks %q", got)
	}
	if s.node("history-ask-title").Text() != "What pages said" || s.doc.ByID("history-robot") != nil {
		t.Fatal("with no question the card shows one")
	}
	s.clean()
	s.click("history-forget-keep")
	if s.doc.ByID("history-ask") != nil || s.memory.pages.Len() != 3 {
		t.Fatalf("kept, the card is still there or %d pages are remembered", s.memory.pages.Len())
	}
	s.questions(batteryQuestion)
	s.click("history-forget")
	if s.node("history-ask-title").Text() != batteryQuestion || len(s.recalledRows()) != 1 {
		t.Fatal("asking to forget took the question away")
	}
	s.clean()
	s.click("history-forget-all")
	if s.memory.pages.Len() != 0 || len(s.recalledRows()) != 0 || s.doc.ByID("history-forget-sure") != nil {
		t.Fatalf("forgotten, %d pages are remembered and %v listed", s.memory.pages.Len(), s.recalledRows())
	}
	if _, off := s.node("history-forget").Attr("disabled"); !off {
		t.Fatal("with nothing remembered the page still offers to forget")
	}
	if len(s.visits()) != 0 || s.doc.ByID("history-unsaid") == nil {
		t.Fatal("forgetting what pages said listed the visits under the question")
	}
	s.questions("")
	if len(s.visits()) != 3 {
		t.Fatalf("forgetting what pages said took visits: %v", s.visits())
	}
	s.clean()
}

func TestAMatchForgetsItsPageOrExcludesItsSite(t *testing.T) {
	s, _, site := harboured(t)
	s.questions("the council voted to replace the batteries at each berth")
	if got := s.recalledRows(); len(got) != 2 {
		t.Fatalf("the matches are %v", got)
	}
	press := func(class string) {
		t.Helper()
		var button *ui.Node
		s.node("history-list").Walk(func(n *ui.Node) bool {
			if button == nil && n.HasClass(class) {
				button = n
			}
			return button == nil
		})
		s.clickAt(button.Box().Center())
		s.frame()
	}
	first := s.recalledRows()[0]
	press("forget")
	if got := s.recalledRows(); len(got) != 1 || got[0] == first || s.memory.pages.Len() != 2 {
		t.Fatalf("one forgotten, the matches are %v of %d pages", got, s.memory.pages.Len())
	}
	if s.Profile.visits.Len() != 3 {
		t.Fatal("forgetting what a page said took its visit")
	}
	s.clean()
	press("exclude")
	if s.config.RecallExcluded != "127.0.0.1" || s.memory.pages.Len() != 0 || len(s.recalledRows()) != 0 {
		t.Fatalf("the site excluded is %q, %d pages are remembered", s.config.RecallExcluded, s.memory.pages.Len())
	}
	s.visit(site + "/ferry")
	s.idles()
	if s.memory.pages.Len() != 0 {
		t.Fatal("a page of a site excluded is remembered")
	}
}

func TestTheFeatureTurnedOffWhileTheHistoryShowsTakesItAllAway(t *testing.T) {
	s, _, _ := harboured(t)
	s.questions("diesel ferry")
	s.setRemembering(false)
	s.frame()
	s.frame()
	for _, id := range []string{"history-ask", "history-remembers", "history-robot", "history-forget"} {
		if s.doc.ByID(id) != nil {
			t.Errorf("the feature off, the page has #%s", id)
		}
	}
	if len(s.visits()) != 1 || len(s.recalledRows()) != 0 {
		t.Fatalf("the feature off, the page lists %v", s.visits())
	}
	s.clean()
	s.setRemembering(true)
	s.frame()
	s.frame()
	if s.doc.ByID("history-remembers") == nil || s.doc.ByID("history-ask") != nil {
		t.Fatal("the feature on again, the page has no line of it, or a card")
	}
	s.questions("diesel ferry")
	if len(s.recalledRows()) != 1 {
		t.Fatalf("the feature on again, the matches are %v", s.recalledRows())
	}
	s.configure(func(c *config.Config) { c.Model = false })
	s.frame()
	s.frame()
	if s.doc.ByID("history-ask") != nil || s.doc.ByID("history-remembers") != nil {
		t.Fatal("the model off, the page still has the card")
	}
	s.clean()
}

func TestAModelThatCannotEmbedSearchesTheTitles(t *testing.T) {
	s, fake, _ := harboured(t)
	fake.SetCapabilities(model.Capabilities{Chat: true})
	s.frame()
	s.frame()
	s.click("history-filter")
	s.types("ferry")
	s.press(input.KeyEnter)
	if s.doc.ByID("history-ask-sub") != nil || s.doc.ByID("history-robot") != nil || len(s.visits()) != 1 {
		t.Fatalf("the page shows a question card or a robot, or lists %v", s.visits())
	}
	s.clean()
}

func TestAnAnswerToAQuestionLeftIsDropped(t *testing.T) {
	s, _, _ := harboured(t)
	release := s.hold()
	s.click("history-filter")
	s.doc.SetValue(s.node("history-filter"), batteryQuestion)
	s.press(input.KeyEnter)
	if got := s.node("history-ask-sub").Text(); got != "Searching what the pages said…" {
		t.Fatalf("while the model embeds the card says %q", got)
	}
	s.clean()
	s.Open(historyPage)
	s.frame()
	close(release)
	for range 20 {
		s.frame()
		time.Sleep(time.Millisecond)
	}
	if s.doc.ByID("history-ask") != nil || len(s.visits()) != 3 {
		t.Fatal("the answer to a question of the page before shows")
	}
}

func TestOnlyTheQuestionsOwnWordsAreMarked(t *testing.T) {
	wanted := terms("Which ARM tops up the batteries?")
	var got []string
	for _, run := range marked("An arm tops up the Batteries, the battery's top.", wanted) {
		text := run.text
		if run.marked {
			text = "[" + text + "]"
		}
		got = append(got, text)
	}
	if strings.Join(got, "") != "An arm [tops] up the [Batteries], the battery's top." {
		t.Fatalf("the passage is marked as %q", strings.Join(got, ""))
	}
	if runs := marked("", wanted); len(runs) != 0 {
		t.Fatalf("an empty passage is %v", runs)
	}
}

func TestALongPassageIsCutRoundTheQuestionsFirstWord(t *testing.T) {
	wanted := terms("batteries")
	long := strings.Repeat("word ", 40) + "the batteries are topped up " + strings.Repeat("après ", 40)
	got := excerpt(long, wanted, 100)
	if !strings.HasPrefix(got, "…word ") || !strings.HasSuffix(got, "après…") || !strings.Contains(got, "batteries") ||
		len(got) > 100+2*len("…") {
		t.Fatalf("the excerpt is %q", got)
	}
	got = excerpt(long, terms("nothing"), 100)
	if !strings.HasPrefix(got, "word word") || !strings.HasSuffix(got, "word…") {
		t.Fatalf("with no word of the question the excerpt is %q", got)
	}
	if got = excerpt("short", wanted, 100); got != "short" {
		t.Fatalf("a short passage is %q", got)
	}
	if got = excerpt(strings.Repeat("é", 300), wanted, 101); got != strings.Repeat("é", 50)+"…" {
		t.Fatalf("a passage of one word is cut as %q", got)
	}
}
