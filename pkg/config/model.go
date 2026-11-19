package config

import (
	"path/filepath"

	"github.com/lantervale/light/pkg/model"
)

// MaxModelThreads is the most threads the model may be set to compute
// on.
const MaxModelThreads = 64

// validModel is c with the model's settings within their bounds: a
// model light knows or none, a file by its whole path or none, and the
// model light knows when both are named.
func (c Config) validModel() Config {
	if !model.Known(c.ModelID) {
		c.ModelID = ""
	}
	if c.ModelFile = text(c.ModelFile, ""); c.ModelID != "" || !filepath.IsAbs(c.ModelFile) {
		c.ModelFile = ""
	}
	c.ModelThreads = within(c.ModelThreads, 0, MaxModelThreads, 0)
	return c
}

// ModelSettings are the settings the model follows.
func (c Config) ModelSettings() model.Settings {
	return model.Settings{On: c.Model, ID: c.ModelID, File: c.ModelFile, Threads: c.ModelThreads, GPU: !c.ModelCPU}
}
