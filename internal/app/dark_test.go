package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lantervale/light"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/tabs"
)

// dark reports whether the shown page is drawn dark.
func (s *shown) dark() bool { return s.view.HasClass("dark") }

func TestTheDarkModeSettingShadesEveryPageOfLightsOwn(t *testing.T) {
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	if s.dark() || s.config.DarkMode {
		t.Fatal("light starts dark")
	}
	s.settings()
	s.click("set-dark-mode")
	if !s.config.DarkMode || !s.dark() || !s.doc.Checked(s.node("set-dark-mode")) {
		t.Fatalf("the setting is %v, the page dark %v", s.config.DarkMode, s.dark())
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
	if kept, err := config.Load(filepath.Join(dir, configFile)); err != nil || !kept.DarkMode {
		t.Fatalf("the file holds %+v, %v", kept, err)
	}
	again := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	if !again.dark() || src(again.node("logo")) != darkLogo {
		t.Fatalf("the next run starts dark %v, its logo %q", again.dark(), src(again.node("logo")))
	}
	again.settings()
	if !again.doc.Checked(again.node("set-dark-mode")) {
		t.Fatal("the field is not checked")
	}
	again.click("set-reset")
	if again.dark() || again.doc.Checked(again.node("set-dark-mode")) {
		t.Fatal("reset left the page dark")
	}
	again.clean()
}

func TestTheStartPagesLogoSwitchesTheDarkModeAndItIsKept(t *testing.T) {
	dir := t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	if src(s.node("logo")) != logo {
		t.Fatalf("the logo is %q", src(s.node("logo")))
	}
	s.click("logo")
	s.at(tabs.Home)
	if !s.config.DarkMode || !s.dark() || src(s.node("logo")) != darkLogo {
		t.Fatalf("after a click the setting is %v, the page dark %v, the logo %q", s.config.DarkMode, s.dark(),
			src(s.node("logo")))
	}
	if word, ok := s.doc.BoxOf(s.node("wordmark")); !ok || s.node("wordmark").Text() != "light" || word.W == 0 {
		t.Fatalf("the wordmark reads %q at %v", s.node("wordmark").Text(), word)
	}
	s.clean()
	s.Open(configPage)
	s.frame()
	if !s.doc.Checked(s.node("set-dark-mode")) {
		t.Fatal("the settings do not show the dark mode the logo set")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if kept, err := config.Load(filepath.Join(dir, configFile)); err != nil || !kept.DarkMode {
		t.Fatalf("the file holds %+v, %v", kept, err)
	}
	again := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	again.click("logo")
	if again.config.DarkMode || again.dark() || src(again.node("logo")) != logo {
		t.Fatalf("after another click the setting is %v, the page dark %v, the logo %q", again.config.DarkMode,
			again.dark(), src(again.node("logo")))
	}
	again.clean()
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
	s.configure(func(c *config.Config) { c.DarkMode = true })
	if doc := s.visit(srv.URL + "/"); doc == nil || s.dark() {
		t.Fatalf("a page of the web is shown dark: %v", s.dark())
	}
	s.configure(func(c *config.Config) { c.DarkMode = false })
	s.configure(func(c *config.Config) { c.DarkMode = true })
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
