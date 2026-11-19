package app

import (
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/console"
)

// posted sends the shown tab where a form of its page goes with a POST:
// the page it has of that address, the form's own often, gives way to
// what answers. The form is sent once: the page is fetched with a GET
// when it is reloaded or come back to.
func (s *Screen) posted(sent web.Submission) {
	s.byPage(func() {
		tab, session := s.browser.Tab(), s.session()
		if l := session.loads[sent.Address]; l != nil {
			l.cancel()
			delete(session.loads, sent.Address)
		}
		session.posts[sent.Address] = sent
		session.note = ""
		s.depart(sent.Address, true, func() {
			s.keep(tab)
			tab.Visit(sent.Address)
		})
	})
}

// unsent says in the tab's console why a form of its page went nowhere.
func (session *session) unsent(why error) {
	session.said.Add(console.Line{Level: console.Warn, Text: "A form was not sent: " + why.Error() + "."})
}
