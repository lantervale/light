package globals

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lantervale/light/pkg/model"
)

// Released reports whether this light is one a release shipped, and not
// one built from the sources or a snapshot of them.
func (b Build) Released() bool {
	return b.Version != "" && b.Version != "dev" && !strings.Contains(b.Version, "SNAPSHOT")
}

// Model is what the command line and the environment give the model
// that runs on this device: its folder in the profile's (none for a
// light that keeps nothing on disk, which has no model), and the helper
// that runs it, light-model, found beside light. A light that is not a
// release also takes a helper (LIGHT_MODEL_HELPER) and a llama.cpp and
// a model (LIGHT_MODEL_DEV) of a developer's own; an error is a
// LIGHT_MODEL_DEV that does not read.
func (p *Profile) Model(build *Build, driven bool) (model.Options, error) {
	opts := model.Options{Build: build.Version, Release: build.Released()}
	if folder := p.Folder(driven); folder != "" {
		opts.Dir = filepath.Join(folder, "model")
	}
	if exe, err := os.Executable(); err == nil {
		opts.Helper = model.FindHelper(exe)
	}
	if opts.Release {
		return opts, nil
	}
	if helper := os.Getenv(model.EnvHelper); helper != "" {
		opts.Helper = helper
	}
	dev, err := model.ParseDev(os.Getenv(model.EnvDev))
	opts.Dev = dev
	return opts, err
}
