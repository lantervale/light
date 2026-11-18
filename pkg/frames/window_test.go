package frames_test

import (
	"testing"

	"github.com/lantervale/light/pkg/frames"
)

func TestTheNamesOfAnotherFramesWindowAreTheStandards(t *testing.T) {
	want := map[string]frames.Shape{
		"window": frames.ItsWindow, "self": frames.ItsWindow, "frames": frames.ItsWindow, "parent": frames.AWindow,
		"top": frames.AWindow, "opener": frames.ANull, "length": frames.ANumber, "closed": frames.ABoolean,
		"location": frames.ItsLocation, "postMessage": frames.AMethod, "close": frames.AMethod, "focus": frames.AMethod,
		"blur": frames.AMethod,
	}
	got := map[string]frames.Shape{}
	for _, m := range frames.WindowMembers() {
		if _, twice := got[m.Name]; twice {
			t.Errorf("%s is listed twice", m.Name)
		}
		got[m.Name] = m.Shape
	}
	if len(got) != len(want) {
		t.Errorf("%d names answer on another frame's window, want %d", len(got), len(want))
	}
	for name, shape := range want {
		if got[name] != shape {
			t.Errorf("%s is %q, want %q", name, got[name], shape)
		}
	}
	location := frames.LocationMembers()
	if len(location) != 2 || location[0] != (frames.Member{Name: "href", Shape: frames.ASetter}) ||
		location[1] != (frames.Member{Name: "replace", Shape: frames.AMethod}) {
		t.Errorf("another frame's location answers %v, want href to set and replace alone", location)
	}
	if quiet := frames.Quiet(); len(quiet) != 1 || quiet[0] != "then" {
		t.Errorf("the names that read undefined are %v", quiet)
	}
}

func TestAMessageReachesOnlyTheOriginItIsFor(t *testing.T) {
	const a, b = "https://a.test", "https://b.test:8443"
	for _, c := range []struct {
		target, sender, receiver string
		reaches, ok              bool
	}{
		{"*", a, b, true, true},
		{"*", "null", "null", true, true},
		{"/", a, a, true, true},
		{"/", a, b, false, true},
		{"/", "null", "null", false, true},
		{a, b, a, true, true},
		{a + "/some/path?q#f", b, a, true, true},
		{"HTTPS://A.test:443", b, a, true, true},
		{a, b, b, false, true},
		{"https://b.test", a, b, false, true},
		{b, a, b, true, true},
		{"http://a.test", b, a, false, true},
		{a, b, "null", false, true},
		{"null", a, "null", false, false},
		{"", a, a, false, false},
		{"a.test", a, a, false, false},
		{"//a.test", a, a, false, false},
		{"https://", a, a, false, false},
		{"about:blank", a, "null", false, true},
		{"data:text/html,x", a, "null", false, true},
	} {
		reaches, ok := frames.Reaches(c.target, c.sender, c.receiver)
		if reaches != c.reaches || ok != c.ok {
			t.Errorf("a message for %q from %s to %s: reaches %v, an origin %v; want %v, %v", c.target, c.sender,
				c.receiver, reaches, ok, c.reaches, c.ok)
		}
	}
}

func TestAFrameSendsOnlyTheWindowsItMay(t *testing.T) {
	box := func(tokens string) frames.Sandbox { return frames.ParseSandbox(tokens, true) }
	for _, c := range []struct {
		name string
		n    frames.Navigation
		want bool
	}{
		{"itself", frames.Navigation{Self: true}, true},
		{"itself, sandboxed", frames.Navigation{Self: true, Sandbox: box("")}, true},
		{"a frame under it", frames.Navigation{Descendant: true}, true},
		{"a frame under it, sandboxed", frames.Navigation{Descendant: true, Sandbox: box("allow-scripts")}, true},
		{"its tab, of another origin, unasked", frames.Navigation{Top: true}, false},
		{"its tab, of its origin", frames.Navigation{Top: true, SameOrigin: true}, true},
		{"its tab, after a click", frames.Navigation{Top: true, Activated: true}, true},
		{"its tab, sandboxed with its origin", frames.Navigation{Top: true, SameOrigin: true,
			Sandbox: box("allow-scripts allow-same-origin")}, false},
		{"its tab, sandboxed, allowed", frames.Navigation{Top: true, Sandbox: box("allow-top-navigation")}, true},
		{"its tab, sandboxed, allowed on a click, unasked", frames.Navigation{Top: true,
			Sandbox: box("allow-top-navigation-by-user-activation")}, false},
		{"its tab, sandboxed, allowed on a click, clicked", frames.Navigation{Top: true, Activated: true,
			Sandbox: box("allow-top-navigation-by-user-activation")}, true},
		{"its tab, sandboxed, clicked, not allowed", frames.Navigation{Top: true, Activated: true,
			Sandbox: box("allow-scripts")}, false},
		{"a frame beside it", frames.Navigation{}, false},
		{"a frame beside it, after a click", frames.Navigation{Activated: true}, false},
		{"a frame beside it, of its origin", frames.Navigation{SameOrigin: true}, true},
		{"a frame beside it, of its origin, sandboxed", frames.Navigation{SameOrigin: true,
			Sandbox: box("allow-same-origin allow-top-navigation")}, false},
	} {
		if got := c.n.Allowed(); got != c.want {
			t.Errorf("a frame sending %s: allowed %v, want %v", c.name, got, c.want)
		}
	}
}
