package commands

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/lantervale/light/cmd/light/globals"
	"github.com/lantervale/light/internal/app"
	"github.com/lantervale/light/pkg/testnet"
)

// Drive is a light a test runner drives over WebDriver. The test
// network and a script's budget are its options alone: they are a test
// runner's, never a person's.
type Drive struct {
	Address string `required:"" env:"LIGHT_WEBDRIVER" placeholder:"HOST:PORT" help:"Where W3C WebDriver is served: an address of this machine only (127.0.0.1:4444). A driven light keeps its settings, history and cookies in memory unless --dir names a folder."`

	HostMap      map[string]string `name:"host-map" placeholder:"NAME=ADDRESS;…" help:"Names, and the names under them, that light finds at an address of this machine (web-platform.test=127.0.0.1), whatever the system resolves them to."`
	TrustCA      string            `name:"trust-ca" placeholder:"FILE" help:"A certificate authority (PEM) this run trusts beside the system's. Every certificate is still checked."`
	ScriptBudget *time.Duration    `name:"script-budget" placeholder:"DURATION" help:"How long one script of a page may run before it is stopped, in place of the two seconds of every page."`

	Open string `arg:"" optional:"" name:"address" help:"The address opened at launch, over the settings' start page."`
}

// Validate refuses a light driven from nowhere and a script's budget of
// no time.
func (d *Drive) Validate() error {
	if d.Address == "" {
		return errors.New("--address is where WebDriver is served: an address of this machine (127.0.0.1:4444)")
	}
	if d.ScriptBudget != nil && *d.ScriptBudget <= 0 {
		return errors.New("--script-budget is how long a script may run: more than no time")
	}
	return nil
}

// Options are what the command line gives a driven light, its requests
// made through the test network when it is given one. It never waits
// for a slow script: one is stopped at its budget. Its pages' text is
// drawn exact, as the pictures a test runner compares need it.
func (d *Drive) Options(build *globals.Build, profile *globals.Profile) (app.Options, error) {
	opts := profile.Options(build, true)
	opts.ExactText = true
	if d.ScriptBudget != nil {
		opts.ScriptBudget = *d.ScriptBudget
	}
	var err error
	opts.Transport, err = d.transport()
	return opts, err
}

// transport makes the requests of a run given a test network, nil for
// any other.
func (d *Drive) transport() (http.RoundTripper, error) {
	if len(d.HostMap) == 0 && d.TrustCA == "" {
		return nil, nil //nolint:nilnil // no transport is light's own
	}
	hosts, err := testnet.ParseHosts(d.HostMap)
	if err != nil {
		return nil, err
	}
	t, err := testnet.Transport(hosts, d.TrustCA)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (d *Drive) Run(ctx context.Context, build *globals.Build, profile *globals.Profile) error {
	opts, err := d.Options(build, profile)
	if err != nil {
		return err
	}
	driven, w := open(opts, d.Open)
	driver, err := app.Drive(ctx, driven, d.Address)
	if err != nil {
		return err
	}
	defer func() { _ = driver.Close() }()
	log.Print("light: WebDriver at http://", driver.Address())
	run(driven, w.AddSystems(driver))
	return nil
}
