package globals_test

import (
	"path/filepath"
	"testing"

	"github.com/lantervale/light/cmd/light/globals"
)

func shown(t *testing.T) {
	t.Helper()
	for _, name := range []string{"EMBER_HIDDEN", "EMBER_AUTOMATION", "EMBER_CAPTURE", "EMBER_CAPTURE_FRAME"} {
		t.Setenv(name, "")
	}
}

func TestTheFolderIsTheOneGivenAndNoneForAHiddenOrADrivenRun(t *testing.T) {
	shown(t)
	given, none := &globals.Profile{Dir: "/somewhere"}, &globals.Profile{}
	if got := none.Folder(false); filepath.Base(got) != "light" || !filepath.IsAbs(got) {
		t.Fatalf("a shown run keeps its files in %q", got)
	}
	if got := none.Folder(true); got != "" {
		t.Fatalf("a driven run keeps its files in %q", got)
	}
	if got := given.Folder(true); got != "/somewhere" {
		t.Fatalf("a driven run given a folder keeps its files in %q", got)
	}
	t.Setenv("EMBER_HIDDEN", "1")
	if got := none.Folder(false); got != "" {
		t.Fatalf("a hidden run keeps its files in %q", got)
	}
	if got := given.Folder(false); got != "/somewhere" {
		t.Fatalf("a hidden run given a folder keeps its files in %q", got)
	}
}

func TestTheOptionsOnlyOverrideWhatTheSettingsSay(t *testing.T) {
	shown(t)
	home, kept, elsewhere := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	usual := filepath.Join(home, "Downloads")
	build := &globals.Build{Name: "light", Version: "1.2.3"}
	got := (&globals.Profile{Dir: kept, Downloads: elsewhere}).Options(build, false)
	if got.Dir != kept || got.Downloads != elsewhere || got.DefaultDownloads != usual || got.Files == nil ||
		got.Version != "1.2.3" || got.Driven || got.Transport != nil || got.ScriptBudget != 0 {
		t.Fatalf("the options are %+v", got)
	}
	if got = (&globals.Profile{}).Options(build, false); got.Downloads != "" || got.DefaultDownloads != usual {
		t.Fatalf("with no folder given, the settings' is overridden by %q, the system's is %q", got.Downloads,
			got.DefaultDownloads)
	}
}

func TestARunIsDrivenByATestRunnerOrByWhatItsEnvironmentSays(t *testing.T) {
	shown(t)
	build, profile := &globals.Build{}, &globals.Profile{}
	if got := profile.Options(build, false); got.Driven || !got.Preview {
		t.Fatalf("light with somebody at its window is driven %v and warns them %v", got.Driven, got.Preview)
	}
	if got := profile.Options(build, true); !got.Driven || got.Dir != "" || got.Preview {
		t.Fatalf("a light a test runner drives is driven %v and keeps its files in %q", got.Driven, got.Dir)
	}
	for name, value := range map[string]string{"EMBER_HIDDEN": "1", "EMBER_AUTOMATION": "127.0.0.1:1",
		"EMBER_CAPTURE": "x.png"} {
		t.Setenv(name, value)
		if got := profile.Options(build, false); !got.Driven || got.Preview {
			t.Fatalf("a light run with %s is not driven, or warns nobody", name)
		}
		t.Setenv(name, "")
	}
}

func TestABuildSaysWhatWasBuilt(t *testing.T) {
	build := globals.Build{Name: "light", Version: "1.2.3", Commit: "abc", Date: "today"}
	if got := build.String(); got != "light 1.2.3 (abc, today)" {
		t.Fatalf("the build is %q", got)
	}
}
