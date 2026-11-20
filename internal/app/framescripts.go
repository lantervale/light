package app

import (
	"context"
	"net/url"
	"slices"
	"time"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/cookies"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/frames"
	"github.com/lantervale/light/pkg/identity"
	"github.com/lantervale/light/pkg/localnet"
)

const (
	// topWindow names the window of a tab's page to the scripts of its
	// frames; a frame's is a number past it, never given twice.
	topWindow = 1
	// opaqueOrigin is the origin of a document nothing shares its
	// origin with.
	opaqueOrigin = "null"
	// inlineAddress is the address of a document written in its iframe.
	inlineAddress = "about:srcdoc"
	// maxMail is how many messages may wait for the windows of a page,
	// or in one end of a channel.
	maxMail = 4096
	// mailRounds is how many times in a frame of the tab its documents
	// are given what they posted each other, and mailTime how long that
	// may take.
	mailRounds = 256
	mailTime   = 4 * time.Millisecond
)

// letter is a message one window of a page posted another: what its
// sender wrote down, the origin it is for, and who sent it, a frame or
// nil for the page, from what origin. It is for the scripts that ran in
// its window when it was posted, and no others.
type letter struct {
	to      *running
	message dom.Posted
	target  string
	from    *frame
	origin  string
}

// originOf is the origin of page as the document f shows: none of its
// own in a sandbox that gives it none, that of the document it is
// written in for an inline one, else that of the address it came from,
// which nothing in it changes.
func (f *frame) originOf(page *web.Page) string {
	switch {
	case f.held.Opaque():
		return opaqueOrigin
	case page.Address != inlineAddress:
		return frames.Origin(page.Base)
	case f.parent != nil:
		return f.parent.origin
	}
	return frames.Origin(f.set.top.Base)
}

// quit ends the scripts of the document f shows.
func (f *frame) quit() {
	if f.run != nil {
		f.run.rt.Close()
		f.run = nil
	}
}

// placed reports whether f's element and those of the frames above it
// are in their documents: one that left is emptied at the end of the
// frame, and nothing under it runs till then.
func (f *frame) placed() bool {
	for ; f != nil; f = f.parent {
		if !rooted(f.node, f.in.Doc) || (f.parent != nil && f.parent.page != f.in) {
			return false
		}
	}
	return true
}

// windowed reports whether f's element has a window: it is in its
// document, and the tab had room for it.
func (f *frame) windowed() bool {
	return !f.gone && rooted(f.node, f.in.Doc) && (f.parent == nil || f.parent.page == f.in) && (!f.live || f.admitted)
}

// label is how the console names the frame: the host of its document,
// or what it is.
func (f *frame) label() string {
	switch {
	case f.page == nil:
		return "frame"
	case f.page.Address == inlineAddress:
		return "srcdoc"
	case f.page.Address == blankAddress:
		return blankAddress
	case f.page.Base != nil && f.page.Base.Host != "":
		return f.page.Base.Host
	}
	return "frame"
}

// chain are the documents f's own is in and itself, the nearest to
// the tab's page first, a nil for one with no origin.
func (f *frame) chain() []*url.URL {
	chain := f.documents()
	chain = chain[:len(chain)-1]
	if f.held.Opaque() {
		chain = append(chain, nil)
	} else if f.page != nil {
		chain = append(chain, f.page.Base)
	}
	return chain
}

// crossed reports whether the document f shows, or one it is in, is of
// another site than its tab's page, or of none.
func (f *frame) crossed() bool { return frames.Partition(f.set.top.Base, f.chain()) != "" }

// byWindow is the frame of that window, nil with true for the page's.
func (set *framed) byWindow(id int) (*frame, bool) {
	if id == topWindow {
		return nil, true
	}
	for _, f := range set.all {
		if f.id == id && f.windowed() {
			return f, true
		}
	}
	return nil, false
}

// within are the frames of the document parent shows, the page's for
// nil, as that document orders them.
func (set *framed) within(parent *frame) []*frame {
	in := set.shownBy(parent)
	if in == nil {
		return nil
	}
	set.adopt(parent, in)
	held := map[*ui.Node]*frame{}
	for _, f := range set.all {
		if f.parent == parent && f.in == in && f.windowed() {
			held[f.node] = f
		}
	}
	var out []*frame
	if len(held) > 0 {
		in.Doc.HTML().Walk(func(n *ui.Node) bool {
			if f := held[n]; f != nil {
				out = append(out, f)
			}
			return len(out) < len(held)
		})
	}
	return out
}

