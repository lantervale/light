package dom

import (
	"runtime"

	"github.com/lantervale/light/pkg/identity"
)

// Navigator is what navigator says of light and of its machine: the
// same as light's requests say. What is left unsaid is this machine's.
type Navigator struct {
	// Agent is navigator.userAgent, and Platform navigator.platform.
	Agent, Platform string
	// Languages is navigator.languages, as Accept-Language lists them.
	Languages []string
	// Cores is navigator.hardwareConcurrency.
	Cores int
	// Driven is navigator.webdriver: whether something else than a
	// person at the window drives light.
	Driven bool
}

// Window is the window around a page's view: its size and its place on
// the screen, in points, and Ratio how many pixels a pixel of the page
// is drawn with.
type Window struct {
	Width, Height, X, Y, Ratio float32
}

func (r *Runtime) navigator(arguments) (any, error) {
	n := r.host.Navigator
	if n.Agent == "" {
		n.Agent = identity.Default("")
	}
	if n.Platform == "" {
		n.Platform = identity.Platform(runtime.GOOS, runtime.GOARCH)
	}
	if len(n.Languages) == 0 {
		n.Languages = identity.Languages(nil)
	}
	if n.Cores <= 0 {
		n.Cores = runtime.NumCPU()
	}
	return map[string]any{"agent": n.Agent, "platform": n.Platform, "languages": n.Languages, "cores": n.Cores,
		"driven": n.Driven}, nil
}

// around is the window around the page as scripts read it: its width,
// its height, where it is, and the pixels a pixel of the page is drawn
// with. A page shown in no window is its own view, drawn as it is.
func (r *Runtime) around(arguments) (any, error) {
	if r.host.Window == nil {
		width, height := float32(0), float32(0)
		if r.host.Viewport != nil {
			width, height = r.host.Viewport()
		}
		return []float32{width, height, 0, 0, 1}, nil
	}
	w := r.host.Window()
	return []float32{w.Width, w.Height, w.X, w.Y, w.Ratio}, nil
}
