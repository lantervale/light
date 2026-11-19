package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/tabs"
)

// dark reports whether the shown page is drawn dark.
func (s *shown) dark() bool { return s.view.HasClass("dark") }

// appearance is the setting that draws light's own pages dark, or light.
func appearance(dark bool) string {
	if dark {
		return config.AppearanceDark
	}
	return config.AppearanceLight
}

// appears chooses the appearance that has value in the settings' page,
// by the keys.
func (s *shown) appears(value string) {
	s.t.Helper()
	const id = "set-appearance"
	options := s.node(id).Children()
	at := slices.IndexFunc(options, func(o *ui.Node) bool {
		v, _ := o.Attr("value")
		return v == value
	})
	if at < 0 {
		s.t.Fatalf("%s has no choice %q", id, value)
	}
	s.click(id)
	for range options {
		s.press(input.KeyUp)
	}
	for range at {
		s.press(input.KeyDown)
	}
	s.press(input.KeyEnter)
}

// appearing is the appearance the settings' page shows as chosen.
func (s *shown) appearing() string { return s.doc.Value(s.node("set-appearance")) }

// under is a light on a system of that appearance, and the system's
// preferences, which the test changes as the system would.
func under(t *testing.T, system platform.Appearance, opts Options) (*shown, *platform.Preferences) {
	t.Helper()
	prefs := platform.NewPreferences()
	prefs.SyncAppearance(system)
	opts.Preferences = prefs
	if opts.Downloads == "" {
		opts.Downloads = t.TempDir()
	}
	return showWith(t, opts), prefs
}

