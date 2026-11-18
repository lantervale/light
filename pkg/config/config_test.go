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
		Timeout: 30, Redirects: 10, UserAgent: "light/1.0 (ember)", MetaRefresh: true, Requests: 1000,
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
	if got := c.Valid(); got != config.Default() {
		t.Fatalf("the bad values became %+v", got)
	}
	c = config.Default()
	c.StartPage, c.Downloads, c.UserAgent = " https://example.com ", " /tmp/files ", " Mozilla/5.0 "
	c.HistoryLimit, c.Zoom, c.Timeout, c.Redirects, c.Requests = 100, 200, 300, 0, 100
	want := c
	want.StartPage, want.Downloads, want.UserAgent = "https://example.com", "/tmp/files", "Mozilla/5.0"
	if got := c.Valid(); got != want {
		t.Fatalf("values at their bounds became %+v, want %+v", got, want)
	}
}

func TestWhatIsWrittenIsReadBack(t *testing.T) {
	c := config.Default()
	c.StartPage, c.Downloads, c.KeepHistory, c.Zoom, c.MetaRefresh, c.PreserveLog = "https://example.com", "/d", false,
		150, false, true
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
		"keep_history": false, "redirects": 99, "pictures": null}`))
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
