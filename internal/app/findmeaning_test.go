package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"math"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/model"
)

// vectored are the texts a model was asked to embed.
type vectored struct {
	sync.Mutex

	texts []string
}

func (e *vectored) all() []string {
	e.Lock()
	defer e.Unlock()
	return slices.Clone(e.texts)
}

// refunds has the model embed a text by what it says of money given
// back: the words asked, and the passages that say it, close to one
// another, and the rest away from them, a refund further still.
func refunds(fake *model.Fake) *vectored {
	seen := &vectored{}
	fake.Vector = func(text string) []float32 {
		seen.Lock()
		seen.texts = append(seen.texts, text)
		seen.Unlock()
		lower := strings.ToLower(text)
		for i, said := range []string{"money back", "full fare", "price of the ticket", "credited"} {
			if strings.Contains(lower, said) {
				angle := float64(i) * 0.2
				return []float32{float32(math.Cos(angle)), float32(math.Sin(angle)), 0}
			}
		}
		if strings.Contains(lower, "refund") {
			return []float32{0, -1, 0}
		}
		return []float32{0, 0, 1}
	}
	return seen
}

// meaning runs frames until the model said what means the words.
func (s *shown) meaning() {
	s.t.Helper()
	s.until("the passages are found", func() bool {
		m := &s.find.meaning
		return m.sought && !m.busy && m.wait == 0
	})
	s.frame()
}

// passages are the chips of the passages, a ✓ before the one the find
// stands at.
func (s *shown) passages() string {
	s.t.Helper()
	var out []string
	for _, chip := range s.node("find-passages").Children() {
		label := chip.Text()
		if on, _ := chip.Attr("aria-pressed"); on == "true" {
			label = "✓" + label
		}
		out = append(out, label)
	}
	return strings.Join(out, " | ")
}

// nothingOfMeaning fails when the find shows something of the model.
func (s *shown) nothingOfMeaning() {
	s.t.Helper()
	for _, id := range []string{"find-robot", "find-quote", "find-passages", "find-passage"} {
		if s.doc.ByID(id) != nil {
			s.t.Errorf("the find shows %s", id)
		}
	}
	if got := s.stands(); got != "Not on this page" {
		s.t.Errorf("the find says %q", got)
	}
	if all, _ := s.findMarks(); len(all) != 0 {
		s.t.Errorf("the find marks %v", all)
	}
}

func TestWordsThePageHasNotAreFoundByWhatThePageMeans(t *testing.T) {
	srv := crossings(t)
	s, fake := keptOn(t)
	seen := refunds(fake)
	page := s.visit(srv.URL + "/")
	s.finds("money back")
	if got := s.stands(); got != "Not in these words · looking for what means them" || len(seen.all()) != 0 {
		t.Fatalf("while the words are typed the find says %q, the model embedded %v", got, seen.all())
	}
	s.meaning()
	if got := s.stands(); got != "Not in these words · 3 passages mean it" {
		t.Fatalf("the find says %q", got)
	}
	const fare = "When a sailing is cancelled, we return the full fare to the card you paid with."
	if got := s.node("find-quote").Text(); got != "“"+fare+"”" {
		t.Fatalf("the passage is quoted as %q", got)
	}
	if got := s.passages(); got != "✓Cancelled sailings | If you cannot travel | Season passes" {
		t.Fatalf("the passages are %q", got)
	}
	if got := s.node("find-passage").Text(); got != "Passage 1 of 3" {
		t.Fatalf("the card counts %q", got)
	}
	robot := s.node("find-robot")
	if said, _ := robot.Attr("aria-label"); said != "These words are not on the page. 3 passages mean it." ||
		!strings.Contains(robot.Text(), said) || len(robot.Children()) != 3 {
		t.Fatalf("the robot says %q, its tip %q", said, robot.Text())
	}
	all, current := s.findMarks()
	if len(all) < 3 || len(current) == 0 || s.under(page, current[0]) != "cancelled" {
		t.Fatalf("the passages are marked at %v, the current one at %v", all, current)
	}
	if !page.Selection().Empty() || s.pickedOpen() {
		t.Fatalf("the passages were selected in the page, the selection's card open %v", s.pickedOpen())
	}
	s.clean()

	s.press(input.KeyEnter)
	if got := s.passages(); got != "Cancelled sailings | ✓If you cannot travel | Season passes" {
		t.Fatalf("after Enter the passages are %q", got)
	}
	if got := s.node("find-quote").Text(); got != "“Tell us before departure and you get the price of the ticket "+
		"back, less a fee.”" {
		t.Fatalf("the second passage is quoted as %q", got)
	}
	s.click("find-passage-2")
	_, current = s.findMarks()
	if got := s.passages(); got != "Cancelled sailings | If you cannot travel | ✓Season passes" ||
		len(current) == 0 || s.under(page, current[0]) != "season" || s.node("find-passage").Text() != "Passage 3 of 3" {
		t.Fatalf("after a click on the third chip the passages are %q, the current mark %v", got, current)
	}
	s.click("find-next")
	if got := s.passages(); !strings.HasPrefix(got, "✓") {
		t.Fatalf("Next from the last passage: %q", got)
	}
	s.clean()
}

