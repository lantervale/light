// light is a small web browser made as a demo of ember: tabs in the
// window's own title bar, a navigation row and an address field, a
// status bubble over the page, a history, a context menu and developer
// tools, all plain ui elements. An http or https address is fetched and
// shown by its type; an HTML page is built as a ui Document of its own.
//
//	go run ./cmd/light [address]
//
// What light does is set at light://config and kept in config.json. The
// environment and the argument only override that, here and nowhere
// else:
//
//   - LIGHT_DIR is the folder the settings, the history and the list of
//     downloads are kept in (light's folder in the user's config folder
//     by default; none in a hidden run, which keeps them in memory);
//   - LIGHT_DOWNLOADS is the folder files are saved to, whatever the
//     settings say (theirs, else ~/Downloads, by default);
//   - the address argument is opened at launch, over the settings' start
//     page.
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/lantervale/ember"
	"github.com/lantervale/ember/color"
	"github.com/lantervale/ember/platform"

	"github.com/lantervale/light/internal/app"
	"github.com/lantervale/light/internal/shell"
	"github.com/lantervale/light/pkg/tabs"
)

func main() {
	win := platform.NewWindow(
		platform.Title("light"),
		platform.Size(1024, 680),
		platform.MinSize(760, 480),
		platform.OwnTitleBar(),
		platform.Background(color.Hex("#2a2530")),
	)
	screen := app.New(win, options())
	if len(os.Args) > 1 {
		screen.Open(tabs.Resolve(os.Args[1]))
	}
	win.Add(screen.Document())
	ember.New().AddEntities(win).AddSystems(screen).MustRun()
	if err := screen.Close(); err != nil {
		log.Print(err)
	}
}

// historyDir is LIGHT_DIR, else light's folder in the user's config
// folder; none for a hidden run not given one, which must leave the
// player's files alone.
func historyDir() string {
	if dir := os.Getenv("LIGHT_DIR"); dir != "" {
		return dir
	}
	config, err := os.UserConfigDir()
	if err != nil || os.Getenv("EMBER_HIDDEN") != "" {
		return ""
	}
	return filepath.Join(config, "light")
}

// options are what the environment gives the screen: where it keeps its
// files, the folder that overrides the settings' for downloads, and the
// system's.
func options() app.Options {
	return app.Options{
		Dir: historyDir(), Downloads: os.Getenv("LIGHT_DOWNLOADS"), DefaultDownloads: systemDownloads(),
		Files: shell.System{},
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
