package globals_test

import (
	"path/filepath"
	"testing"

	"github.com/lantervale/light/cmd/light/globals"
	"github.com/lantervale/light/pkg/model"
)

func TestTheModelIsKeptInTheProfileAndALightThatKeepsNothingHasNone(t *testing.T) {
	t.Setenv("EMBER_HIDDEN", "")
	t.Setenv(model.EnvHelper, "")
	t.Setenv(model.EnvDev, "")
	build := &globals.Build{Version: "dev"}
	opts, err := (&globals.Profile{Dir: "/kept"}).Model(build, false)
	if err != nil || opts.Dir != filepath.Join("/kept", "model") || opts.Release || opts.Build != "dev" {
		t.Fatalf("the model's options are %+v (%v)", opts, err)
	}
	if opts, err = (&globals.Profile{}).Model(build, true); err != nil || opts.Dir != "" {
		t.Fatalf("a driven light keeps its model in %q (%v)", opts.Dir, err)
	}
	t.Setenv("EMBER_HIDDEN", "1")
	if opts, err = (&globals.Profile{}).Model(build, false); err != nil || opts.Dir != "" {
		t.Fatalf("a hidden light keeps its model in %q (%v)", opts.Dir, err)
	}
}

func TestADevelopersHelperAndFilesAreTakenButNotByARelease(t *testing.T) {
	t.Setenv(model.EnvHelper, "/built/light-model")
	t.Setenv(model.EnvDev, "library=/opt/lib;model=/m/q.gguf")
	profile := &globals.Profile{Dir: "/kept"}
	for _, version := range []string{"dev", "0.2.1-SNAPSHOT-abc1234"} {
		opts, err := profile.Model(&globals.Build{Version: version}, false)
		if err != nil || opts.Helper != "/built/light-model" || opts.Dev.Library != "/opt/lib" ||
			opts.Dev.Model != "/m/q.gguf" {
			t.Fatalf("light %s takes %+v (%v)", version, opts, err)
		}
	}
	opts, err := profile.Model(&globals.Build{Version: "0.2.0"}, false)
	if err != nil || opts.Helper == "/built/light-model" || opts.Dev != (model.Dev{}) || !opts.Release {
		t.Fatalf("a released light takes %+v (%v)", opts, err)
	}
	t.Setenv(model.EnvDev, "nonsense")
	if _, err = profile.Model(&globals.Build{Version: "dev"}, false); err == nil {
		t.Fatal("a LIGHT_MODEL_DEV that does not read is taken")
	}
	if _, err = profile.Model(&globals.Build{Version: "0.2.0"}, false); err != nil {
		t.Fatalf("a released light reads LIGHT_MODEL_DEV: %v", err)
	}
}