func TestAPagesPassagesAreEmbeddedOnceAndDroppedWithThePage(t *testing.T) {
	const fare = "When a sailing is cancelled, we return the full fare to the card you paid with."
	srv := crossings(t)
	s, fake := keptOn(t)
	seen := refunds(fake)
	s.visit(srv.URL + "/")
	s.finds("money back")
	s.meaning()
	texts := seen.all()
	if n := len(texts); n < 6 || texts[n-1] != "money back" || !slices.Contains(texts, fare) ||
		slices.Contains(texts, "Cancelled sailings") || slices.Contains(texts, "A ticket nobody sees.") {
		t.Fatalf("the model embedded %q", texts)
	}
	s.refinds("my money back")
	s.meaning()
	if more := seen.all(); len(more) != len(texts)+1 || more[len(more)-1] != "my money back" {
		t.Fatalf("other words had the model embed %q", more[len(texts):])
	}
	if got := s.stands(); got != "Not in these words · 3 passages mean it" {
		t.Fatalf("for other words the find says %q", got)
	}

	s.refinds("fare")
	if got := s.stands(); got != "1 of 1" || s.doc.ByID("find-robot") != nil || s.doc.ByID("find-quote") != nil {
		t.Fatalf("words the page has are found as they are: %q", got)
	}
	s.visit(srv.URL + "/terms")
	if m := &s.find.meaning; s.findShown() || m.page != nil || m.vectors != nil || m.passages != nil {
		t.Fatalf("the page gone, the find is open %v and holds %d vectors", s.findShown(), len(m.vectors))
	}
}

func TestTheRobotOfTheFindGoesBackToTheExactWordsAndBringsThePassagesBack(t *testing.T) {
	srv := crossings(t)
	s, fake := keptOn(t)
	seen := refunds(fake)
	s.visit(srv.URL + "/")
	s.finds("money back")
	s.meaning()
	before := len(seen.all())
	s.click("find-robot")
	robot := s.node("find-robot")
	all, _ := s.findMarks()
	if on, _ := robot.Attr("aria-pressed"); on != "false" || len(robot.Children()) != 2 || len(all) != 0 ||
		s.stands() != "Not in these words on this page" || s.doc.ByID("find-quote") != nil ||
		s.doc.ByID("find-passages") != nil || !s.disabled("find-next") {
		t.Fatalf("after a click the robot is on %q, the find says %q, the marks %v", on, s.stands(), all)
	}
	s.clean()
	s.refinds("a refund")
	for range meaningPause + 5 {
		s.frame()
	}
	if got := len(seen.all()); got != before || s.stands() != "Not in these words on this page" {
		t.Fatalf("with its robot off the find had the model embed %d texts more and says %q", got-before, s.stands())
	}
	s.click("find-robot")
	s.meaning()
	if got := s.stands(); got != "Not in these words on this page" || s.doc.ByID("find-robot") != nil {
		t.Fatalf("words nothing means leave %q", got)
	}
	s.refinds("money back")
	s.meaning()
	if got := s.stands(); got != "Not in these words · 3 passages mean it" || s.doc.ByID("find-quote") == nil {
		t.Fatalf("the robot on again, the find says %q", got)
	}
	s.clean()
}

func TestWithoutAModelTheFindIsByTheExactWordsAndShowsNothingOfOne(t *testing.T) {
	srv := crossings(t)
	plain := kept(t)
	plain.visit(srv.URL + "/")
	plain.finds("money back")
	for range meaningPause + 5 {
		plain.frame()
	}
	plain.nothingOfMeaning()
	plain.clean()

	off, fake := modelled(t, Options{Transport: &fenced{}})
	seen := refunds(fake)
	off.visit(srv.URL + "/")
	off.finds("money back")
	for range meaningPause + 5 {
		off.frame()
	}
	off.nothingOfMeaning()
	if len(seen.all()) != 0 {
		t.Fatalf("a model that is off embedded %q", seen.all())
	}
	off.clean()

	unhelped, other := keptOn(t)
	seen = refunds(other)
	unhelped.assists.Set(findFeature, false)
	unhelped.visit(srv.URL + "/")
	unhelped.finds("money back")
	for range meaningPause + 5 {
		unhelped.frame()
	}
	unhelped.nothingOfMeaning()
	if len(seen.all()) != 0 {
		t.Fatalf("a model that does not help the find embedded %q", seen.all())
	}

	bare, last := keptOn(t)
	last.SetCapabilities(model.Capabilities{Chat: true})
	bare.visit(srv.URL + "/")
	bare.finds("money back")
	for range meaningPause + 5 {
		bare.frame()
	}
	bare.nothingOfMeaning()
}

