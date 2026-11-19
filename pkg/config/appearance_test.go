package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/config"
)

func TestTheAppearanceFollowsTheSystemUntilSetAndIsKept(t *testing.T) {
	if got := config.Default().Appearance; got != config.AppearanceSystem {
		t.Fatalf("light starts with the appearance %q", got)
	}
	for _, kept := range []string{config.AppearanceSystem, config.AppearanceLight, config.AppearanceDark} {
		got, err := config.Decode([]byte(`{"version": 1, "appearance": "` + kept + `"}`))
		if err != nil || got.Appearance != kept {
			t.Errorf("an appearance of %q is read as %+v, %v", kept, got, err)
		}
	}
	for _, bad := range []string{`"Dark"`, `"night"`, `" dark"`, `true`, `1`, `null`, `{}`} {
		got, err := config.Decode([]byte(`{"appearance": ` + bad + `}`))
		if err != nil || got != config.Default() {
			t.Errorf("an appearance of %s read as %+v, %v, want the default", bad, got, err)
		}
	}
	path := filepath.Join(t.TempDir(), "config.json")
	dark := config.Default()
	dark.Appearance = config.AppearanceDark
	if err := config.Save(path, dark); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"appearance": "dark"`) || strings.Contains(string(data), "dark_mode") {
		t.Fatalf("the file holds %s", data)
	}
	if got, err := config.Load(path); err != nil || got != dark {
		t.Fatalf("read back %+v, %v, want %+v", got, err, dark)
	}
}

func TestTheDarkModeAnEarlierLightKeptIsReadAsItsAppearance(t *testing.T) {
	for file, want := range map[string]string{
		`{"version": 1, "dark_mode": true}`:                        config.AppearanceDark,
		`{"version": 1, "dark_mode": false}`:                       config.AppearanceSystem,
		`{"version": 1, "dark_mode": "yes"}`:                       config.AppearanceSystem,
		`{"version": 1, "dark_mode": true, "appearance": "light"}`: config.AppearanceLight,
		`{"version": 1, "dark_mode": true, "appearance": ""}`:      config.AppearanceSystem,
		`{"version": 1, "dark_mode": true, "appearance": "night"}`: config.AppearanceSystem,
	} {
		got, err := config.Decode([]byte(file))
		if err != nil || got.Appearance != want {
			t.Errorf("%s is read as the appearance %q, %v, want %q", file, got.Appearance, err, want)
		}
	}
	former, err := config.Decode([]byte(`{"version": 1, "dark_mode": true, "zoom": 150}`))
	if err != nil {
		t.Fatal(err)
	}
	written, err := former.Encode()
	if err != nil || strings.Contains(string(written), "dark_mode") ||
		!strings.Contains(string(written), `"appearance": "dark"`) {
		t.Fatalf("written again, the file holds %s, %v", written, err)
	}
}

func TestTheAppearanceIsDarkWhenSetSoOrWhenItFollowsADarkSystem(t *testing.T) {
	for _, c := range []struct {
		set    string
		system bool
		want   bool
	}{
		{config.AppearanceSystem, false, false}, {config.AppearanceSystem, true, true},
		{config.AppearanceLight, false, false}, {config.AppearanceLight, true, false},
		{config.AppearanceDark, false, true}, {config.AppearanceDark, true, true},
	} {
		if got := (config.Config{Appearance: c.set}).Dark(c.system); got != c.want {
			t.Errorf("set to %q on a system dark %v, light is dark %v", c.set, c.system, got)
		}
	}
}

func TestTheWheelScrollsAsItComesUntilReversedOrGivenASpeed(t *testing.T) {
	d := config.Default()
	if d.ScrollReversed || d.ScrollSpeed != 100 || d.ScrollFactor() != 1 {
		t.Fatalf("light starts scrolling reversed %v at %d %%, by %v", d.ScrollReversed, d.ScrollSpeed,
			d.ScrollFactor())
	}
	got, err := config.Decode([]byte(`{"version": 1, "scroll_reversed": true, "scroll_speed": 250}`))
	if err != nil || !got.ScrollReversed || got.ScrollSpeed != 250 || got.ScrollFactor() != -2.5 {
		t.Fatalf("read %+v, %v, a factor of %v", got, err, got.ScrollFactor())
	}
	for speed, want := range map[int]int{
		config.MinScrollSpeed: 25, config.MaxScrollSpeed: 400, 24: 100, 401: 100, 0: 100, -100: 100,
	} {
		if kept := (config.Config{ScrollSpeed: speed}).Valid().ScrollSpeed; kept != want {
			t.Errorf("a speed of %d is kept as %d, want %d", speed, kept, want)
		}
	}
	for _, bad := range []string{`"fast"`, `1.5`, `null`, `true`} {
		got, err = config.Decode([]byte(`{"scroll_speed": ` + bad + `, "scroll_reversed": "no"}`))
		if err != nil || got != config.Default() {
			t.Errorf("a speed of %s read as %+v, %v, want the default", bad, got, err)
		}
	}
	if half := (config.Config{ScrollSpeed: 50}).ScrollFactor(); half != 0.5 {
		t.Fatalf("at 50 %% the wheel scrolls by %v", half)
	}
}
