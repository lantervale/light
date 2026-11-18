// light is a small web browser made as a demo of ember: tabs in the
// window's own title bar, a navigation row and an address field, a
// status bubble over the page, a history, a context menu and developer
// tools, all plain ui elements. An http or https address is fetched and
// shown by its type; an HTML page is built as a ui Document of its own.
//
//	go run ./cmd/light [address]
//
// Built with the v8 tag (CGO_CXXFLAGS=-nostdinc++ go run -tags v8
// ./cmd/light), it runs the pages' JavaScript.
//
// What light does is set at light://config and kept in config.json. The
// command line and the environment only override that, here and nowhere
// else: see CLI, or light --help.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"github.com/lantervale/ember"
	"github.com/lantervale/ember/color"
	"github.com/lantervale/ember/platform"

	"github.com/lantervale/light/internal/app"
	"github.com/lantervale/light/internal/shell"
	"github.com/lantervale/light/pkg/identity"
	"github.com/lantervale/light/pkg/tabs"
)

const (
	name        = "light"
	description = "A small web browser on ember."
)

//nolint:gochecknoglobals // these global variables exist to be overridden during build
var (
	version = "dev"
	commit  = "dirty"
	date    = "latest"
)

// CLI is light's command line.
type CLI struct {
	Address string `arg:"" optional:"" help:"The address opened at launch, over the settings' start page."`

	Dir       string `env:"LIGHT_DIR" placeholder:"FOLDER" help:"The folder the settings, the history, the list of downloads and what pages store are kept in (light's folder in the user's config folder by default; none in a hidden run, which keeps them in memory)."`
	Downloads string `env:"LIGHT_DOWNLOADS" placeholder:"FOLDER" help:"The folder files are saved to, whatever the settings say (theirs, else ~/Downloads, by default)."`

	Version kong.VersionFlag `help:"Say which light this is."`
}

func main() {
	var cli CLI
	cmd := parser(&cli)
	_, err := cmd.Parse(os.Args[1:])
	cmd.FatalIfErrorf(err)
	win := platform.NewWindow(
		platform.Title("light"),
		platform.Size(1024, 680),
		platform.MinSize(760, 480),
		platform.OwnTitleBar(),
		platform.Background(color.Hex("#2a2530")),
	)
	screen := app.New(win, cli.options())
	if cli.Address != "" {
		screen.Open(tabs.Resolve(cli.Address))
	}
	win.Add(screen.Document())
	ember.New().AddEntities(win).AddSystems(screen).MustRun()
	if err = screen.Close(); err != nil {
		log.Print(err)
	}
}

// parser reads light's command line into cli.
func parser(cli *CLI, opts ...kong.Option) *kong.Kong {
	return kong.Must(cli, append([]kong.Option{
		kong.Name(name),
		kong.Description(description),
		kong.UsageOnError(),
		kong.Vars{"version": fmt.Sprintf("%s %s (%s, %s)", name, version, commit, date)},
	}, opts...)...)
}

func parse(args []string, opts ...kong.Option) (CLI, error) {
	var cli CLI
	_, err := parser(&cli, opts...).Parse(args)
	return cli, err
}

// dir is the folder given, else light's folder in the user's config
// folder; none for a hidden run not given one, which must leave the
// player's files alone.
func (c CLI) dir() string {
	if c.Dir != "" {
		return c.Dir
	}
	config, err := os.UserConfigDir()
	if err != nil || os.Getenv("EMBER_HIDDEN") != "" {
		return ""
	}
	return filepath.Join(config, "light")
}

// options are what the command line gives the screen: where it keeps
// its files, the folder that overrides the settings' for downloads, and
// the system's.
func (c CLI) options() app.Options {
	return app.Options{
		Dir: c.dir(), Downloads: c.Downloads, DefaultDownloads: systemDownloads(),
		Files: shell.System{}, Version: version,
		Languages: identity.System(), Driven: identity.Driven(os.Environ()),
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