// shownBy is the document a window shows, the page for nil; nil for
// none.
func (set *framed) shownBy(parent *frame) *web.Page {
	in := set.top
	if parent != nil {
		in = parent.page
	}
	if in == nil || in.Doc == nil {
		return nil
	}
	return in
}

// scriptsOf are the scripts of a window: the page's for nil.
func (set *framed) scriptsOf(f *frame) *running {
	if f != nil {
		return f.run
	}
	if live := set.session.scripts; live != nil && live.page == set.top {
		return live
	}
	return nil
}

// originIn is the origin of the document of a window: the page's for
// nil.
func (set *framed) originIn(f *frame) string {
	if f != nil {
		return f.origin
	}
	return frames.Origin(set.top.Base)
}

// windows is the windows of a tab's page as the scripts of one of its
// documents know them: by is the frame that document is in, nil for
// the page itself. Who asks is by, whatever its scripts say.
type windows struct {
	s   *Screen
	set *framed
	by  *frame
}

// frame is the frame of the window a script names, nil with true for
// the page's.
func (w *windows) frame(id int) (*frame, bool) {
	if id == dom.OwnWindow {
		return w.by, w.by == nil || w.by.windowed()
	}
	return w.set.byWindow(id)
}

// name is how the scripts of by know the window of f.
func (w *windows) name(f *frame) int {
	switch f {
	case w.by:
		return dom.OwnWindow
	case nil:
		return topWindow
	}
	return f.id
}

func (w *windows) Parent(of int) int {
	f, ok := w.frame(of)
	switch {
	case !ok:
		return dom.NoWindow
	case f == nil:
		return w.name(nil)
	}
	return w.name(f.parent)
}

func (w *windows) Top(int) int { return w.name(nil) }

func (w *windows) Length(of int) int {
	f, ok := w.frame(of)
	if !ok {
		return 0
	}
	return len(w.set.within(f))
}

func (w *windows) Child(of, index int) int {
	f, ok := w.frame(of)
	if !ok {
		return dom.NoWindow
	}
	if children := w.set.within(f); index >= 0 && index < len(children) {
		return w.name(children[index])
	}
	return dom.NoWindow
}

func (w *windows) Named(of int, name string) int {
	f, ok := w.frame(of)
	if !ok || name == "" {
		return dom.NoWindow
	}
	for _, child := range w.set.within(f) {
		if has, _ := child.node.Attr("name"); has == name {
			return w.name(child)
		}
	}
	return dom.NoWindow
}

func (w *windows) Closed(of int) bool {
	_, ok := w.frame(of)
	return !ok
}

func (w *windows) Of(n *ui.Node) int {
	for _, child := range w.set.within(w.by) {
		if child.node == n {
			return child.id
		}
	}
	return dom.NoWindow
}

// Element is the iframe of by's document that shows the window of.
func (w *windows) Element(of int) *ui.Node {
	if f, ok := w.set.byWindow(of); ok && f != nil && f.parent == w.by {
		return f.node
	}
	return nil
}

// Kin reports whether the document the window of shows is kin to
// by's: of its origin, keeping its storage where it does, and so in
// its engine. A document of by's kin that has no scripts running yet
// gets them here, so by's can reach it.
func (w *windows) Kin(of int) bool {
	mine := w.set.scriptsIn(w.set.session, w.by)
	if of == dom.OwnWindow || mine == nil || mine.kin == nil {
		return false
	}
	f, ok := w.set.byWindow(of)
	if !ok || f == w.by {
		return false
	}
	theirs := w.set.scriptsIn(w.set.session, f)
	if f == nil && theirs != nil && theirs.page != w.set.top {
		return false
	}
	if theirs == nil && f != nil && (f.page != nil || w.s.initial(f, mine.kin)) && w.s.reachable(f) &&
		w.set.kinOf(f) == mine.kin {
		w.s.startFrame(f)
		theirs = f.run
	}
	return theirs != nil && theirs.kin == mine.kin && !theirs.rt.Closed()
}

