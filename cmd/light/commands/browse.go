package commands

import (
	"github.com/lantervale/ember/platform"

	"github.com/lantervale/light/cmd/light/globals"
	"github.com/lantervale/light/internal/app"
	"github.com/lantervale/light/pkg/model"
)

// Browse is a person's browser, the command of a light given none.
type Browse struct {
	Address string `arg:"" optional:"" help:"The address opened at launch, over the settings' start page."`
	// Patient is for the captures and the tests of a hidden run: a
	// person's light is patient without it.
	Patient bool `hidden:"" help:"Let a script that runs long go on, and ask whether to stop it, in a run nobody sits at too: a hidden one, a capture, one ember's automation drives, which stop it at its budget otherwise."`
}

// Options are what the command line gives a person's browser: a slow
// script of a page goes on and its user is asked, and light follows
// the appearance and the languages of their system, unless nobody sits
// at the window (a hidden run, a capture, ember's automation), where
// the script is stopped at its budget and the system is not looked at.
func (b *Browse) Options(build *globals.Build, profile *globals.Profile) app.Options {
	opts := profile.Options(build, false)
	opts.Patient = b.Patient || !opts.Driven
	if !opts.Driven {
		opts.Preferences = platform.NewPreferences()
	}
	return opts
}

// Modelled are the browser's options with the model that runs on this
// device, off until its user turns it on: none for a light nobody sits
// at, and none for one that keeps nothing on disk. An error is a model
// of a developer's own that does not read.
func (b *Browse) Modelled(build *globals.Build, profile *globals.Profile) (app.Options, error) {
	opts := b.Options(build, profile)
	if opts.Driven {
		return opts, nil
	}
	given, err := profile.Model(build, false)
	if err != nil || given.Dir == "" {
		return opts, err
	}
	local := model.Open(given, model.Settings{})
	opts.Model, opts.Library = local, local
	return opts, nil
}

func (b *Browse) Run(build *globals.Build, profile *globals.Profile) error {
	opts, err := b.Modelled(build, profile)
	if err != nil {
		return err
	}
	run(open(opts, b.Address))
	return nil
}
