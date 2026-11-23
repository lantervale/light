package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/platform"

	"github.com/lantervale/light/pkg/config"
)

func TestTheLanguagesAreThoseTheSystemSaysAndFollowThem(t *testing.T) {
	site := telling(t)
	prefs := platform.NewPreferences()
	s := showWith(t, Options{Downloads: t.TempDir(), Languages: []string{"fr-FR", "en-US"}, Preferences: prefs})
	told := func(what, placeholder, accept, scripts string) {
		t.Helper()
		s.settings()
		if got, _ := s.node("set-accept-language").Attr("placeholder"); got != placeholder {
			t.Errorf("%s, the settings show the system's languages as %q, want %q", what, got, placeholder)
		}
		path := "/" + strings.ReplaceAll(what, " ", "-")
		s.visit(site.URL + path)
		if h := site.said("GET " + path); !strings.Contains(h, "Accept-Language: "+accept+"\n") {
			t.Errorf("%s, the site was told:\n%s", what, h)
		}
		if got := strings.Join(s.navigator().Languages, ","); got != scripts {
			t.Errorf("%s, scripts read the languages %q, want %q", what, got, scripts)
		}
		s.settings()
	}
	told("while the system says none", "fr-FR, fr, en-US, en", "fr-FR,fr;q=0.9,en-US;q=0.8,en;q=0.7",
		"fr-FR,fr,en-US,en")
	prefs.SyncLanguages("de-CH", "en")
	s.frame()
	told("once the system says its own", "de-CH, de, en", "de-CH,de;q=0.9,en;q=0.8", "de-CH,de,en")
	if got := s.translatesInto().tag; got != "en-US" {
		t.Errorf("a reader of German then English is translated into %q", got)
	}
	prefs.SyncLanguages("fr-CA", "en")
	s.frame()
	told("once the system changed", "fr-CA, fr, en", "fr-CA,fr;q=0.9,en;q=0.8", "fr-CA,fr,en")
	if got := s.translatesInto().tag; got != "fr" {
		t.Errorf("a reader of French then English is translated into %q", got)
	}
	s.set("set-accept-language", "it")
	told("with the setting", "fr-CA, fr, en", "it", "it")
	prefs.SyncLanguages("es-ES")
	s.frame()
	told("with the setting over a system that changed", "es-ES, es", "it", "it")
	s.set("set-accept-language", "")
	told("with the setting emptied", "es-ES, es", "es-ES,es;q=0.9", "es-ES,es")
	prefs.SyncLanguages()
	s.frame()
	told("once the system says none again", "fr-FR, fr, en-US, en", "fr-FR,fr;q=0.9,en-US;q=0.8,en;q=0.7",
		"fr-FR,fr,en-US,en")
	if s.config != config.Default() {
		t.Fatalf("following the system changed the settings: %+v", s.config)
	}
}

// glide is how many frames a scroll by the wheel or the keys takes to
// land, as Chrome glides them: 12 at most, and one to start.
const glide = 14

// wheeled turns the wheel by lines at a point of the window, and is how
// far up what y measures went once the page glided there.
func (s *shown) wheeled(at geom.Vec2, lines float32, y func() float32) float32 {
	s.t.Helper()
	s.in.SetMouse(at)
	s.frame()
	before := y()
	s.in.AddWheel(geom.V2(0, lines))
	for range glide {
		s.frame()
	}
	return before - y()
}

func near(got, want float32) bool { return got > want-0.01 && got < want+0.01 }