// Post leaves a message for the scripts that run in a window now: it
// comes at the tab's next frame, if their document's origin is then
// the one the message is for.
func (w *windows) Post(to int, message dom.Posted, target string) error {
	set := w.set
	if !set.take(set.scriptsOf(w.by), message) {
		return nil
	}
	_, is := frames.Reaches(target, opaqueOrigin, opaqueOrigin)
	f, ok := w.frame(to)
	live := set.scriptsOf(f)
	if !is || !ok || live == nil || len(set.mail) >= maxMail {
		set.lose(message)
		if !is {
			return dom.ErrTargetOrigin
		}
		return nil
	}
	set.mail = append(set.mail, letter{to: live, message: message, target: target, from: w.by,
		origin: set.originIn(w.by)})
	return nil
}

// deliver gives the windows of the page what was posted them, in the
// order it was: each to the scripts it was for, if they still run in
// their window and their document is of the origin the message is for.
//
// What a message makes its reader post comes in the same frame of the
// tab, after what was posted before it, until rounds of them were
// given or they took too long: a talk between two documents does not
// wait a frame of the tab for each answer.
func (set *framed) deliver() {
	began := time.Now()
	set.sweep()
	for round := 0; round < mailRounds && time.Since(began) < mailTime; round++ {
		if len(set.mail) == 0 && !set.carries() {
			break
		}
		set.post()
		set.carry()
	}
	set.orphans()
}

// post gives the windows what waits for them now.
func (set *framed) post() {
	mail := set.mail
	set.mail = nil
	for _, m := range mail {
		var to *frame
		if at := slices.IndexFunc(set.all, func(f *frame) bool { return f.run == m.to }); at >= 0 {
			to = set.all[at]
		}
		if set.scriptsOf(to) != m.to || m.to.rt.Closed() {
			set.lose(m.message)
			continue
		}
		if reaches, _ := frames.Reaches(m.target, m.origin, set.originIn(to)); !reaches {
			set.lose(m.message)
			continue
		}
		set.give(m.to, m.message)
		source := dom.NoWindow
		switch {
		case m.from == to:
			source = dom.OwnWindow
		case m.from == nil:
			source = topWindow
		case m.from.windowed():
			source = m.from.id
		}
		m.to.rt.Message(m.message, m.origin, source)
	}
}

// Go sends a window elsewhere when the document that asks may: itself
// and the frames in it always, its tab as its sandbox and the user's
// act allow, any other frame only when it is of its origin.
func (w *windows) Go(to int, address string, replace bool) {
	s, set := w.s, w.set
	target, ok := w.frame(to)
	asking := set.scriptsOf(w.by)
	if !ok || asking == nil || !w.sends(target, asking, address, true) {
		return
	}
	from := asking.page
	user := asking.acting
	s.later = append(s.later, func() {
		switch {
		case s.browser.Tab() != set.tab || set.scriptsOf(w.by) != asking:
		case target != nil && !target.gone:
			how := aStep
			if replace {
				how = inPlace
			}
			s.navigate(target, wanted{from: from, ref: address, user: user, how: how})
		case target != nil:
		case replace:
			s.fromPage(from.Address, func() {
				asking.by(s)(func() {
					set.tab.Redirect(set.tab.Current(), address)
					s.show()
				})
			})
		default:
			s.fromPage(from.Address, func() { asking.by(s)(func() { s.Open(address) }) })
		}
	})
}

// sends reports whether by's document may send target, the tab for
// nil, elsewhere: to address when named is set. The console is told
// of what it may not.
func (w *windows) sends(target *frame, asking *running, address string, named bool) bool {
	set := w.set
	origin := set.originIn(w.by)
	move := frames.Navigation{
		Self: target == w.by, Top: target == nil, Activated: asking.acting,
		Descendant: target != nil && (w.by == nil || target.within(w.by)),
		SameOrigin: origin != opaqueOrigin && origin == set.originIn(target),
	}
	if w.by != nil {
		move.Sandbox = w.by.held
	}
	if named && !onTheWeb(address) || !move.Allowed() {
		where := "elsewhere"
		if named {
			where = "to '" + address + "'"
		}
		set.session.said.Add(console.Line{Level: console.Error, Frame: w.label(), Text: "Unsafe attempt to initiate " +
			"navigation for a frame from the frame with URL '" + asking.page.Address + "': it may not send that frame " +
			where + "."})
		return false
	}
	return true
}

