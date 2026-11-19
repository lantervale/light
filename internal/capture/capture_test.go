//go:build capture

// Package capture runs light with a model that answers what it is told
// to, for the pictures of the surfaces a model shows: never part of a
// build, it is run by hand with ember's capture variables.
//
//	EMBER_HIDDEN=1 EMBER_CAPTURE=card.png EMBER_CAPTURE_FRAME=90 LIGHT_CAPTURE_DIR=profile \
//	LIGHT_CAPTURE_MODEL=ready go test -tags capture ./internal/capture
package capture_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/lantervale/ember"

	"github.com/lantervale/light/internal/app"
	"github.com/lantervale/light/pkg/model"
)

func init() { runtime.LockOSThread() }

// TestMain shows light at LIGHT_CAPTURE_ADDRESS with the profile of
// LIGHT_CAPTURE_DIR and a model in the state LIGHT_CAPTURE_MODEL names:
// ready, working, preparing, or bare for one whose files are not on the
// device.
func TestMain(*testing.M) {
	fake := model.NewFake()
	fake.Manual = true
	state := os.Getenv("LIGHT_CAPTURE_MODEL")
	if state == "bare" {
		for _, e := range fake.Catalogue() {
			fake.SetInstalled(e.ID, false)
		}
	}
	profile := app.NewProfile(app.Options{Dir: os.Getenv("LIGHT_CAPTURE_DIR"), Downloads: os.TempDir(),
		Model: fake, Library: fake, Driven: true})
	switch state {
	case "working":
		fake.SetState(model.Working, "")
	case "preparing":
		fake.SetState(model.Preparing, "")
	}
	win := app.Window(nil)
	screen := profile.Screen(win)
	if address := os.Getenv("LIGHT_CAPTURE_ADDRESS"); address != "" {
		screen.Open(screen.Resolve(address))
	}
	win.Add(screen.Document())
	ember.New().AddEntities(win).AddSystems(profile).MustRun()
	_ = profile.Close()
}
