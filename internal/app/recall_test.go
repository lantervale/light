package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"context"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/model"
	"github.com/lantervale/light/pkg/recall"
)

// quay is a site of three short articles, one with a script and
// with markup written as text.
func quay(t *testing.T) *httptest.Server {
	t.Helper()
	srv, _ := answers(t, map[string]answer{
		"/ferry": typed("text/html", `<title>Council votes to retire the diesel ferry</title><body>
			<p>The council voted to replace the diesel ferry with a battery-electric vessel.</p>
			<script>var hidden = "a token no reader sees";</script></body>`),
		"/charging": typed("text/html", `<title>How shore charging works</title><body>
			<p>An automatic arm tops up the batteries at each berth &lt;b&gt;quickly&lt;/b&gt;.</p></body>`),
		"/knots":     typed("text/html", `<title>Knots</title><body><p>A bowline does not slip under load.</p></body>`),
		"/notes.txt": typed("text/plain", "The council voted to replace the diesel ferry."),
	})
	return srv
}

// remembering is a light whose model is on and remembers what pages
// say as soon as they are shown.
func remembering(t *testing.T, opts Options) (*shown, *model.Fake) {
	t.Helper()
	if opts.Now == nil {
		opts.Now = clock()
	}
	s, fake := modelled(t, athome(opts))
	s.configure(func(c *config.Config) { c.Model = true })
	s.memory.settle = 0
	s.setRemembering(true)
	s.frame()
	s.frame()
	return s, fake
}

// athome keeps the requests of a light on this machine.
func athome(opts Options) Options {
	if opts.Transport == nil {
		opts.Transport = &fenced{}
	}
	return opts
}

// homeOn is a light whose model is on and whose requests stay on this
// machine.
func homeOn(t *testing.T) (*shown, *model.Fake) {
	t.Helper()
	s, fake := modelled(t, athome(Options{}))
	s.configure(func(c *config.Config) { c.Model = true })
	s.frame()
	s.frame()
	return s, fake
}

// withheld is a model that embeds only once it is released.
type withheld struct {
	model.Model
	release chan struct{}
}

func (h *withheld) Embed(ctx context.Context, texts model.Texts) ([][]float32, error) {
	<-h.release
	return h.Model.Embed(ctx, texts)
}

// hold has the light's model wait before it embeds, until the channel
// it gives is closed.
func (s *shown) hold() chan struct{} {
	release := make(chan struct{})
	s.opts.Model = &withheld{s.opts.Model, release}
	return release
}

// reads visits a page and waits until what it says is remembered.
func (s *shown) reads(address string) {
	s.t.Helper()
	s.visit(address)
	s.until("remembered what "+address+" says", func() bool { return s.memory.pages.Has(address) })
}

// idles runs frames for long enough that a page shown would have been
// remembered.
func (s *shown) idles() {
	for range 12 {
		s.frame()
		time.Sleep(2 * time.Millisecond)
	}
}

// saidOf is the passage remembered of a page that the vector of a text
// is the closest to, "" for a page that is not remembered.
func (s *shown) saidOf(fake *model.Fake, address, text string) string {
	s.t.Helper()
	vectors, err := fake.Embed(s.t.Context(), model.Texts{Texts: []string{text}})
	if err != nil {
		s.t.Fatal(err)
	}
	for _, m := range s.memory.pages.Search(vectors[0], recall.Query{Threshold: -1, Most: 100}) {
		if m.Address == address {
			return m.Passage
		}
	}
	return ""
}

func TestNothingIsRememberedUntilTheFeatureIsTurnedOn(t *testing.T) {
	srv := quay(t)
	s, fake := homeOn(t)
	s.memory.settle = 0
	if s.assists.On(recallFeature) || s.remembers() {
		t.Fatal("with the model on, light remembers before it was asked to")
	}
	s.visit(srv.URL + "/ferry")
	s.idles()
	if s.memory.pages.Len() != 0 {
		t.Fatalf("with the feature off %d pages are remembered", s.memory.pages.Len())
	}
	s.click("robot")
	if got := s.pressed("model-helps"); strings.Contains(got, recallLabel) {
		t.Fatalf("the chips pressed are %q", got)
	}
	s.click("model-helps-" + recallFeature)
	if !s.remembers() || !strings.Contains(s.pressed("model-helps"), recallLabel) {
		t.Fatal("the chip did not turn remembering on")
	}
	s.until("the shown page is remembered", func() bool { return s.memory.pages.Has(srv.URL + "/ferry") })
	said := s.saidOf(fake, srv.URL+"/ferry", "diesel ferry")
	if said != "The council voted to replace the diesel ferry with a battery-electric vessel." {
		t.Fatalf("the page is remembered as saying %q", said)
	}
	s.clean()
}

