//go:build capture

// Package capture runs light with a model that answers what it is told
// to, for the pictures of the surfaces a model shows: never part of a
// build, it is run by hand with ember's capture variables.
//
//	EMBER_HIDDEN=1 EMBER_CAPTURE=card.png EMBER_CAPTURE_FRAME=90 LIGHT_CAPTURE_DIR=profile \
//	LIGHT_CAPTURE_MODEL=ready go test -tags capture ./internal/capture
package capture_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/lantervale/ember"
	"github.com/lantervale/ember/geom"

	"github.com/lantervale/light/internal/app"
	"github.com/lantervale/light/pkg/model"
)

func init() { runtime.LockOSThread() }

// local refuses every request that would leave this machine.
type local struct{}

func (local) RoundTrip(r *http.Request) (*http.Response, error) {
	if host := r.URL.Hostname(); host != "127.0.0.1" && host != "localhost" {
		return nil, errors.New(host + " is not reached from a capture")
	}
	return http.DefaultTransport.RoundTrip(r)
}

// TestMain shows light at LIGHT_CAPTURE_ADDRESS with the profile of
// LIGHT_CAPTURE_DIR and a model in the state LIGHT_CAPTURE_MODEL names:
// ready, working, preparing, or bare for one whose files are not on the
// device. LIGHT_CAPTURE_REPLY is what the model answers, whole;
// LIGHT_CAPTURE_CATEGORY what it answers when asked for a page's category;
// LIGHT_CAPTURE_SITE a folder served on this machine and opened;
// LIGHT_CAPTURE_SIZE the window's size, as 1024x900.
func TestMain(*testing.M) {
	fake := model.NewFake()
	fake.Manual = true
	state := os.Getenv("LIGHT_CAPTURE_MODEL")
	if state == "bare" {
		for _, e := range fake.Catalogue() {
			fake.SetInstalled(e.ID, false)
		}
	}
	if reply := os.Getenv("LIGHT_CAPTURE_REPLY"); reply != "" {
		fake.Manual = false
		fake.Reply = func(model.Request) (string, error) { return reply, nil }
	}
	if category := os.Getenv("LIGHT_CAPTURE_CATEGORY"); category != "" {
		fake.Manual = false
		reply := fake.Reply
		fake.Reply = func(req model.Request) (string, error) {
			switch {
			case req.Grammar != "":
				return category, nil
			case reply != nil:
				return reply(req)
			}
			return "", nil
		}
	}
	address := os.Getenv("LIGHT_CAPTURE_ADDRESS")
	if site := os.Getenv("LIGHT_CAPTURE_SITE"); site != "" {
		srv := httptest.NewServer(http.FileServer(http.Dir(site)))
		defer srv.Close()
		address = srv.URL + "/"
	}
	profile := app.NewProfile(app.Options{Dir: os.Getenv("LIGHT_CAPTURE_DIR"), Downloads: os.TempDir(),
		Transport: local{},
		Model:     fake, Library: fake, Driven: true})
	switch state {
	case "working":
		fake.SetState(model.Working, "")
	case "preparing":
		fake.SetState(model.Preparing, "")
	}
	win := app.Window(nil)
	if w, h, ok := strings.Cut(os.Getenv("LIGHT_CAPTURE_SIZE"), "x"); ok {
		width, _ := strconv.Atoi(w)
		height, _ := strconv.Atoi(h)
		win.SetSize(geom.V2(float32(width), float32(height)))
	}
	screen := profile.Screen(win)
	if address != "" {
		screen.Open(screen.Resolve(address))
	}
	win.Add(screen.Document())
	ember.New().AddEntities(win).AddSystems(profile).MustRun()
	_ = profile.Close()
}