func TestTheWheelOverAPageOfTheWebScrollsAsTheSettingsSay(t *testing.T) {
	srv := crossings(t)
	s := show(t)
	page := s.visit(srv.URL + "/")
	host, _ := s.doc.BoxOf(s.host)
	at, y := host.Center(), func() float32 { return box(t, page, "moved").Y }
	notch := s.wheeled(at, 1, y)
	if notch <= 0 || !near(s.wheeled(at, -1, y), -notch) {
		t.Fatalf("with nothing set a turn of the wheel scrolls the page by %v", notch)
	}
	s.configure(func(c *config.Config) { c.ScrollSpeed = 200 })
	if got := s.wheeled(at, 1, y); !near(got, 2*notch) {
		t.Fatalf("at 200 %% a turn scrolls by %v, twice %v", got, notch)
	}
	s.configure(func(c *config.Config) { c.ScrollReversed = true })
	if got := s.wheeled(at, 1, y); !near(got, -2*notch) {
		t.Fatalf("reversed, the same turn scrolls by %v", got)
	}
	if got := s.wheeled(at, -1, y); !near(got, 2*notch) {
		t.Fatalf("reversed, a turn the other way scrolls by %v", got)
	}
	s.configure(func(c *config.Config) { c.ScrollSpeed, c.ScrollReversed = 50, false })
	if got := s.wheeled(at, 1, y); !near(got, notch/2) {
		t.Fatalf("at 50 %% a turn scrolls by %v, half %v", got, notch)
	}
	s.newWindow(srv.URL + "/")
	s.frame()
	if got := s.window(1).doc.WheelScroll(); got != 0.5 {
		t.Fatalf("a new window scrolls by %v times the wheel", got)
	}
}

func TestTheWheelOverAPageOfLightsOwnScrollsAsTheSettingsSayAndTheyAreKept(t *testing.T) {
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	s.settings()
	head, _ := s.doc.BoxOf(s.node("config-head"))
	at, y := head.Center(), func() float32 { r, _ := s.doc.BoxOf(s.node("set-reset")); return r.Y }
	notch := s.wheeled(at, 1, y)
	if notch <= 0 || !near(s.wheeled(at, -1, y), -notch) {
		t.Fatalf("with nothing set a turn of the wheel scrolls the settings by %v", notch)
	}
	if s.doc.Checked(s.node("set-scroll-reversed")) || s.doc.Value(s.node("set-scroll-speed")) != "100" {
		t.Fatalf("the settings start reversed %v at %q", s.doc.Checked(s.node("set-scroll-reversed")),
			s.doc.Value(s.node("set-scroll-speed")))
	}
	s.set("set-scroll-speed", "300")
	s.Open(configPage)
	s.frame()
	if got := s.wheeled(at, 1, y); s.config.ScrollSpeed != 300 || !near(got, 3*notch) {
		t.Fatalf("set to %d %%, a turn scrolls by %v, three times %v", s.config.ScrollSpeed, got, notch)
	}
	s.wheeled(at, -1, y)
	s.click("set-scroll-reversed")
	s.Open(configPage)
	s.frame()
	if got := s.wheeled(at, -1, y); !s.config.ScrollReversed || !near(got, 3*notch) {
		t.Fatalf("reversed %v, a turn the other way scrolls by %v", s.config.ScrollReversed, got)
	}
	s.set("set-scroll-speed", "9")
	if s.config.ScrollSpeed != config.Default().ScrollSpeed && s.config.ScrollSpeed != config.MinScrollSpeed {
		t.Fatalf("a speed under its bounds is kept as %d", s.config.ScrollSpeed)
	}
	s.set("set-scroll-speed", "300")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	kept, err := config.Load(filepath.Join(dir, configFile))
	if err != nil || !kept.ScrollReversed || kept.ScrollSpeed != 300 {
		t.Fatalf("the file holds %+v, %v", kept, err)
	}
	again := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	if got := again.doc.WheelScroll(); got != -3 {
		t.Fatalf("the next run scrolls by %v times the wheel", got)
	}
	again.settings()
	again.click("set-reset")
	if got := again.doc.WheelScroll(); got != 1 || again.doc.Checked(again.node("set-scroll-reversed")) {
		t.Fatalf("reset, light scrolls by %v times the wheel", got)
	}
	again.clean()
}