// May reports whether by's document may have the document of the
// window to navigate: its script called into that document, which is
// kin to it, and what a document may not do itself it does not have
// another do. A form is sent only from a sandbox that sends forms.
func (w *windows) May(to int, address string, form bool) bool {
	target, ok := w.set.byWindow(to)
	asking := w.set.scriptsOf(w.by)
	if !ok || asking == nil {
		return false
	}
	if form && w.by != nil && !w.by.held.Submits() {
		w.set.session.said.Add(console.Line{Level: console.Warn, Frame: w.label(), Text: "A form was not sent: the " +
			"script that sent it is of a sandboxed frame, and the 'allow-forms' permission is not set."})
		return false
	}
	return w.sends(target, asking, address, address != "")
}

// Focus gives a window the keyboard when the document that asks may:
// on the user's click, or when it holds the keyboard itself.
func (w *windows) Focus(to int) {
	set := w.set
	target, ok := w.frame(to)
	asking, live := set.scriptsOf(w.by), set.scriptsOf(target)
	if !ok || asking == nil || !asking.acting && !asking.keyboard {
		return
	}
	for _, other := range w.s.scripted(set) {
		other.keyboard = false
	}
	if live != nil {
		live.keyboard = true
	}
	if target != nil && target.in.Doc != nil {
		target.in.Doc.Focus(target.node)
	}
}

func (w *windows) label() string {
	if w.by == nil {
		return ""
	}
	return w.by.label()
}

// runFrames starts the scripts of the frames of the page that have
// some and none running, and runs a frame of each whose element is
// still in its document: one whose scripts fail, or never end, stops
// alone.
func (s *Screen) runFrames(set *framed) {
	if set == nil {
		return
	}
	for _, f := range slices.Clone(set.all) {
		if f.gone || f.page == nil || !f.placed() {
			continue
		}
		if f.run == nil && s.runs(f) {
			s.startFrame(f)
		}
		if f.run != nil {
			f.run.rt.Frame()
		}
		if f.loads && (f.run == nil || f.run.rt.Closed() || f.run.rt.Loaded()) {
			f.loads = false
			s.tellFrame(f, "load")
		}
	}
}

// startFrame makes the scripts of the document f shows, and runs them:
// later, while a script of its kin waits for an answer and holds the
// engine it is to share.
func (s *Screen) startFrame(f *frame) {
	live := &running{page: f.page, at: f.page.Address}
	if f.page.Address == inlineAddress && f.page.Base != nil {
		live.at = f.page.Base.String()
	}
	if live.kin = f.set.kinOf(f); live.kin.Held() {
		return
	}
	rt, err := dom.New(f.page, s.frameHost(f, live))
	if err != nil {
		f.set.session.said.Add(console.Line{Level: console.Error, Frame: f.label(), Source: f.page.Address,
			Text: "The frame's scripts do not run: " + err.Error()})
		f.page.Scripts, f.page.Tree = nil, nil
		return
	}
	live.rt, f.run = rt, live
	rt.Start()
}

