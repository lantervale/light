// light is a small web browser made as a demo of ember: tabs in the
// window's own title bar, a navigation row and an address field, a
// status bubble over the page, a history, a context menu and developer
// tools, all plain ui elements. An http or https address is fetched and
// shown by its type; an HTML page is built as a ui Document of its own.
//
//	go run ./cmd/light [address]
//	go run ./cmd/light drive --address 127.0.0.1:4444 [address]
//
// Built with the v8 tag (CGO_CXXFLAGS=-nostdinc++ go run -tags v8
// ./cmd/light), it runs the pages' JavaScript.
//
// What light does is set at light://config and kept in config.json. The
// command line and the environment only override that, here and nowhere
// else: see CLI, or light --help. CLI is all this file holds: the
// options the commands share are in globals, and the commands in
// commands: browse, a person's browser and the command of a light given
// none, and drive, a light a test runner drives over WebDriver.
package main

import (
	"context"
	"os"

	"github.com/alecthomas/kong"

	"github.com/lantervale/light/cmd/light/commands"
	"github.com/lantervale/light/cmd/light/globals"
)

const (
	name        = "light"
	description = "A small web browser on ember. With no command it browses: light [<address>]."
)

//nolint:gochecknoglobals // these global variables exist to be overridden during build
var (
	version = "dev"
	commit  = "dirty"
	date    = "latest"
)

// CLI is light's command line.
type CLI struct {
	*globals.Profile

	Browse *commands.Browse `cmd:"" default:"withargs" help:"Browse the web, from the address given or the settings' start page: what light does when no command is named."`
	Drive  *commands.Drive  `cmd:"" help:"Serve W3C WebDriver, for a test runner to drive light: the test network and a script's budget are options of this command alone."`

	Version kong.VersionFlag `help:"Say which light this is."`
}

func main() {
	cli := newCLI()
	parser := newParser(cli)
	ctx, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)
	ctx.BindTo(context.Background(), (*context.Context)(nil))
	ctx.FatalIfErrorf(ctx.Run(cli.Profile, build()))
}

func newCLI() *CLI {
	return &CLI{
		Profile: &globals.Profile{},
		Browse:  &commands.Browse{}, Drive: &commands.Drive{},
	}
}

// newParser reads light's command line into cli.
func newParser(cli *CLI, opts ...kong.Option) *kong.Kong {
	return kong.Must(cli, append([]kong.Option{
		kong.Name(name),
		kong.Description(description),
		kong.UsageOnError(),
		kong.Vars{"version": build().String()},
	}, opts...)...)
}

func build() *globals.Build {
	return &globals.Build{Name: name, Version: version, Commit: commit, Date: date}
}
