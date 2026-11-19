// Package commands holds light's commands: browse, a person's browser,
// and drive, a light a test runner drives.
package commands

import (
	"log"

	"github.com/lantervale/ember"
	"github.com/lantervale/ember/world"

	"github.com/lantervale/light/internal/app"
)

// open makes a light of those options, its first window at address
// when one is given, and the World they are in, with the system's
// preferences the options hold.
func open(opts app.Options, address string) (*app.Profile, *world.World) {
	profile := app.NewProfile(opts)
	win := app.Window(nil)
	screen := profile.Screen(win)
	if address != "" {
		screen.Open(screen.Resolve(address))
	}
	win.Add(screen.Document())
	w := ember.New().AddEntities(win).AddSystems(profile)
	if opts.Preferences != nil {
		w.AddEntity(opts.Preferences)
	}
	return profile, w
}

// run runs the World until its last window closes, then has the
// profile keep what it must.
func run(profile *app.Profile, w *world.World) {
	w.MustRun()
	if err := profile.Close(); err != nil {
		log.Print(err)
	}
}