// frameHost is what the scripts of the document f shows ask of the
// screen: what the page's own ask (scriptHost), as a frame may. Its
// origin, its cookies, its storage and what it may reach are f's, set
// here and never read from its scripts.
func (s *Screen) frameHost(f *frame, live *running) dom.Host {
	set := f.set
	session, label, origin, opaque := set.session, f.label(), f.origin, f.held.Opaque()
	later := func(do func()) {
		s.later = append(s.later, func() {
			if !f.gone && f.run == live && s.browser.Tab() == set.tab {
				do()
			}
		})
	}
	host := dom.Host{
		Console: func(m dom.Message) {
			session.said.Add(console.Line{Level: console.Level(m.Level), Text: m.Text, Source: m.Source, Line: m.Line,
				Stack: m.Stack, Depth: m.Depth, At: m.At, Frame: label})
		},
		Ask: s.frameDialogs(f, live), Hold: &s.modalOf(f.set.tab).hold,
		Slow: s.frameSlowed(f, live), Patience: s.patience(),
		Navigate: func(address string, replace bool) {
			if !onTheWeb(address) {
				return
			}
			user, how := live.acting, aStep
			if replace {
				how = inPlace
			}
			later(func() {
				if replace || !s.placeFrame(f, address) {
					s.navigate(f, wanted{from: live.page, ref: address, user: user, how: how})
				}
			})
		},
		Reload: func() {
			later(func() {
				if live.page.Address == inlineAddress {
					f.key = ""
					return
				}
				s.navigate(f, wanted{from: live.page, ref: live.page.Address, how: inPlace})
			})
		},
		Post: func(sent web.Submission) {
			user := live.acting
			later(func() { s.postFrom(f, live, sent, user) })
		},
		Moved: func(how, address string) {
			live.at = address
			if f.run == live {
				s.renamed(f, how, address)
				s.later = append(s.later, s.showNavigation)
			}
		},
		Travel: func(delta int) {
			user := live.acting
			later(func() { s.travel(f, user, delta) })
		},
		Steps:   set.tab.Len,
		Fetch:   func(q dom.Request, done func(dom.Response)) func() { return s.fetchInFrame(f, live, q, done) },
		Storage: func(area string) dom.Storage { return s.frameStorage(f, origin, area) },
		Bases:   s.frameDatabases(f, origin),
		Viewport: func() (float32, float32) {
			if f.gone || f.in.Doc == nil {
				return 0, 0
			}
			return s.frameSize(f)
		},
		Navigator: s.navigator(),
		Window:    func() dom.Window { return s.around(1) },
		Budget:    s.opts.ScriptBudget,
		Origin:    func() string { return origin },
		Sandboxed: dom.Sandboxed{Opaque: opaque, NoModals: !f.held.Asks()},
		Windows:   &windows{s: s, set: set, by: f},
		Loading:   func() bool { return set.loading(f) },
		Kin:       live.kin, Self: f.id,
		Allows: f.frameAllows,
	}
	s.pageModel(&host)
	if !opaque {
		within := district{origin: origin}
		if f.crossed() {
			within.top = frames.Site(set.top.Base)
		}
		host.Broadcast = func(name string, m dom.Posted) { s.broadcast(live, within, name, m) }
	}
	if !opaque {
		host.Cookies = s.jar.Frame(live.address, set.top.Base, f.crossed())
	}
	return host
}

// frameStorage is an area of the storage of a frame's document, of
// that origin: the site's own when the frame and every document it is
// in are of its tab's site, else one kept apart under that site, which
// neither the site's own pages nor its frames under another site read.
func (s *Screen) frameStorage(f *frame, origin, area string) dom.Storage {
	if origin == opaqueOrigin {
		return nil
	}
	top := ""
	if f.crossed() {
		top = frames.Site(f.set.top.Base)
	}
	return s.storageIn(f.set.session, top, origin, area)
}

// asksInFrame is ctx for what a script of the document f shows asked
// for: from that document, with the cookies its frame's requests carry
// and none without an origin, never in the clear under a page that
// came over https.
func (s *Screen) asksInFrame(ctx context.Context, f *frame, live *running, q dom.Request) context.Context {
	set := f.set
	if f.held.Opaque() {
		ctx = cookies.Without(ctx)
	} else {
		ctx = cookies.Framed(ctx, set.top.Base, f.crossed())
	}
	dest := identity.Empty
	switch {
	case q.Script:
		dest = identity.Script
	case q.Sheet:
		dest = identity.Style
	}
	ctx = fetch.Asking(fetch.By(localnet.From(ctx, live.page.Space), live.address()),
		identity.Request{Destination: dest, Opaque: f.held.Opaque()})
	top, documents := set.top.Address, append([]*url.URL{live.page.Base}, f.documents()...)
	return fetch.Checked(ctx, func(to *url.URL) error {
		if frames.Mixed(documents, to) {
			return &MixedError{Top: top, Address: to.String()}
		}
		return nil
	})
}