func TestTheModelsCardListsTheFindAmongWhatItHelpsWith(t *testing.T) {
	s, _ := keptOn(t)
	s.openCard()
	s.frame()
	chip := s.node("model-helps-" + findFeature)
	if on, _ := chip.Attr("aria-pressed"); chip.Text() != "Find by meaning" || on != "true" {
		t.Fatalf("the card's chip reads %q, on %q", chip.Text(), on)
	}
	s.click("model-helps-" + findFeature)
	if s.helps(findFeature) {
		t.Fatal("a click on its chip left the find helped")
	}
}

func TestAModelThatFailsLeavesTheFindByTheExactWords(t *testing.T) {
	srv := crossings(t)
	s, fake := keptOn(t)
	s.visit(srv.URL + "/")
	s.finds("money back")
	fake.SetState(model.Failed, "it broke")
	s.meaning()
	if got := s.stands(); got != "Not in these words on this page" || s.doc.ByID("find-robot") != nil {
		t.Fatalf("with a model that fails the find says %q", got)
	}
	s.clean()
}

func TestTheFindByMeaningInTheDark(t *testing.T) {
	srv := crossings(t)
	s, fake := keptOn(t)
	refunds(fake)
	s.configure(func(c *config.Config) { c.DarkMode = true })
	s.visit(srv.URL + "/")
	s.finds("money back")
	s.meaning()
	if s.doc.ByID("find-quote") == nil {
		t.Fatal("no passage is quoted")
	}
	s.clean()
}

func TestAPagesPassagesAreItsSentencesUnderTheirHeadings(t *testing.T) {
	srv, _ := answers(t, map[string]answer{"/": typed("text/html", `<title>Long</title>
		<p>Before any heading, a sentence. Too short.</p>
		<h2>One  <em>heading</em></h2>
		<p id="long">`+strings.Repeat("word ", 150)+`ends here. What comes after it?</p>
		<script>var hidden = "a script is not a passage";</script>`)})
	s := kept(t)
	page := s.visit(srv.URL + "/")
	var got []string
	for _, p := range passagesOf(page) {
		if len(p.text) > passageSize {
			t.Errorf("a passage of %d bytes", len(p.text))
		}
		if p.from.Node == nil || p.to.Node == nil {
			t.Errorf("the passage %q is nowhere in the page", p.text)
		}
		text := p.text
		if len(text) > 30 {
			text = text[:14] + "…" + text[len(text)-14:]
		}
		got = append(got, p.heading+": "+text)
	}
	want := []string{": Before any hea…g, a sentence.", "One heading: word word word…word word word",
		"One heading: word word word…word word word", "One heading: word word word…ord ends here.",
		"One heading: What comes after it?"}
	if !slices.Equal(got, want) {
		t.Fatalf("the passages are %q, want %q", got, want)
	}
	var first *ui.Node
	page.ByID("long").Walk(func(n *ui.Node) bool {
		if n.IsText() && first == nil {
			first = n
		}
		return true
	})
	if p := passagesOf(page)[1]; p.from.Node != first || p.from.Offset != 0 {
		t.Fatalf("the long sentence starts at %v", p.from)
	}
}

func TestTheClosestPassagesAreThoseOverTheThresholdTheClosestFirst(t *testing.T) {
	to := []float32{1, 0}
	at := func(cos float64) []float32 { return []float32{float32(cos), float32(math.Sqrt(1 - cos*cos))} }
	vectors := [][]float32{at(0.2), at(0.7), at(meaningThreshold - 0.01), at(0.9), {1}, at(0.6), at(0.8), at(0.95),
		at(0.55)}
	if got := closest(to, vectors); !slices.Equal(got, []int{7, 3, 6, 1, 5}) {
		t.Fatalf("the closest are %v", got)
	}
	if got := closest(to, nil); len(got) != 0 {
		t.Fatalf("of no passage the closest are %v", got)
	}
}
