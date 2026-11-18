package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/tabs"
)

func TestTheDefaultsAreWhatLightDidBeforeItHadSettings(t *testing.T) {
	c := config.Default()
	want := config.Config{
		StartPage: tabs.Home, KeepHistory: true, HistoryLimit: 2000, Zoom: 100, Pictures: true,
		Timeout: 30, Redirects: 10, MetaRefresh: true, Requests: 1000,
		JavaScript: true, Frames: true, Cookies: true,
	}
	if c != want {
		t.Fatalf("the defaults are %+v, want %+v", c, want)
	}
	if c.Valid() != c {
		t.Fatalf("the defaults are not valid: %+v", c.Valid())
	}
}

func TestAValueOutOfItsBoundsFallsBackToItsDefault(t *testing.T) {
	c := config.Default()
	c.StartPage, c.UserAgent = "  ", "light\nX-Injected: 1"
	c.HistoryLimit, c.Zoom, c.Timeout, c.Redirects, c.Requests = 3, 500, 0, -1, 1<<30
	c.ToolsHeight = 40
	if got := c.Valid(); got != config.Default() {
		t.Fatalf("the bad values became %+v", got)
	}
	c = config.Default()
	c.StartPage, c.Downloads, c.UserAgent = " https://example.com ", " /tmp/files ", " Mozilla/5.0 "
	c.HistoryLimit, c.Zoom, c.Timeout, c.Redirects, c.Requests = 100, 200, 300, 0, 100
	c.ToolsHeight = 120
	want := c
	want.StartPage, want.Downloads, want.UserAgent = "https://example.com", "/tmp/files", "Mozilla/5.0"
	if got := c.Valid(); got != want {
		t.Fatalf("values at their bounds became %+v, want %+v", got, want)
	}
}

func TestTheUserAgentLightOnceWroteIsTheDefaultAndOneThatWasSetIsKept(t *testing.T) {
	former, err := config.Decode([]byte(`{"version": 1, "user_agent": "light/1.0 (ember)", "zoom": 150}`))
	if err != nil || former.UserAgent != "" || former.Zoom != 150 {
		t.Fatalf("the file of an earlier light is read as %+v (%v)", former, err)
	}
	set, err := config.Decode([]byte(
		`{"version": 1, "user_agent": "Probe/2.0", "accept_language": "fr_ch, fr;q=0.9,, en"}`))
	if err != nil || set.UserAgent != "Probe/2.0" || set.Languages != "fr-CH, fr, en" {
		t.Fatalf("what was set is read as %+v (%v)", set, err)
	}
	if got := (config.Config{Languages: "no language, here!"}).Valid().Languages; got != "no, language" {
		t.Fatalf("words are read as the tags %q", got)
	}
	if got := (config.Config{Languages: "fr\nX-Injected: 1"}).Valid().Languages; got != "" {
		t.Fatalf("a list with a line break is kept as %q", got)
	}
}

func TestWhatIsWrittenIsReadBack(t *testing.T) {
	c := config.Default()
	c.StartPage, c.Downloads, c.KeepHistory, c.Zoom, c.MetaRefresh, c.PreserveLog = "https://example.com", "/d", false,
		150, false, true
	c.JavaScript = false
	data, err := c.Encode()
	if err != nil || !strings.Contains(string(data), `"version": 1`) {
		t.Fatalf("%s, %v", data, err)
	}
	got, err := config.Decode(data)
	if err != nil || got != c {
		t.Fatalf("read back %+v, %v, want %+v", got, err, c)
	}
}

func TestReadingIsTolerant(t *testing.T) {
	got, err := config.Decode([]byte(`{"version": 7, "zoom": 125, "theme": "dark", "timeout": "soon",
		"keep_history": false, "redirects": 99, "pictures": null, "javascript": "yes"}`))
	want := config.Default()
	want.Zoom, want.KeepHistory = 125, false
	if err != nil || got != want {
		t.Fatalf("read %+v, %v, want %+v: an unknown key ignored, a bad value its default", got, err, want)
	}
	for _, corrupt := range []string{"", "{", `[1, 2]`, `"zoom"`} {
		got, err = config.Decode([]byte(corrupt))
		if err == nil || got != config.Default() {
			t.Errorf("%q read as %+v, %v, want the defaults and why", corrupt, got, err)
		}
	}
}

