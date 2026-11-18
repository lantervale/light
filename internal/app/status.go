package app

// follow shows the whole address while its field has the focus, and
// moves the status bubble to the right while the pointer is where it
// stands on the left.
func (s *Screen) follow() {
	if editing := s.doc.Focused(s.address); editing != s.editing {
		s.editing = editing
		s.locate()
	}
	stage, _ := s.doc.BoxOf(s.status.Parent())
	bubble, shown := s.doc.BoxOf(s.status)
	at := s.win.Input().Mouse()
	under := shown && at.X >= stage.X && at.X < stage.X+bubble.W+16 && at.Y >= stage.Y+stage.H-bubble.H-8 &&
		at.Y < stage.Y+stage.H
	if under != s.status.HasClass("away") {
		toggle(s.status, "away", under)
	}
}

// point names the address under the pointer, "" for none.
func (s *Screen) point(address string) {
	s.over = address
	s.say()
}

// say fills the status bubble: the address under the pointer, else the
// host the shown page is waited for from, else what the tab's last move
// came to, else nothing, and no bubble.
func (s *Screen) say() {
	text := shownAs(s.over)
	switch {
	case text != "":
	case s.loading():
		text = "Waiting for " + hostOf(shownBy(s.browser.Tab().Current())) + "…"
	default:
		text = s.session().note
	}
	if text != s.status.Text() {
		s.status.SetText(text)
	}
	toggle(s.status, "shown", text != "")
}
