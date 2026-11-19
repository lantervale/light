package commands_test

import (
	"os"
	"testing"

	"github.com/lantervale/light/cmd/light/commands"
	"github.com/lantervale/light/cmd/light/globals"
	"github.com/lantervale/light/pkg/model"
)

func shown(t *testing.T) {
	t.Helper()
	for _, name := range []string{"EMBER_HIDDEN", "EMBER_AUTOMATION", "EMBER_CAPTURE", "EMBER_CAPTURE_FRAME"} {
		t.Setenv(name, "")
	}
}

func TestABrowserIsItsPersonsAndWaitsForASlowScript(t *testing.T) {
	shown(t)
	t.Setenv("LIGHT_WEBDRIVER", "127.0.0.1:4444")
	build, profile := &globals.Build{Version: "1.2.3"}, &globals.Profile{Dir: "/kept"}
	browse := &commands.Browse{}
	got := browse.Options(build, profile)
	if got.Driven || got.ExactText || !got.Patient || got.Dir != "/kept" || got.Version != "1.2.3" ||
		got.Transport != nil || got.ScriptBudget != 0 {
		t.Fatalf("a browser's options are %+v", got)
	}
	nobody := map[string]string{"EMBER_HIDDEN": "1", "EMBER_AUTOMATION": "127.0.0.1:1", "EMBER_CAPTURE": "x.png"}
	for name, value := range nobody {
		t.Setenv(name, value)
		if got = browse.Options(build, profile); got.Patient || !got.Driven || got.ExactText {
			t.Fatalf("a light run with %s waits for a slow script: %v, driven: %v, its pages' text exact: %v", name,
				got.Patient, got.Driven, got.ExactText)
		}
		if !(&commands.Browse{Patient: true}).Options(build, profile).Patient {
			t.Fatalf("--patient does not have a light run with %s wait", name)
		}
		t.Setenv(name, "")
	}
}

func TestABrowserWithAFolderHasAModelThatIsOffAndWritesNothing(t *testing.T) {
	shown(t)
	t.Setenv(model.EnvDev, "")
	dir := t.TempDir()
	build, profile := &globals.Build{Version: "1.2.3"}, &globals.Profile{Dir: dir}
	browse := &commands.Browse{}
	got, err := browse.Modelled(build, profile)
	if err != nil || got.Model == nil || got.Library == nil {
		t.Fatalf("a browser's options have the model %v, %v", got.Model, err)
	}
	t.Cleanup(func() { _ = got.Library.Close() })
	if state := got.Model.Status().State; state != model.Off {
		t.Fatalf("a browser's model starts %v", state)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Fatalf("a model that is off wrote %v", left)
	}
	t.Setenv("EMBER_HIDDEN", "1")
	if hidden, problem := browse.Modelled(build, profile); problem != nil || hidden.Model != nil ||
		hidden.Library != nil {
		t.Fatalf("a hidden light has the model %v, %v", hidden.Model, problem)
	}
	t.Setenv("EMBER_HIDDEN", "")
	t.Setenv(model.EnvDev, "nonsense")
	if _, err = browse.Modelled(&globals.Build{Version: "dev"}, profile); err == nil {
		t.Fatal("a LIGHT_MODEL_DEV that does not read is not refused")
	}
}
