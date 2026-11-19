package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/history"
	"github.com/lantervale/light/pkg/model"
	"github.com/lantervale/light/pkg/tabs"
)

const lettersSummary = "Readers write about the ferry."

// summing is a light whose model is on and sums pages up as it is
// stepped, showing the gazette's article.
func summing(t *testing.T, opts Options) (*shown, *model.Fake, string) {
	t.Helper()
	srv := gazette(t)
	s, fake := helped(t, opts)
	fake.Manual = true
	fake.Reply = func(req model.Request) (string, error) {
		if strings.Contains(req.Messages[0].Text, "Letters") {
			return lettersSummary, nil
		}
		return gazetteSummary, nil
	}
	s.visit(srv.URL + "/")
	return s, fake, srv.URL + "/"
}

func sums(fake *model.Fake) []model.Request { return askedTo(fake, "You sum up web pages") }

// summedUp gives the model's answers whole and has them kept.
func (s *shown) summedUp(fake *model.Fake) {
	s.t.Helper()
	s.frame()
	if len(s.digests) == 0 {
		s.t.Fatal("the model is asked to sum up nothing")
	}
	s.until("the summary is kept", func() bool {
		fake.Finish()
		return len(s.digests) == 0
	})
	s.frame()
}

func (s *shown) visitOf(address string) history.Visit {
	s.t.Helper()
	v, ok := s.Profile.visits.Last(address)
	if !ok {
		s.t.Fatalf("%s was never visited", address)
	}
	return v
}

func TestAPageLeftIsSummedUpInTheBackground(t *testing.T) {
	s, fake, _ := summing(t, Options{Now: morning})
	for range 4 {
		s.frame()
	}
	if len(sums(fake)) != 0 {
		t.Fatalf("the page still shown, the model was asked %d times", len(sums(fake)))
	}
	s.Open(tabs.Home)
	s.until("the model is asked", func() bool { return len(sums(fake)) == 1 })
	asked := sums(fake)[0]
	if asked.Priority != model.Background || asked.Grammar != "" || asked.MaxTokens != summaryTokens ||
		!strings.Contains(asked.System, "never") || !strings.Contains(asked.System, "data") ||
		strings.Contains(asked.System, "ferry") {
		t.Fatalf("the model is told %q, at %v, for %d tokens", asked.System, asked.Priority, asked.MaxTokens)
	}
	lead, data, _ := strings.Cut(asked.Messages[0].Text, opens)
	if strings.Contains(lead, "ferry") || !strings.Contains(data, gazetteTitle+"\n") ||
		!strings.Contains(data, "the town council voted on Tuesday") || strings.Count(data, ends) != 1 ||
		strings.Contains(data, "not for the model") {
		t.Fatalf("the model is asked %q", asked.Messages[0].Text)
	}
	if got := s.texts("continue", "card"); len(got) != 1 || strings.Contains(got[0], gazetteSummary) {
		t.Fatalf("before the model answers the cards are %q", got)
	}
	if there, ticked := s.robot("continue-robot"); !there || !ticked {
		t.Fatalf("by the heading the robot is there %v, ticked %v", there, ticked)
	}
}

func TestTheCardOfAPageSummedUpSaysWhatItSaysUnderOneRobot(t *testing.T) {
	s, fake, address := summing(t, Options{Now: morning})
	s.Open(tabs.Home)
	s.summedUp(fake)
	if got := s.visitOf(address).Summary; got != gazetteSummary {
		t.Fatalf("the visit says %q", got)
	}
	want := "1 | " + gazetteTitle + " | " + siteOf(address) + " · just now | " + gazetteSummary
	if got := s.texts("continue", "card"); len(got) != 1 || got[0] != want {
		t.Fatalf("the cards are %q, want %q", got, want)
	}
	card := s.node("continue").Children()[1].Children()[0]
	block, tile := card.Children()[1].Box(), card.Children()[0].Children()[0].Box()
	if !card.Children()[1].HasClass("summary") || block.Y < tile.Y+tile.H || block.X != tile.X ||
		block.X+block.W > card.Box().X+card.Box().W-14 || block.Y+block.H > card.Box().Y+card.Box().H {
		t.Fatalf("the summary at %v is not under the tile %v in the card %v", block, tile, card.Box())
	}
	robot, head := s.box("continue-robot"), s.node("continue").Children()[0].Box()
	if robot.X+robot.W != head.X+head.W || robot.Y < head.Y || robot.Y+robot.H > head.Y+head.H {
		t.Fatalf("the robot at %v is not at the end of the heading %v", robot, head)
	}
	tip := s.node("continue-robot").Children()[2]
	if tip.Text() != "One-line summaries, written on this device when a page is closed." {
		t.Fatalf("the robot says %q", tip.Text())
	}
	s.in.SetMouse(robot.Center())
	s.frame()
	s.frame()
	if at, shown := s.doc.BoxOf(tip); !shown || at.X+at.W > robot.X {
		t.Fatalf("under the pointer the robot's sentence shows %v at %v", shown, at)
	}
	s.clean()
	s.in.SetMouse(geom.V2(2, 2))
	s.visit(address)
	s.Open(tabs.Home)
	for range 4 {
		s.frame()
	}
	if len(sums(fake)) != 1 {
		t.Fatalf("a page summed up already had the model asked %d times", len(sums(fake)))
	}
}