func TestWhatAPageSaysIsReadOnceItSettled(t *testing.T) {
	srv := quay(t)
	s, _ := remembering(t, Options{})
	s.memory.settle = time.Hour
	s.visit(srv.URL + "/ferry")
	s.idles()
	if s.memory.pages.Len() != 0 {
		t.Fatal("a page just shown is remembered already")
	}
	s.memory.settle = 0
	s.until("the page is remembered", func() bool { return s.memory.pages.Has(srv.URL + "/ferry") })
	before := s.memory.changed
	s.idles()
	s.Screen.settled.read = false
	s.idles()
	if s.memory.changed != before || len(s.memory.reading) != 0 {
		t.Fatal("a page read again without a new visit is embedded again")
	}
}

func TestWhatIsNeverRemembered(t *testing.T) {
	srv := quay(t)
	for name, c := range map[string]struct {
		opts   Options
		set    func(c *config.Config)
		visits string
	}{
		"a page of light's own":    {visits: "light://guide"},
		"a file that is no page":   {visits: srv.URL + "/notes.txt"},
		"a page that is not there": {visits: srv.URL + "/nowhere"},
		"a page of a driven run":   {opts: Options{Driven: true}, visits: srv.URL + "/ferry"},
		"a page of a site excluded": {set: func(c *config.Config) { c.RecallExcluded = "127.0.0.1" },
			visits: srv.URL + "/ferry"},
		"a page the history does not keep": {set: func(c *config.Config) { c.KeepHistory = false },
			visits: srv.URL + "/ferry"},
	} {
		t.Run(name, func(t *testing.T) {
			s, fake := remembering(t, c.opts)
			var embedded atomic.Int32
			fake.Vector = func(string) []float32 {
				embedded.Add(1)
				return []float32{1}
			}
			if c.set != nil {
				s.configure(c.set)
			}
			if s.Open(c.visits); onTheWeb(c.visits) {
				s.visit(c.visits)
			}
			s.idles()
			if s.memory.pages.Len() != 0 || embedded.Load() != 0 {
				t.Fatalf("%d pages are remembered, %d texts were embedded", s.memory.pages.Len(), embedded.Load())
			}
		})
	}
}

func TestAModelThatIsOffOrNotGivenRemembersNothing(t *testing.T) {
	srv := quay(t)
	dir := filepath.Join(t.TempDir(), "profile")
	off, _ := modelled(t, athome(Options{Dir: dir}))
	off.assists.Set(recallFeature, true)
	none := showWith(t, athome(Options{Downloads: t.TempDir()}))
	for name, s := range map[string]*shown{"off": off, "not given": none} {
		s.memory.settle = 0
		s.visit(srv.URL + "/ferry")
		s.idles()
		if s.remembers() || s.memory.pages.Len() != 0 {
			t.Errorf("a model %s remembers %d pages", name, s.memory.pages.Len())
		}
	}
	if err := off.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, recallFile)); !os.IsNotExist(err) {
		t.Fatalf("with the model off the folder holds the file: %v", err)
	}
}

func TestRemovingAVisitOrClearingTheHistoryForgetsWhatItsPageSaid(t *testing.T) {
	srv := quay(t)
	s, _ := remembering(t, Options{})
	for _, path := range []string{"/ferry", "/charging", "/knots"} {
		s.reads(srv.URL + path)
	}
	visit, _ := lastVisit(s.Profile.visits, srv.URL+"/charging")
	s.Profile.visits.Remove(visit)
	s.rewrite()
	s.frame()
	if s.memory.pages.Has(srv.URL+"/charging") || s.memory.pages.Len() != 2 {
		t.Fatalf("a visit removed, %d pages are remembered", s.memory.pages.Len())
	}
	s.configure(func(c *config.Config) { c.RecallExcluded = "127.0.0.1" })
	s.frame()
	if s.memory.pages.Len() != 0 {
		t.Fatalf("their site excluded, %d pages are remembered", s.memory.pages.Len())
	}
	s.configure(func(c *config.Config) { c.RecallExcluded = "" })
	s.reads(srv.URL + "/ferry")
	s.clearHistory()
	s.frame()
	if s.memory.pages.Len() != 0 {
		t.Fatalf("the history cleared, %d pages are remembered", s.memory.pages.Len())
	}
}