func TestTheFileIsTheDefaultsUntilWrittenAndACorruptOneSaysSo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "light", "config.json")
	got, err := config.Load(path)
	if err != nil || got != config.Default() {
		t.Fatalf("no file read as %+v, %v", got, err)
	}
	got.Zoom = 150
	if err = config.Save(path, got); err != nil {
		t.Fatal(err)
	}
	again, err := config.Load(path)
	if err != nil || again.Zoom != 150 {
		t.Fatalf("read back %+v, %v", again, err)
	}
	if err = os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err = config.Load(path); err == nil || got != config.Default() {
		t.Fatalf("a corrupt file read as %+v, %v", got, err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{not json" {
		t.Fatalf("reading rewrote the corrupt file: %q", data)
	}
}

func TestTheDarkModeIsOffUntilSetAndKept(t *testing.T) {
	if config.Default().DarkMode {
		t.Fatal("light starts dark")
	}
	got, err := config.Decode([]byte(`{"version": 1, "dark_mode": true}`))
	if err != nil || !got.DarkMode {
		t.Fatalf("read %+v, %v, want the dark mode on", got, err)
	}
	for _, bad := range []string{`"dark"`, `1`, `null`, `{}`} {
		got, err = config.Decode([]byte(`{"dark_mode": ` + bad + `}`))
		if err != nil || got != config.Default() {
			t.Errorf("a dark mode of %s read as %+v, %v, want the default", bad, got, err)
		}
	}
	path := filepath.Join(t.TempDir(), "config.json")
	dark := config.Default()
	dark.DarkMode = true
	if err = config.Save(path, dark); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), `"dark_mode": true`) {
		t.Fatalf("the file holds %s", data)
	}
	if got, err = config.Load(path); err != nil || got != dark {
		t.Fatalf("read back %+v, %v, want %+v", got, err, dark)
	}
}

func TestCookiesAreKeptUntilSetNotToAndThirdPartiesOnesNot(t *testing.T) {
	if d := config.Default(); !d.Cookies || d.ThirdPartyCookies {
		t.Fatalf("light starts with cookies %v, a third party's %v", d.Cookies, d.ThirdPartyCookies)
	}
	got, err := config.Decode([]byte(`{"version": 1, "cookies": false, "third_party_cookies": true}`))
	if err != nil || got.Cookies || !got.ThirdPartyCookies {
		t.Fatalf("read %+v, %v, want cookies off and a third party's on", got, err)
	}
	for _, bad := range []string{`"yes"`, `1`, `null`} {
		got, err = config.Decode([]byte(`{"cookies": ` + bad + `, "third_party_cookies": ` + bad + `}`))
		if err != nil || got != config.Default() {
			t.Errorf("cookies of %s read as %+v, %v, want the default", bad, got, err)
		}
	}
	path := filepath.Join(t.TempDir(), "config.json")
	set := config.Default()
	set.Cookies, set.ThirdPartyCookies = false, true
	if err = config.Save(path, set); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"cookies": false`) ||
		!strings.Contains(string(data), `"third_party_cookies": true`) {
		t.Fatalf("the file holds %s", data)
	}
	if got, err = config.Load(path); err != nil || got != set {
		t.Fatalf("read back %+v, %v, want %+v", got, err, set)
	}
}

func TestPagesAreKeptFromTheLocalNetworkUntilSetNotToBe(t *testing.T) {
	if config.Default().LocalNetwork {
		t.Fatal("light starts with pages let into the local network")
	}
	got, err := config.Decode([]byte(`{"version": 1, "local_network": true}`))
	if err != nil || !got.LocalNetwork {
		t.Fatalf("read %+v, %v, want pages let into the local network", got, err)
	}
	if got, err = config.Decode([]byte(`{"version": 1}`)); err != nil || got.LocalNetwork {
		t.Fatalf("read %+v, %v from a file that says nothing of it", got, err)
	}
}