func TestTheAppearanceSettingShadesEveryPageOfLightsOwn(t *testing.T) {
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	if s.dark() || s.config.Appearance != config.AppearanceSystem {
		t.Fatalf("light starts dark %v, its appearance %q", s.dark(), s.config.Appearance)
	}
	s.settings()
	var names []string
	for _, option := range s.node("set-appearance").Children() {
		names = append(names, option.Text())
	}
	if got := strings.Join(names, ", "); got != "As the system, Light, Dark" || s.appearing() != asSystem {
		t.Fatalf("the choices are %s, the one shown %q", got, s.appearing())
	}
	if s.doc.ByID("set-dark-mode") != nil {
		t.Fatal("the page still has the dark mode's box")
	}
	s.appears(config.AppearanceDark)
	if s.config.Appearance != config.AppearanceDark || !s.dark() || s.appearing() != config.AppearanceDark {
		t.Fatalf("the setting is %q, the page dark %v", s.config.Appearance, s.dark())
	}
	addresses := []string{tabs.Search("tabs"), "ftp://example.test", "light://nowhere",
		tabs.SourceOf(tabs.Page(aboutName))}
	for _, page := range pages() {
		addresses = append(addresses, tabs.Page(page.name))
	}
	for _, address := range addresses {
		s.Open(address)
		s.frame()
		if !s.dark() {
			t.Errorf("%s is not dark", address)
		}
		s.clean()
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if kept, err := config.Load(filepath.Join(dir, configFile)); err != nil ||
		kept.Appearance != config.AppearanceDark {
		t.Fatalf("the file holds %+v, %v", kept, err)
	}
	again := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	if !again.dark() || src(again.node("logo")) != darkLogo {
		t.Fatalf("the next run starts dark %v, its logo %q", again.dark(), src(again.node("logo")))
	}
	again.settings()
	if again.appearing() != config.AppearanceDark {
		t.Fatalf("the field shows %q", again.appearing())
	}
	again.appears(config.AppearanceLight)
	if again.dark() || again.config.Appearance != config.AppearanceLight {
		t.Fatalf("set light, the page is dark %v, the setting %q", again.dark(), again.config.Appearance)
	}
	again.appears(config.AppearanceDark)
	again.click("set-reset")
	if again.dark() || again.appearing() != asSystem ||
		again.config.Appearance != config.AppearanceSystem {
		t.Fatalf("reset left the page dark %v, the choice %q", again.dark(), again.appearing())
	}
	again.clean()
}

func TestTheAppearanceIsTheSettingsElseTheSystems(t *testing.T) {
	for _, c := range []struct {
		system platform.Appearance
		set    string
		dark   bool
	}{
		{platform.AppearanceUnknown, config.AppearanceSystem, false},
		{platform.AppearanceLight, config.AppearanceSystem, false},
		{platform.AppearanceDark, config.AppearanceSystem, true},
		{platform.AppearanceUnknown, config.AppearanceLight, false},
		{platform.AppearanceLight, config.AppearanceLight, false},
		{platform.AppearanceDark, config.AppearanceLight, false},
		{platform.AppearanceUnknown, config.AppearanceDark, true},
		{platform.AppearanceLight, config.AppearanceDark, true},
		{platform.AppearanceDark, config.AppearanceDark, true},
	} {
		dir := t.TempDir()
		kept := config.Default()
		kept.Appearance = c.set
		if err := config.Save(filepath.Join(dir, configFile), kept); err != nil {
			t.Fatal(err)
		}
		s, _ := under(t, c.system, Options{Dir: dir})
		want := logo
		if c.dark {
			want = darkLogo
		}
		if s.dark() != c.dark || s.Screen.dark() != c.dark || src(s.node("logo")) != want {
			t.Errorf("set to %q on a system %v, light starts dark %v with the logo %q", c.set, c.system, s.dark(),
				src(s.node("logo")))
		}
		s.clean()
	}
	none := showWith(t, Options{Downloads: t.TempDir()})
	if none.dark() || none.Screen.dark() {
		t.Fatal("told nothing of the system, light is dark")
	}
}

func TestLightFollowsTheSystemsAppearanceAsItChanges(t *testing.T) {
	s, prefs := under(t, platform.AppearanceLight, Options{})
	s.newWindow(configPage)
	s.frame()
	second := s.window(1)
	s.toggleTools()
	s.frame()
	if s.dark() || second.dark() || !s.node("devtools").HasClass("light") {
		t.Fatal("on a light system light is dark")
	}
	prefs.SyncAppearance(platform.AppearanceDark)
	s.frame()
	if !s.dark() || !second.dark() || src(s.node("logo")) != darkLogo || s.node("devtools").HasClass("light") {
		t.Fatalf("the system went dark: the first window %v, the second %v, the logo %q", s.dark(), second.dark(),
			src(s.node("logo")))
	}
	if second.appearing() != asSystem || s.config != config.Default() {
		t.Fatalf("following the system changed the settings: %+v", s.config)
	}
	s.clean()
	prefs.SyncAppearance(platform.AppearanceUnknown)
	s.frame()
	if s.dark() || src(s.node("logo")) != logo {
		t.Fatal("a system that no longer says is not followed as a light one")
	}
	prefs.SyncAppearance(platform.AppearanceDark)
	s.frame()
	second.appears(config.AppearanceLight)
	if s.dark() || second.dark() {
		t.Fatal("set light on a dark system, light is dark")
	}
	prefs.SyncAppearance(platform.AppearanceLight)
	s.frame()
	prefs.SyncAppearance(platform.AppearanceDark)
	s.frame()
	if s.dark() || second.dark() || src(s.node("logo")) != logo {
		t.Fatal("set light, light followed the system dark")
	}
	second.appears(asSystem)
	if !s.dark() || !second.dark() || s.config.Appearance != config.AppearanceSystem {
		t.Fatalf("back to the system's, which is dark, light is dark %v, set to %q", s.dark(), s.config.Appearance)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestThePageThatRunsScriptsIsShadedWhileItShows(t *testing.T) {
	s, prefs := under(t, platform.AppearanceLight, Options{})
	s.Open(scriptsPage)
	s.frame()
	page := s.session().pages[scriptsPage]
	if page == nil || page.Doc == nil || page.Doc.Body().HasClass("dark") {
		t.Fatalf("on a light system the page that runs scripts is %+v", page)
	}
	prefs.SyncAppearance(platform.AppearanceDark)
	s.frame()
	if !page.Doc.Body().HasClass("dark") {
		t.Fatal("the system went dark and the page that runs scripts stayed light")
	}
	s.configure(func(c *config.Config) { c.Appearance = config.AppearanceLight })
	if page.Doc.Body().HasClass("dark") {
		t.Fatal("set light, the page that runs scripts stayed dark")
	}
}

func TestTheDarkModeOfAnEarlierLightStaysDark(t *testing.T) {
	dir := t.TempDir()
	former := `{"version": 1, "dark_mode": true, "zoom": 150}`
	if err := os.WriteFile(filepath.Join(dir, configFile), []byte(former), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := under(t, platform.AppearanceLight, Options{Dir: dir})
	if !s.dark() || s.config.Appearance != config.AppearanceDark || s.config.Zoom != 150 {
		t.Fatalf("an earlier light's dark mode is read as %+v, dark %v", s.config, s.dark())
	}
	s.settings()
	if s.appearing() != config.AppearanceDark {
		t.Fatalf("the settings show %q", s.appearing())
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, configFile)); string(data) != former {
		t.Fatalf("the file was written again with no setting changed: %s", data)
	}
	again, _ := under(t, platform.AppearanceLight, Options{Dir: dir})
	again.settings()
	again.set("set-timeout", "12")
	if err := again.Close(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, configFile))
	if strings.Contains(string(data), "dark_mode") || !strings.Contains(string(data), `"appearance": "dark"`) {
		t.Fatalf("once a setting changed the file holds %s", data)
	}
}

func TestTheStartPagesLogoSetsTheOtherAppearanceAndItIsKept(t *testing.T) {
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	if src(s.node("logo")) != logo {
		t.Fatalf("the logo is %q", src(s.node("logo")))
	}
	s.click("logo")
	s.at(tabs.Home)
	if s.config.Appearance != config.AppearanceDark || !s.dark() || src(s.node("logo")) != darkLogo {
		t.Fatalf("after a click the setting is %q, the page dark %v, the logo %q", s.config.Appearance, s.dark(),
			src(s.node("logo")))
	}
	if word, ok := s.doc.BoxOf(s.node("wordmark")); !ok || s.node("wordmark").Text() != "light" || word.W == 0 {
		t.Fatalf("the wordmark reads %q at %v", s.node("wordmark").Text(), word)
	}
	s.clean()
	s.Open(configPage)
	s.frame()
	if s.appearing() != config.AppearanceDark {
		t.Fatal("the settings do not show the appearance the logo set")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if kept, err := config.Load(filepath.Join(dir, configFile)); err != nil ||
		kept.Appearance != config.AppearanceDark {
		t.Fatalf("the file holds %+v, %v", kept, err)
	}
	again := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	again.click("logo")
	if again.config.Appearance != config.AppearanceLight || again.dark() || src(again.node("logo")) != logo {
		t.Fatalf("after another click the setting is %q, the page dark %v, the logo %q", again.config.Appearance,
			again.dark(), src(again.node("logo")))
	}
	again.clean()
}

func TestTheLogoSetsTheOtherAppearanceThanTheSystemsShown(t *testing.T) {
	s, prefs := under(t, platform.AppearanceDark, Options{})
	if !s.dark() || src(s.node("logo")) != darkLogo {
		t.Fatal("on a dark system light starts light")
	}
	s.click("logo")
	if s.config.Appearance != config.AppearanceLight || s.dark() || src(s.node("logo")) != logo {
		t.Fatalf("a click on a dark system's logo set %q, the page dark %v", s.config.Appearance, s.dark())
	}
	prefs.SyncAppearance(platform.AppearanceLight)
	s.frame()
	s.click("logo")
	if s.config.Appearance != config.AppearanceDark || !s.dark() {
		t.Fatalf("a click on the light logo set %q, the page dark %v", s.config.Appearance, s.dark())
	}
	s.settings()
	s.appears(asSystem)
	if s.dark() || s.config.Appearance != config.AppearanceSystem {
		t.Fatalf("back to the system's, which is light, light is dark %v", s.dark())
	}
	s.clean()
}

func TestTheDarkLogoIsTheLogoWithItsNavyAndItsWhiteSwapped(t *testing.T) {
	files := icons()
	if string(files[logo].Data) != string(light.Logo) {
		t.Fatal("the logo is not the embedded one")
	}
	dark := string(files[darkLogo].Data)
	for _, part := range []string{`<rect width="64" height="64" rx="14" fill="#fff"/>`,
		`<circle cx="32" cy="32" r="23" fill="#0e2a3f"/>`, `stroke="#f15a2b"`} {
		if !strings.Contains(dark, part) {
			t.Errorf("the dark logo has no %s: %s", part, dark)
		}
	}
	if len(dark) != len(light.Logo) {
		t.Errorf("the dark logo differs by more than its two colours: %s", dark)
	}
}

func TestTheSheetStrikesTheDarkWordmarkWithAThickOrangeBar(t *testing.T) {
	rule := ".dark #wordmark { color: #ffffff; text-decoration: line-through #f15a2b 9px; }"
	if !strings.Contains(sheet, rule) {
		t.Fatalf("the sheet has no %q", rule)
	}
}

func TestAPageOfTheWebIsNotShaded(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.configure(func(c *config.Config) { c.Appearance = config.AppearanceDark })
	if doc := s.visit(srv.URL + "/"); doc == nil || s.dark() {
		t.Fatalf("a page of the web is shown dark: %v", s.dark())
	}
	s.configure(func(c *config.Config) { c.Appearance = config.AppearanceLight })
	s.configure(func(c *config.Config) { c.Appearance = config.AppearanceDark })
	if s.dark() {
		t.Fatal("the setting shaded the page of the web it is changed over")
	}
	s.visit(srv.URL + "/notes.txt")
	if !s.dark() {
		t.Fatal("a text light shows itself is not dark")
	}
	s.click("back")
	s.click("back")
	s.at(tabs.Home)
	if !s.dark() {
		t.Fatal("back on the start page, it is not dark")
	}
	s.clean()
}
