package commands

import (
	"github.com/lantervale/light/cmd/light/globals"
	"github.com/lantervale/light/internal/app"
)

// Browse is a person's browser, the command of a light given none.
type Browse struct {
	Address string `arg:"" optional:"" help:"The address opened at launch, over the settings' start page."`
	// Patient is for the captures and the tests of a hidden run: a
	// person's light is patient without it.
	Patient bool `hidden:"" help:"Let a script that runs long go on, and ask whether to stop it, in a run nobody sits at too: a hidden one, a capture, one ember's automation drives, which stop it at its budget otherwise."`
}

// Options are what the command line gives a person's browser: a slow
// script of a page goes on and its user is asked, unless nobody sits
// at the window (a hidden run, a capture, ember's automation), where it
// is stopped at its budget.
func (b *Browse) Options(build *globals.Build, profile *globals.Profile) app.Options {
	opts := profile.Options(build, false)
	opts.Patient = b.Patient || !opts.Driven
	return opts
}

func (b *Browse) Run(build *globals.Build, profile *globals.Profile) error {
	run(open(b.Options(build, profile), b.Address))
	return nil
}