func TestAPageForgottenWhileItIsEmbeddedIsNotKept(t *testing.T) {
	srv := quay(t)
	s, _ := remembering(t, Options{})
	release := s.hold()
	s.visit(srv.URL + "/ferry")
	s.until("the page is being embedded", func() bool { return s.memory.reading[srv.URL+"/ferry"] })
	s.forgetSaid()
	close(release)
	s.until("the embedding landed", func() bool { return !s.memory.reading[srv.URL+"/ferry"] })
	if s.memory.pages.Len() != 0 {
		t.Fatal("what was forgotten came back")
	}
}

func TestWhatPagesSaidIsKeptInAFileForItsUserAlone(t *testing.T) {
	srv := quay(t)
	dir := filepath.Join(t.TempDir(), "profile")
	s, _ := remembering(t, Options{Dir: dir})
	s.reads(srv.URL + "/ferry")
	s.reads(srv.URL + "/knots")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, recallFile)
	if runtime.GOOS != "windows" {
		for name, want := range map[string]os.FileMode{dir: 0o700, path: 0o600} {
			if info, err := os.Stat(name); err != nil || info.Mode().Perm() != want {
				t.Fatalf("%s is %v, %v", name, info, err)
			}
		}
	}
	kept, err := recall.Load(path, recall.MaxBytes)
	if err != nil || kept.Len() != 2 || !kept.Has(srv.URL+"/knots") {
		t.Fatalf("the file holds %d pages, %v", kept.Len(), err)
	}
	for _, other := range []string{historyFile, configFile, assistFile} {
		if data, _ := os.ReadFile(filepath.Join(dir, other)); strings.Contains(string(data), "bowline") {
			t.Errorf("%s holds what a page said", other)
		}
	}
	again, _ := modelled(t, athome(Options{Dir: dir, Now: clock()}))
	if again.memory.pages.Len() != 2 || !again.remembers() {
		t.Fatalf("the next run remembers %d pages, the feature on %v", again.memory.pages.Len(), again.remembers())
	}
	again.forgetSaid()
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("everything forgotten, the file is still there: %v", err)
	}
}