// under is what the shown page has under at, a point of its Document:
// the element, the frame whose document it is of, nil for the page's,
// and the point in that document.
func (s *Screen) under(top *web.Page, at geom.Vec2) (*ui.Node, *frame, geom.Vec2) {
	doc := top.Doc
	n := doc.ElementAt(at)
	var in *frame
	set := s.session().frames[top]
	for n != nil && doc.Frame(n) != nil {
		inner, ok := doc.FramePoint(n, at)
		f := set.frameOf(n)
		if !ok || f == nil || f.page == nil || f.page.Doc != doc.Frame(n) {
			return nil, in, at
		}
		in, doc, at = f, f.page.Doc, inner
		n = doc.ElementAt(at)
	}
	return n, in, at
}

// scriptsIn are the scripts of the document in shows, the page's for
// nil, for a set that may not be.
func (set *framed) scriptsIn(session *session, in *frame) *running {
	if in != nil {
		return in.run
	}
	return session.scripts
}

// postFrom sends a form the scripts of f's document sent, where its
// target says, when that document may send that frame elsewhere.
func (s *Screen) postFrom(f *frame, live *running, sent web.Submission, user bool) {
	set := f.set
	if s.pops(f, sent.Target) {
		if !user {
			set.session.said.Add(console.Line{Level: console.Warn, Frame: f.label(), Text: "Blocked opening '" +
				sent.Address + "' in a new window: a script asked for it, not the user."})
			return
		}
		s.fromPage(live.page.Address, func() { s.openTab(sent.Address, &sent) })
		return
	}
	to, may := s.target(f, sent.Target, sent.Address)
	if !may {
		return
	}
	move := frames.Navigation{Top: to == nil, Self: to == f, Descendant: to != nil && to.within(f), Activated: user,
		SameOrigin: f.origin != opaqueOrigin && f.origin == set.originIn(to), Sandbox: f.held}
	if !move.Allowed() {
		set.session.said.Add(console.Line{Level: console.Error, Frame: f.label(), Text: "Unsafe attempt to initiate " +
			"navigation for a frame from the frame with URL '" + live.page.Address + "': it may not send that frame " +
			"to '" + sent.Address + "'."})
		return
	}
	if to != nil {
		s.navigate(to, wanted{from: live.page, form: &sent, user: user})
		return
	}
	s.fromPage(live.page.Address, func() { s.posted(sent) })
}

// district is where a document keeps what it stores: its origin, under
// the site of its tab's page when it is framed by another site.
type district struct{ top, origin string }

// districtOf is the district of the document f shows, the page's for
// nil; false for a document nothing shares its origin with.
func (set *framed) districtOf(f *frame) (district, bool) {
	if f == nil {
		origin := frames.Origin(set.top.Base)
		return district{origin: origin}, origin != opaqueOrigin && origin != ""
	}
	if f.origin == opaqueOrigin || f.origin == "" {
		return district{}, false
	}
	within := district{origin: f.origin}
	if f.crossed() {
		within.top = frames.Site(set.top.Base)
	}
	return within, true
}

// broadcast gives a message of a BroadcastChannel to the other
// documents of the profile that keep their storage where its sender
// does: those of its origin, in every tab of every window, each at its
// next frame.
func (p *Profile) broadcast(from *running, within district, name string, m dom.Posted) {
	for _, s := range p.screens {
		for _, session := range s.sessions {
			for _, set := range session.frames {
				hears := func(f *frame) {
					live := set.scriptsOf(f)
					if at, ok := set.districtOf(f); ok && at == within && live != nil && live != from {
						live.rt.Broadcast(name, within.origin, m)
					}
				}
				hears(nil)
				for _, f := range set.all {
					hears(f)
				}
			}
		}
	}
}

// loading reports whether a frame of the document parent shows, the
// page's for nil, is still on its way: its document, or the scripts
// that have not told its window it loaded.
func (set *framed) loading(parent *frame) bool {
	if in := set.shownBy(parent); in != nil {
		set.adopt(parent, in)
	}
	for _, f := range set.all {
		switch {
		case f.parent != parent || f.gone:
		case !f.live:
			if rooted(f.node, f.in.Doc) {
				return true
			}
		case f.load != nil:
			return true
		case f.loads:
			return true
		}
	}
	return false
}

// runs reports whether the document f shows has scripts light runs.
func (s *Screen) runs(f *frame) bool {
	return s.scripting() && f.held.Runs() && f.page.Scripted() && f.page.Tree != nil && f.page.Tree.Activate == nil
}
