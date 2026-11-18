package frames

import "net/url"

// Kin names the documents of a tab's page that reach each other's
// objects: those of one origin that keep their storage in one place.
// top is the address of the tab's page, chain the documents a frame's
// own is in and itself, the nearest to the page first (a nil for one
// with no origin), none for the page; origin is the document's, as
// light stamped it, and sandbox what its frame keeps it from. Two
// documents of a page are kin when both have a key and the keys are
// equal; false is a document that is kin to none: one with no origin
// of its own, or whose sandbox gives it none, or runs no script.
//
// A document of the page's origin under one of another site keeps its
// storage apart from the page's (Partition), so the two are not kin:
// what browsers let script each other, light keeps apart.
func Kin(top *url.URL, chain []*url.URL, origin string, sandbox Sandbox) (string, bool) {
	if origin == "" || origin == "null" || sandbox.Opaque() || !sandbox.Runs() {
		return "", false
	}
	if under := Partition(top, chain); under != "" {
		return origin + " under " + under, true
	}
	return origin, true
}