func TestTheRobotOfTheStartPageHidesTheSummariesAndItIsKept(t *testing.T) {
	dir := t.TempDir()
	s, fake, address := summing(t, Options{Now: morning, Dir: dir})
	s.Open(tabs.Home)
	s.summedUp(fake)
	s.click("continue-robot")
	if got := s.texts("continue", "card"); len(got) != 1 || strings.Contains(got[0], gazetteSummary) ||
		len(s.texts("continue", "summary")) != 0 {
		t.Fatalf("with the robot off the cards are %q", got)
	}
	if there, ticked := s.robot("continue-robot"); !there || ticked || !s.config.HideSummaries {
		t.Fatalf("with the robot off it is there %v, ticked %v, the setting %v", there, ticked, s.config.HideSummaries)
	}
	if got := s.visitOf(address).Summary; got != gazetteSummary {
		t.Fatalf("hidden, the visit says %q", got)
	}
	s.clean()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, configFile)); err != nil ||
		!strings.Contains(string(data), `"hide_summaries": true`) {
		t.Fatalf("the settings' file holds %s, %v", data, err)
	}
	again, _ := helped(t, Options{Now: morning, Dir: dir})
	if there, ticked := again.robot("continue-robot"); !there || ticked || len(again.texts("continue", "summary")) != 0 {
		t.Fatalf("started again the robot is there %v, ticked %v", there, ticked)
	}
	again.click("continue-robot")
	if got := again.texts("continue", "summary"); len(got) != 1 || got[0] != gazetteSummary || again.config.HideSummaries {
		t.Fatalf("with the robot on again the summaries are %q", got)
	}
	if _, ticked := again.robot("continue-robot"); !ticked {
		t.Fatal("the robot is not ticked again")
	}
}

func TestAClosedTabAndATabLeftForAnotherAreSummedUpOnce(t *testing.T) {
	s, fake, address := summing(t, Options{Now: morning})
	letters := strings.TrimSuffix(address, "/") + "/letters"
	s.OpenInNewTab(letters)
	s.until("the letters show", func() bool { return s.session().pages[letters] != nil && s.readable() != nil })
	s.summedUp(fake)
	if got := s.visitOf(address).Summary; got != gazetteSummary || len(sums(fake)) != 1 {
		t.Fatalf("the tab left says %q, the model asked %d times", got, len(sums(fake)))
	}
	s.closeTab(1)
	s.summedUp(fake)
	if got := s.visitOf(letters).Summary; got != lettersSummary || len(sums(fake)) != 2 {
		t.Fatalf("the tab closed says %q, the model asked %d times", got, len(sums(fake)))
	}
	s.selectTab(0)
	s.newTab()
	for range 4 {
		s.frame()
	}
	if len(sums(fake)) != 2 {
		t.Fatalf("pages summed up already had the model asked %d times", len(sums(fake)))
	}
}

func TestASummaryIsOneLineOfPlainTextAndNoAnswerIsNone(t *testing.T) {
	s, fake, address := summing(t, Options{Now: morning})
	fake.Reply = func(model.Request) (string, error) {
		return " <b>The ferry</b>\nis electric. " + strings.Repeat("And more. ", 60), nil
	}
	s.Open(tabs.Home)
	s.summedUp(fake)
	got := s.visitOf(address).Summary
	if !strings.HasPrefix(got, "<b>The ferry</b> is electric. And more.") || len(got) > summaryLength+3 ||
		!strings.HasSuffix(got, "…") {
		t.Fatalf("the visit says %q", got)
	}
	if shown := s.texts("continue", "summary"); len(shown) != 1 || shown[0] != got {
		t.Fatalf("the card says %q", shown)
	}
	s.clean()
	fake.Reply = func(model.Request) (string, error) { return "  \n", nil }
	letters := address + "letters"
	s.visit(letters)
	s.Open(tabs.Home)
	s.summedUp(fake)
	if none := s.visitOf(letters).Summary; none != "" || len(s.texts("continue", "summary")) != 1 {
		t.Fatalf("an empty answer made the visit say %q", none)
	}
}

