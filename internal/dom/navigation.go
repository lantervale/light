package dom

// sends reports whether r's document may navigate as it is about to:
// elsewhere, to address when it names one, by a form when form.
//
// The standard checks a navigation against the document of the script
// that asked for it (the incumbent), not the one whose location, link
// or form it went through. V8 does not tell light the incumbent, so
// light asks the document whose scripts were entered, which is that
// script's unless it handed a function of its own to its kin to call:
// when it is not r's, it must be allowed to send r's window there
// itself (Windows.May), or r goes nowhere. A document that navigates
// from its own script is asked nothing here.
//
// This keeps a sandboxed frame that is kin to its page from sending
// its kin where its sandbox does not let it by reaching through their
// documents. It is no wall: such a frame can have its kin run what it
// likes, as in every browser.
func (r *Runtime) sends(address string, form bool) bool {
	by := r.asking.running
	if by == nil || by == r || by.closed {
		return true
	}
	return by.windows().May(r.host.Self, address, form)
}