func TestAFileOfWhatPagesSaidThatIsBrokenIsRefusedAndRemovedByForgetting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, recallFile)
	if err := os.WriteFile(path, []byte(recall.Magic+"\x01\x00\x00\x00 the bowline"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := remembering(t, Options{Dir: dir})
	if s.memory.problem == nil || s.memory.pages.Len() != 0 || s.unreadable != nil {
		t.Fatalf("the broken file reads as %d pages, %v; the history's problem is %v", s.memory.pages.Len(),
			s.memory.problem, s.unreadable)
	}
	s.Open(historyPage)
	s.frame()
	note := s.node("history-unremembered").Text()
	if !strings.Contains(note, "not in use") || strings.Contains(note, "bowline") {
		t.Fatalf("the page says %q", note)
	}
	if s.doc.ByID("history-problem") != nil {
		t.Fatal("the history says its own file is broken")
	}
	s.click("history-forget")
	s.click("history-forget-all")
	if _, err := os.Stat(path); !os.IsNotExist(err) || s.memory.problem != nil {
		t.Fatalf("forgetting leaves the file: %v, %v", err, s.memory.problem)
	}
	if note = s.node("history-unremembered").Text(); note != unremembered {
		t.Fatalf("the page then says %q", note)
	}
	s.clean()
}

func TestTurningTheFeatureOffInTheCardOffersToForget(t *testing.T) {
	srv := quay(t)
	s, _ := remembering(t, Options{})
	s.reads(srv.URL + "/ferry")
	s.click("robot")
	if s.doc.ByID("model-recall-offer") != nil {
		t.Fatal("forgetting is offered while the feature is on")
	}
	s.click("model-helps-" + recallFeature)
	if s.remembers() {
		t.Fatal("the chip left remembering on")
	}
	if offer := s.node("model-recall-offer").Text(); !strings.Contains(offer, "what 1 page said") {
		t.Fatalf("the card offers %q", offer)
	}
	s.clean()
	s.click("model-recall-keep")
	if s.doc.ByID("model-recall-offer") != nil || s.memory.pages.Len() != 1 || !s.cardShown() {
		t.Fatalf("kept, the offer is still there or %d pages are remembered", s.memory.pages.Len())
	}
	s.click("model-helps-" + recallFeature)
	s.click("model-helps-" + recallFeature)
	s.click("model-recall-forget")
	if s.doc.ByID("model-recall-offer") != nil || s.memory.pages.Len() != 0 {
		t.Fatalf("forgotten, %d pages are remembered", s.memory.pages.Len())
	}
	s.click("model-helps-" + recallFeature)
	s.click("model-helps-" + recallFeature)
	if s.doc.ByID("model-recall-offer") != nil {
		t.Fatal("with nothing remembered forgetting is offered")
	}
	s.clean()
}

func TestTheSettingsSayTheFeatureIsOffUntilSwitchedOnAndForget(t *testing.T) {
	srv := quay(t)
	s, _ := homeOn(t)
	s.memory.settle = 0
	s.settings()
	if s.doc.Checked(s.node("set-recall")) {
		t.Fatal("the setting is on before it was switched on")
	}
	said := s.node("recall-settings").Text()
	for _, want := range []string{recallLabel, "Off until you switch it on", "on this device", unremembered} {
		if !strings.Contains(said, want) {
			t.Errorf("the settings do not say %q", want)
		}
	}
	if strings.Contains(strings.ToLower(said), "private window") {
		t.Error("the settings speak of private windows, which light has not")
	}
	if _, off := s.node("set-recall-clear").Attr("disabled"); !off ||
		s.node("recall-kept").Text() != "Nothing is remembered." {
		t.Fatalf("with nothing remembered the settings say %q", s.node("recall-kept").Text())
	}
	s.clean()
	s.click("set-recall")
	if !s.remembers() {
		t.Fatal("the setting did not turn remembering on")
	}
	s.reads(srv.URL + "/ferry")
	s.reads(srv.URL + "/knots")
	s.settings()
	if got := s.node("recall-kept").Text(); got != "Light keeps what 2 pages said." {
		t.Fatalf("the settings say %q", got)
	}
	s.click("set-recall-excluded")
	s.doc.SetValue(s.node("set-recall-excluded"), "https://www.Bank.example/x, mail.example")
	s.press(input.KeyEnter)
	if s.config.RecallExcluded != "bank.example, mail.example" ||
		s.doc.Value(s.node("set-recall-excluded")) != "bank.example, mail.example" {
		t.Fatalf("the sites excluded are %q", s.config.RecallExcluded)
	}
	s.click("set-recall")
	if s.remembers() || s.doc.Checked(s.node("set-recall")) {
		t.Fatal("the setting did not turn remembering off")
	}
	if offer := s.node("set-recall-offer").Text(); !strings.Contains(offer, "what 2 pages said") {
		t.Fatalf("the settings offer %q", offer)
	}
	s.clean()
	s.click("set-recall-forget")
	if s.memory.pages.Len() != 0 || s.doc.ByID("set-recall-offer") != nil ||
		s.node("recall-kept").Text() != "Nothing is remembered." {
		t.Fatalf("forgotten, %d pages are remembered", s.memory.pages.Len())
	}
	s.configure(func(c *config.Config) { c.Appearance = config.AppearanceDark })
	s.frame()
	s.clean()
	s.configure(func(c *config.Config) { c.Model = false })
	s.frame()
	if s.doc.ByID("set-recall") != nil || strings.Contains(s.node("page").Text(), recallLabel) {
		t.Fatal("with the model off the settings still show the feature")
	}
	s.clean()
}

// ids are the ids of the elements of the shown page that have one.
func (s *shown) ids() []string {
	var out []string
	s.node("page").Walk(func(n *ui.Node) bool {
		if n.ID() != "" {
			out = append(out, n.ID())
		}
		return true
	})
	return out
}