func TestWithoutTheModelNothingIsSummedUpAndTheStartPageIsAsItWas(t *testing.T) {
	srv := gazette(t)
	visits := func(s *shown) {
		s.visit(srv.URL + "/")
		s.Profile.visits.Summarise(srv.URL+"/", "A summary written before.")
		s.visit(srv.URL + "/letters")
		s.Open(tabs.Home)
		for range 6 {
			s.frame()
		}
	}
	plain := showWith(t, Options{Downloads: t.TempDir(), Now: morning, Transport: &fenced{}})
	visits(plain)
	for name, prepare := range map[string]func(*shown){
		"the model off":   func(s *shown) { s.configure(func(c *config.Config) { c.Model = false }) },
		"the feature off": func(s *shown) { s.assists.Set(summariesFeature, false) },
	} {
		s, fake := helped(t, Options{Now: morning})
		prepare(s)
		s.frame()
		visits(s)
		there, _ := s.robot("continue-robot")
		if shown := s.texts("continue", "summary"); there || len(shown) != 0 || len(sums(fake)) != 0 {
			t.Fatalf("%s: the robot is there %v, the summaries %q, the model asked %d times", name, there, shown,
				len(sums(fake)))
		}
		cards, was := s.node("continue").Children()[1].Children(), plain.node("continue").Children()[1].Children()
		if got, want := cards[1].Box(), was[1].Box(); got != want || s.box("continue") != plain.box("continue") ||
			len(cards[1].Children()) != len(was[1].Children()) {
			t.Errorf("%s: the second card is at %v in %v, without a model at %v in %v", name, got, s.box("continue"), want,
				plain.box("continue"))
		}
		s.clean()
	}
	s, fake := helped(t, Options{})
	s.configure(func(c *config.Config) { c.KeepHistory = false })
	s.visit(srv.URL + "/")
	s.Open(configPage)
	s.Open(tabs.Home)
	for range 6 {
		s.frame()
	}
	if len(sums(fake)) != 0 || s.doc.ByID("continue") != nil {
		t.Fatalf("with no history kept the model was asked %d times", len(sums(fake)))
	}
}

func TestTheSummariesShowInTheDarkAndInANarrowWindow(t *testing.T) {
	s, fake, address := summing(t, Options{Now: morning})
	s.Open(tabs.Home)
	s.summedUp(fake)
	s.Profile.visits.Add("https://marine-notes.test/charging", "How shore charging works", morning())
	s.Profile.visits.Summarise("https://marine-notes.test/charging", "An automatic arm tops up the batteries at "+
		"each berth during the turnaround, which takes a little under ten minutes on a calm day and longer on others.")
	s.Profile.visits.Add("https://harbourferry.test/tickets", "Ticket conditions", morning())
	s.rewrite()
	s.frame()
	s.frame()
	for _, dark := range []bool{false, true} {
		for _, width := range []float32{1024, 760} {
			s.configure(func(c *config.Config) { c.DarkMode = dark })
			s.win.SetSize(geom.V2(width, 680))
			s.frame()
			s.frame()
			s.frame()
			if got := s.texts("continue", "summary"); len(got) != 2 || s.dark() != dark {
				t.Fatalf("dark %v, %v wide: the summaries are %q", dark, width, got)
			}
			robot := s.node("continue-robot").Children()
			_, pale := s.doc.BoxOf(robot[0])
			if _, ink := s.doc.BoxOf(robot[len(robot)-1]); ink == dark || pale != dark {
				t.Fatalf("dark %v: the robot is drawn pale %v, in ink %v", dark, pale, ink)
			}
			cards := s.node("continue").Children()[1].Children()
			if first, second := cards[0].Box(), cards[1].Box(); second.X != first.X || second.Y < first.Y+first.H {
				t.Fatalf("dark %v, %v wide: the cards are at %v and %v", dark, width, first, second)
			}
			for _, card := range cards {
				for _, part := range card.Children() {
					if box, in := part.Box(), card.Box(); box.X < in.X || box.X+box.W > in.X+in.W || box.Y+box.H > in.Y+in.H {
						t.Errorf("dark %v, %v wide: %v leaves its card %v", dark, width, box, in)
					}
				}
			}
			s.clean()
		}
	}
	if got := s.visitOf(address).Summary; got != gazetteSummary {
		t.Fatalf("the visit says %q", got)
	}
}
