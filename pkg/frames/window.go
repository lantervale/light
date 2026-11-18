package frames

import "net/url"

// opaque is how an origin nothing shares is written.
const opaque = "null"

// Shape is what a member of another frame's window is to a script.
type Shape string

const (
	// ItsWindow is the window itself, and AWindow another one, or the reader's
	// own.
	ItsWindow Shape = "self"
	AWindow   Shape = "window"
	// ANull, ANumber and ABoolean are plain values.
	ANull    Shape = "null"
	ANumber  Shape = "number"
	ABoolean Shape = "boolean"
	// ItsLocation is the window's location, itself a facade, which may
	// also be set: the window then goes there.
	ItsLocation Shape = "location"
	// AMethod is a function of the reader's own realm, and ASetter what
	// can only be set.
	AMethod Shape = "method"
	ASetter Shape = "setter"
)

// Member is one name a script may use on the window, or the location,
// of another frame: the cross-origin properties of the HTML Standard.
type Member struct {
	Name  string
	Shape Shape
}

// WindowMembers is every name of the window of another frame that
// answers a script; its child frames answer by their index and by
// their name too. LocationMembers is the same for its location. Any
// other name, read, set, defined, deleted or asked for, is a
// SecurityError, but those of Quiet, which read undefined.
func WindowMembers() []Member {
	return []Member{
		{"window", ItsWindow}, {"self", ItsWindow}, {"frames", ItsWindow},
		{"parent", AWindow}, {"top", AWindow}, {"opener", ANull},
		{"length", ANumber}, {"closed", ABoolean}, {"location", ItsLocation},
		{"postMessage", AMethod}, {"close", AMethod}, {"focus", AMethod}, {"blur", AMethod},
	}
}

func LocationMembers() []Member {
	return []Member{{"href", ASetter}, {"replace", AMethod}}
}

// Quiet are the names that read undefined on another frame's window
// and location, so a promise can be resolved with one; with them go
// the symbols toStringTag, hasInstance and isConcatSpreadable.
func Quiet() []string { return []string{"then"} }

// Carried is a kind of value a message carries from one document to
// another, and the mark it is written down under.
type Carried struct{ Name, Mark string }

// Carries are the kinds of values a message carries: what the
// structured clone copies, written down as text and bytes and made
// again from the receiver's own objects. A plain text, a boolean, null
// and a finite number are written as they are. Whatever else a script
// posts (a function, a symbol, a node, a window, a promise) is refused.
func Carries() []Carried {
	return []Carried{
		{"undefined", "u"}, {"a number that is none", "n"}, {"a bigint", "i"}, {"a value met before", "r"},
		{"a wrapped primitive", "p"}, {"a Date", "d"}, {"a RegExp", "x"}, {"an ArrayBuffer", "b"}, {"a view", "v"},
		{"a Map", "m"}, {"a Set", "s"}, {"an error", "e"}, {"an Array", "a"}, {"an Object", "o"}, {"a Blob", "B"},
		{"a File", "F"}, {"a MessagePort", "P"},
	}
}

// Reaches reports whether a message posted for target, a targetOrigin
// as scripts write one, is for a window whose document is of the
// origin receiver, when the sender's is sender: "*" is for any, "/"
// for the sender's own. The second is false for what is no origin.
func Reaches(target, sender, receiver string) (bool, bool) {
	switch target {
	case "*":
		return true, true
	case "/":
		return sender == receiver && sender != opaque, true
	}
	to, err := url.Parse(target)
	if err != nil || to.Scheme == "" || to.Host == "" && to.Opaque == "" {
		return false, false
	}
	return Origin(to) == receiver && receiver != opaque, true
}

// Navigation is who asks a window to go elsewhere and which window.
type Navigation struct {
	// Self is a frame sending itself, Descendant one sending a frame of
	// its own document or under one, and Top one sending its tab.
	Self, Descendant, Top bool
	// SameOrigin is whether the two documents are of one origin that is
	// not opaque, and Activated whether the asker heard a click of the
	// user's a moment ago.
	SameOrigin, Activated bool
	// Sandbox is the asker's.
	Sandbox Sandbox
}

// Allowed reports whether the navigation is one light lets a script
// make: a frame sends itself and the frames under it anywhere; its tab
// when it is of the tab's origin or heard a click, and its sandbox, if
// any, has the token for it; any other frame only when it is of its
// origin, and never from a sandbox.
func (n Navigation) Allowed() bool {
	switch {
	case n.Self, n.Descendant:
		return true
	case n.Top:
		if n.Sandbox.On {
			return n.Sandbox.NavigatesTop(n.Activated)
		}
		return n.SameOrigin || n.Activated
	}
	return n.SameOrigin && !n.Sandbox.On
}
