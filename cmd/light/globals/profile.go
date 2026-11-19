package globals

import (
	"os"
	"path/filepath"

	"github.com/lantervale/light/internal/app"
	"github.com/lantervale/light/internal/shell"
	"github.com/lantervale/light/pkg/identity"
)

// Profile is where a light keeps its files.
type Profile struct {
	Dir       string `env:"LIGHT_DIR" placeholder:"FOLDER" help:"The folder the settings, the history, the list of downloads and what pages store are kept in (light's folder in the user's config folder by default; none in a hidden run and in one a test runner drives, which keep them in memory)."`
	Downloads string `env:"LIGHT_DOWNLOADS" placeholder:"FOLDER" help:"The folder files are saved to, whatever the settings say (theirs, else ~/Downloads, by default)."`
}

// Folder is the folder given, else light's folder in the user's config
// folder; none for a hidden run and for one a test runner drives not
// given one, which must leave the player's files alone.
func (p *Profile) Folder(driven bool) string {
	if p.Dir != "" {
		return p.Dir
	}
	config, err := os.UserConfigDir()
	if err != nil || os.Getenv("EMBER_HIDDEN") != "" || driven {
		return ""
	}
	return filepath.Join(config, "light")
}

// Options are what the command line gives the screen: where it keeps
// its files, the folder that overrides the settings' for downloads, and
// the system's. driven says a test runner drives the run over
// WebDriver.
func (p *Profile) Options(build *Build, driven bool) app.Options {
	return app.Options{
		Dir: p.Folder(driven), Downloads: p.Downloads, DefaultDownloads: systemDownloads(),
		Files: shell.System{}, Version: build.Version,
		Languages: identity.System(), Driven: driven || identity.Driven(os.Environ()),
	}
}

// systemDownloads is the user's Downloads folder.
func systemDownloads() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Downloads")
}
