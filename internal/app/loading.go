package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/netlog"
	"github.com/lantervale/light/pkg/tabs"
)

// dimAfter is how long a tab holds the page it shows as it is while the
// next one loads: past it the page held is dimmed, and still the one
// shown.
const dimAfter = 4 * time.Second

// pending is a move of a tab whose page is not ready: the tab stands
// where it stood and shows what it showed until the page lands, then
// arrive moves its history there. A move stopped, or one a later move
// takes the place of, leaves the tab as it was.
type pending struct {
	// address is the address the page is fetched at.
	address string
	arrive  func()
	// named is whether the address bar names the address while it is
	// waited for: only a move its user asked for is, so no page has its
	// content read under an address it sent the tab to.
	named bool
	began time.Time
}

// depart moves the shown tab to address, as arrive does it to its
// history: at once to a page light has, or one of its own; once the
// page is ready to one of the web that is fetched, the tab holding
// meanwhile the page it shows. fresh fetches a page the tab has again.
// A move to the address the tab waits for already goes on waiting.
func (s *Screen) depart(address string, fresh bool, arrive func()) {
	tab, session := s.browser.Tab(), s.session()
	key := shownBy(address)
	if p := session.pending; p != nil && p.address == key && session.loads[key] != nil {
		p.arrive, p.named = arrive, p.named || s.from == ""
		s.showNavigation()
		return
	}
	s.giveUp(session)
	if !s.holds(tab, session, key, fresh) {
		if fresh {
			delete(session.pages, key)
		}
		arrive()
		s.show()
		return
	}
	session.pending = &pending{address: key, arrive: arrive, named: s.from == "", began: time.Now()}
	s.begin(tab, key)
	s.showTabs()
	s.showNavigation()
	s.say()
}

// holds reports whether a move of tab to the page fetched at key waits
// for it on the page the tab shows: the page is one of the web the tab
// has not, or is to fetch again, and the tab shows one.
func (s *Screen) holds(tab *tabs.Tab, session *session, key string, fresh bool) bool {
	if !onTheWeb(key) || (session.pages[key] != nil && !fresh) {
		return false
	}
	at := shownBy(tab.Current())
	return !onTheWeb(at) || session.pages[at] != nil
}

// openFresh is Open for an address whose page is fetched even when the
// tab has it.
func (s *Screen) openFresh(address string) {
	tab := s.browser.Tab()
	s.session().note = ""
	s.depart(address, true, func() {
		s.keep(tab)
		tab.Visit(address)
		s.visit(address)
	})
}

// dismiss takes off the window, in the frame its page is left, what a
// tab's page that was left asked or was told of: its question and the
// bar of its slow script.
func (s *Screen) dismiss(tab *tabs.Tab) {
	a := s.modals.of[tab]
	if a == nil || a.s != s {
		return
	}
	if a.open != nil && a.open.live.rt.Closed() {
		s.unshow(a.open)
		a.open = nil
	}
	if a.slow != nil && a.slow.live.rt.Closed() {
		s.unslow(a)
		a.slow = nil
	}
}

// giveUp ends the move a tab waits for, and its load.
func (s *Screen) giveUp(session *session) {
	p := session.pending
	if p == nil {
		return
	}
	session.pending = nil
	if l := session.loads[p.address]; l != nil {
		l.cancel()
		delete(session.loads, p.address)
	}
}

// stop gives up the move the shown tab waits for and stays on the page
// it shows, and reports whether there was one.
func (s *Screen) stop() bool {
	session := s.session()
	if session.pending == nil {
		return false
	}
	s.giveUp(session)
	s.showTabs()
	s.showNavigation()
	s.say()
	return true
}

// stopByKey stops the shown tab's move at Escape, as browsers do.
func (s *Screen) stopByKey() {
	if s.win.Input().Pressed(input.KeyEscape) && !s.overlaid() {
		s.stop()
	}
}

// waited is the address the shown tab waits for, "" for none: the one
// it moves to, or the one it stands on while its first page loads.
func (s *Screen) waited(tab *tabs.Tab) string {
	session := s.sessionOf(tab)
	if p := session.pending; p != nil {
		return p.address
	}
	if at := shownBy(tab.Current()); session.loads[at] != nil {
		return at
	}
	return ""
}

// The share of the progress line each step of a load fills once it
// ended.
func filled(m web.Mark) float32 {
	switch {
	case m >= web.Built:
		return 1
	case m >= web.Gathered:
		return 0.9
	case m >= web.Parsed:
		return 0.7
	case m >= web.Received:
		return 0.6
	case m >= web.Answered:
		return 0.45
	case m >= web.Sent:
		return 0.2
	}
	return 0.08
}

// loadline makes the line under the navigation row that says how far
// the shown tab's load is. It lies over the top of the page, so the
// page never moves for it.
func (s *Screen) loadline() *ui.Node {
	s.line = ui.Div(ui.ID("loadline"), ui.Attr("role", "progressbar"), ui.Attr("aria-label", "Loading"),
		ui.Attr("aria-hidden", "true"))
	return s.line
}

