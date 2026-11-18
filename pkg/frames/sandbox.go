package frames

import "strings"

// Sandbox is what an iframe's sandbox attribute leaves its document.
// The zero value is a frame that has none, which may do everything.
type Sandbox struct {
	// On is whether the frame is sandboxed at all.
	On bool
	// Each of the rest is a token of the attribute, false for one a
	// sandboxed frame was not given.
	Scripts, SameOrigin, Forms, Popups, PopupsEscape, Downloads, Modals bool
	TopNavigation, TopNavigationByUser                                  bool
}

// ParseSandbox reads a sandbox attribute, present whether the iframe
// has one: an empty one allows nothing, and a token light does not
// know is ignored.
func ParseSandbox(value string, present bool) Sandbox {
	s := Sandbox{On: present}
	for _, token := range strings.Fields(strings.ToLower(value)) {
		switch token {
		case "allow-scripts":
			s.Scripts = true
		case "allow-same-origin":
			s.SameOrigin = true
		case "allow-forms":
			s.Forms = true
		case "allow-popups":
			s.Popups = true
		case "allow-popups-to-escape-sandbox":
			s.PopupsEscape = true
		case "allow-downloads":
			s.Downloads = true
		case "allow-modals":
			s.Modals = true
		case "allow-top-navigation":
			s.TopNavigation = true
		case "allow-top-navigation-by-user-activation":
			s.TopNavigationByUser = true
		}
	}
	return s
}

// Under is s for a frame in a document that is itself sandboxed as
// parent: it is left only what both allow.
func (s Sandbox) Under(parent Sandbox) Sandbox {
	if !parent.On {
		return s
	}
	if !s.On {
		return parent
	}
	return Sandbox{
		On:      true,
		Scripts: s.Scripts && parent.Scripts, SameOrigin: s.SameOrigin && parent.SameOrigin,
		Forms: s.Forms && parent.Forms, Popups: s.Popups && parent.Popups,
		PopupsEscape: s.PopupsEscape && parent.PopupsEscape, Downloads: s.Downloads && parent.Downloads,
		Modals: s.Modals && parent.Modals, TopNavigation: s.TopNavigation && parent.TopNavigation,
		TopNavigationByUser: s.TopNavigationByUser && parent.TopNavigationByUser,
	}
}

// Opaque reports whether the frame's document has an origin of its
// own that nothing shares: it keeps and sends no cookie.
func (s Sandbox) Opaque() bool { return s.On && !s.SameOrigin }

// Submits reports whether a form of the frame is sent.
func (s Sandbox) Submits() bool { return !s.On || s.Forms }

// Saves reports whether a link of the frame may save a file.
func (s Sandbox) Saves() bool { return !s.On || s.Downloads }

// NavigatesTop reports whether the frame may send its tab elsewhere,
// by an act of the user (a click) or not.
func (s Sandbox) NavigatesTop(user bool) bool {
	return !s.On || s.TopNavigation || user && s.TopNavigationByUser
}