// progressing shows, each frame, what the shown tab waits for: the
// line as far as its load is, creeping toward its next step so it is
// seen to live; the spinner of every tab that loads, turned; and the
// page held dimmed once it was held for long.
func (s *Screen) progressing() {
	tab := s.browser.Tab()
	session := s.session()
	at := s.waited(tab)
	if at == "" {
		if s.line.HasClass("shown") {
			s.line.RemoveClass("shown")
			s.line.SetAttr("aria-hidden", "true")
			s.lineAt = 0
		}
		toggle(s.view, "waiting", false)
	} else {
		var reached web.Mark
		if l := session.loads[at]; l != nil {
			reached = l.timing.Reached()
		}
		floor, next := filled(reached), filled(reached+1)
		s.lineAt = max(s.lineAt, floor)
		s.lineAt += (next - s.lineAt) * 0.02
		if !s.line.HasClass("shown") {
			s.line.AddClass("shown")
			s.line.RemoveAttr("aria-hidden")
		}
		s.line.SetStyle(fmt.Sprintf("transform: scaleX(%.3f)", s.lineAt))
		p := session.pending
		toggle(s.view, "waiting", p != nil && time.Since(p.began) > s.dimAfter)
	}
	s.turn++
	for i, other := range s.browser.Tabs {
		if i < len(s.strip) && s.waited(other) != "" {
			s.strip[i].icon.SetStyle(fmt.Sprintf("transform: rotate(%ddeg)", s.turn*12%360))
		}
	}
}

// spinner is the picture a tab that loads shows in place of its icon.
const spinner = "spinner.svg"

// drew marks the page the shown tab shows as drawn, the frame after the
// one that first drew it, and says its timing where LIGHT_TIMING asks.
func (s *Screen) drew() {
	page := s.session().pages[shownBy(s.browser.Tab().Current())]
	if page == nil || page.Timing == nil || !page.Timing.At(web.Drawn).IsZero() || page.Timing.At(web.Shown).IsZero() {
		return
	}
	page.Timing.Mark(web.Drawn)
	s.tellTiming(page)
}

// tellTiming prints the timing of page on the standard error where
// LIGHT_TIMING asks for it: once it is drawn, and again once loaded.
func (s *Screen) tellTiming(page *web.Page) {
	if os.Getenv("LIGHT_TIMING") != "" {
		fmt.Fprintln(os.Stderr, "light: timing: "+page.Address+": "+timingText(page.Timing))
	}
}

// timingLine is when each step of the load of the shown tab's page at
// address ended, in milliseconds since it was asked for, "" for a page
// that has no timing.
func (s *Screen) timingLine(address string) string {
	page := s.session().pages[address]
	if page == nil || page.Timing == nil {
		return ""
	}
	return address + ": " + timingText(page.Timing)
}

func timingText(t *web.Timing) string {
	var steps []string
	for _, step := range timed(t) {
		steps = append(steps, step[0]+" "+step[1])
	}
	return strings.Join(steps, ", ")
}

// timed is the steps of a load that ended, each with how long after the
// page was asked for.
func timed(t *web.Timing) [][2]string {
	var steps [][2]string
	for _, m := range web.Marks()[1:] {
		if took, ok := t.Since(m); ok {
			steps = append(steps, [2]string{m.String(), fmt.Sprintf("%.1f ms", float64(took.Microseconds())/1000)})
		}
	}
	return steps
}

// timingOf is timed for the page the shown tab has at address, for the
// developer tools.
func (s *Screen) timingOf(address string) [][2]string {
	if page := s.session().pages[address]; page != nil && page.Address == address {
		return timed(page.Timing)
	}
	return nil
}

// tellRequests prints each request of the shown tab that ended since
// the last time, on the standard error where LIGHT_TIMING asks for it:
// those of a page on its way once it is the one shown.
func (s *Screen) tellRequests() {
	if os.Getenv("LIGHT_TIMING") == "" {
		return
	}
	session := s.session()
	page := session.pages[shownBy(s.browser.Tab().Current())]
	if page == nil || page.Timing == nil || session.pending != nil {
		return
	}
	for _, row := range session.log.Rows() {
		if row.Done && row.ID > session.told {
			session.told = row.ID
			fmt.Fprintln(os.Stderr, "light: request: "+requestLine(row, page.Timing))
		}
	}
}

// requestLine is one request in a line: when it was asked, since the
// page of timing was and against the end of its parse, on which
// connection, by which protocol, and how long each of its steps took.
func requestLine(row netlog.Row, timing *web.Timing) string {
	ms := func(d time.Duration) string { return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000) }
	began := timing.At(web.Started)
	if began.IsZero() {
		began = timing.At(web.Asked)
	}
	parts := []string{"#" + strconv.Itoa(row.ID), "+" + ms(row.Started.Sub(began))}
	if parsed := timing.At(web.Parsed); !parsed.IsZero() && row.Started.Before(parsed) {
		parts = append(parts, "("+ms(parsed.Sub(row.Started))+" before parsed)")
	} else if !parsed.IsZero() {
		parts = append(parts, "("+ms(row.Started.Sub(parsed))+" after parsed)")
	}
	how := "new"
	switch {
	case row.Remote == "":
		how = "no connection"
	case row.Reused && row.Idle:
		how = "used again idle"
	case row.Reused:
		how = "used again"
	}
	parts = append(parts, string(row.Type)+" by "+row.Initiator, row.Proto, how)
	t := row.Timing()
	for _, step := range []struct {
		name string
		took time.Duration
	}{
		{"blocked", t.Blocked}, {"dns", t.DNS}, {"connect", t.Connect}, {"tls", t.TLS}, {"send", t.Send},
		{"wait", t.Wait}, {"receive", t.Receive}, {"took", row.Took},
	} {
		if step.took >= 0 {
			parts = append(parts, step.name+" "+ms(step.took))
		}
	}
	return strings.Join(append(parts, strconv.FormatInt(row.Transferred, 10)+" bytes", strconv.Itoa(row.Status),
		row.Address), ", ")
}
