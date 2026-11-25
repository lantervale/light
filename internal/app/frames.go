package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/content"
	"github.com/lantervale/light/pkg/cookies"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/frames"
	"github.com/lantervale/light/pkg/identity"
	"github.com/lantervale/light/pkg/localnet"
	"github.com/lantervale/light/pkg/netlog"
	"github.com/lantervale/light/pkg/tabs"
)

// byFrame is who asks for a frame's document, as the network panel
// names it; what that document asks for is asked by "frame".
const byFrame = "iframe"

// idleFrames is how many iframe elements that are in no document a page
// is kept track of: those its scripts made and have not placed yet.
const idleFrames = 4 * frames.MaxFrames

// frame is an iframe element of a tab's page, or of the document
// another frame shows.
type frame struct {
	set  *framed
	node *ui.Node
	// in is the page whose Document node is of, and parent the frame
	// that shows it, nil for the tab's page.
	in     *web.Page
	parent *frame
	// live is whether node is in its document, and admitted whether the
	// tab had room for it; key is what its attributes asked it to show
	// when they were last followed.
	live, admitted bool
	key            string
	sandbox        frames.Sandbox
	// warned is the sandbox attribute the console was last warned of.
	warned string
	// page is the document it shows, nil for none, and load the one on
	// its way.
	page *web.Page
	load *load
	gone bool
	// id names the frame's window to the scripts of its tab. held is
	// the sandbox the document it shows was loaded under and origin that
	// document's, "null" for one nothing shares; run is its scripts, nil
	// for a document that has none to run (framescripts.go).
	id     int
	held   frames.Sandbox
	origin string
	run    *running
	// loads is whether its element is still to hear load: once the
	// window of the document it shows did.
	loads bool
	// at is the address the tab's history has it at, "" for what its
	// element asks, and asked the src it last followed. home is the
	// address its document came as, first the one it came from, and
	// pushed those it gave itself since (framehistory.go).
	at, asked   string
	home, first string
	pushed      map[string]bool
	// ord is its number among the frames its document ever had, and
	// born how many the document it shows had.
	ord, born int
	// began is what its element asked for when it came into its page:
	// where the tab's history has it before its first step.
	began wanted
}

func (f *frame) depth() int {
	depth := 0
	for at := f; at != nil; at = at.parent {
		depth++
	}
	return depth
}

// above are the documents f is in, the nearest first and the tab's
// page last.
func (f *frame) above() []frames.Ancestor {
	var out []frames.Ancestor
	for at := f; at != nil; at = at.parent {
		out = append(out, frames.Ancestor{URL: at.in.Base, Opaque: at.parent != nil && at.parent.sandbox.Opaque()})
	}
	return out
}

// within reports whether f is of, or under, the frame of.
func (f *frame) within(of *frame) bool {
	for at := f; at != nil; at = at.parent {
		if at == of {
			return true
		}
	}
	return false
}

// framed are the frames of one page of a tab, those of its frames'
// documents included.
type framed struct {
	tab     *tabs.Tab
	session *session
	top     *web.Page
	all     []*frame
	// last is the id of the window made last, born how many frames the
	// page ever had, and mail what the windows
	// posted each other that has not come yet (framescripts.go).
	last int
	born int
	mail []letter
	// ports are the ends of the channels between its documents, by
	// their number, and lastPort the number given last (frameports.go).
	ports    map[int]*end
	lastPort int
	// kin are the documents of the page that share an engine, by what
	// names them (framekin.go).
	kin map[string]*dom.Kin
}

// frames are the frames of a page of the tab, made when first asked.
func (s *Screen) framesOf(tab *tabs.Tab, top *web.Page) *framed {
	session := s.sessionOf(tab)
	if session.frames[top] == nil {
		session.frames[top] = &framed{tab: tab, session: session, top: top, last: topWindow}
	}
	return session.frames[top]
}

// adopt takes the iframe elements built for the Document of in since it
// last did.
func (set *framed) adopt(parent *frame, in *web.Page) {
	born := &set.born
	if parent != nil {
		born = &parent.born
	}
	for _, n := range in.Frames.Take() {
		set.last++
		set.all = append(set.all, &frame{set: set, node: n, in: in, parent: parent, id: set.last, origin: opaqueOrigin,
			ord: *born})
		*born++
	}
}

// frameList are the frames of the page, none for a page that has no
// set of them yet.
func (set *framed) frameList() []*frame {
	if set == nil {
		return nil
	}
	return set.all
}

// count is how many frames the tab holds beside f.
func (set *framed) count(f *frame) int {
	count := 0
	for _, other := range set.all {
		if other != f && other.admitted {
			count++
		}
	}
	return count
}

// of is the frame of the iframe element n, nil for none.
func (set *framed) of(n *ui.Node) *frame {
	for _, f := range set.all {
		if f.node == n {
			return f
		}
	}
	return nil
}

// named is the frame whose element has that name, nil for none.
func (set *framed) named(name string) *frame {
	for _, f := range set.all {
		if has, _ := f.node.Attr("name"); f.live && has == name {
			return f
		}
	}
	return nil
}

// stop ends the load of f that is on its way.
func (f *frame) stop() {
	if f.load != nil {
		f.load.cancel()
		f.load = nil
	}
}

// blank has f show nothing: its load is stopped, and the frames of the
// document it showed go with it.
func (f *frame) blank() {
	f.stop()
	if f.page != nil {
		for _, child := range slices.Clone(f.set.all) {
			if child.parent == f {
				child.drop()
			}
		}
	}
	f.quit()
	f.page, f.origin = nil, opaqueOrigin
	if f.in.Doc != nil {
		f.in.Doc.SetFrame(f.node, nil)
	}
}

// drop is blank for a frame that is no more.
func (f *frame) drop() {
	f.blank()
	f.gone, f.live, f.admitted = true, false, false
	f.set.all = slices.DeleteFunc(f.set.all, func(other *frame) bool { return other == f })
}

// release lets go of every frame of the page.
func (set *framed) release() {
	for _, f := range slices.Clone(set.all) {
		if !f.gone {
			f.drop()
		}
	}
	set.mail, set.ports = nil, nil
}

// pause stops the loads on their way while the page is not the one its
// tab shows: they start again when it is.
func (set *framed) pause() {
	for _, f := range set.all {
		if f.load != nil {
			f.stop()
			f.live, f.key = false, ""
		}
	}
}

// quitFrames lets go of the frames of the pages a tab holds no more,
// and stops the loads of those it does not show.
func (s *Screen) quitFrames(tab *tabs.Tab, session *session) {
	held := map[*web.Page]bool{}
	for _, page := range session.pages {
		held[page] = true
	}
	shown := session.pages[tab.Current()]
	for page, set := range session.frames {
		switch {
		case !held[page]:
			set.release()
			delete(session.frames, page)
		case page != shown:
			set.pause()
		}
	}
}

// frames follows the iframes of the shown page of the web: those that
// came into a document are given what their attributes ask for, those
// that left one are emptied, and those whose attributes changed show
// what they ask for now.
func (s *Screen) frames() {
	tab := s.browser.Tab()
	top := s.sessionOf(tab).pages[tab.Current()]
	if top == nil || top.Doc == nil || top.Err != nil || top.Base == nil {
		return
	}
	set := s.framesOf(tab, top)
	set.adopt(nil, top)
	idle := 0
	for _, f := range slices.Clone(set.all) {
		if f.gone {
			continue
		}
		placed := rooted(f.node, f.in.Doc) && (f.parent == nil || f.parent.page == f.in)
		switch key := s.asks(f); {
		case !placed && f.live:
			f.blank()
			f.live, f.admitted, f.key = false, false, ""
		case placed && (!f.live || key != f.key):
			s.follow1(f, key)
		}
		if raised := panicOf(f.in.Doc, f.node); raised != nil && f.page != nil {
			set.session.said.Add(console.Line{Level: console.Error,
				Text: fmt.Sprintf("A frame broke light: %v. Its stack is in light's errors.log.", raised.Value)})
			s.unshown(f, f.page, "its page broke light")
		}
		if f.page != nil {
			set.adopt(f, f.page)
		}
		if !f.live {
			idle++
		}
	}
	if set.session.returned {
		set.session.returned = false
		for _, f := range slices.Clone(set.all) {
			if f.live && !f.gone && (f.parent == nil || f.parent.load == nil) {
				s.bring(f, tab.Frame(f.path()))
			}
		}
	}
	for i := 0; idle > idleFrames && i < len(set.all); i++ {
		if f := set.all[i]; !f.live {
			f.drop()
			idle--
			i--
		}
	}
}

// panicOf is the panic that emptied the frame n of doc, nil for none.
func panicOf(doc *ui.Document, n *ui.Node) *ui.FrameError {
	var raised *ui.FrameError
	if doc == nil || !errors.As(doc.FrameBroken(n), &raised) {
		return nil
	}
	return raised
}

// broken has page, the shown one, fail as one that broke light when it
// panicked in its frame: the tab then shows why, the console holds the
// panic, and the rest of light goes on.
func (s *Screen) broken(page *web.Page) {
	raised := panicOf(s.doc, s.host)
	if raised == nil || page == nil || page.Err != nil {
		return
	}
	page.Err = fmt.Errorf("the page broke light: %v", raised.Value)
	s.session().said.Add(console.Line{Level: console.Error,
		Text: fmt.Sprintf("The page broke light: %v. Its stack is in light's errors.log.", raised.Value)})
}

// asks is what the attributes of f's element, and the settings, ask it
// to show.
func (s *Screen) asks(f *frame) string {
	src, _ := f.node.Attr("src")
	inline, has := f.node.Attr("srcdoc")
	return fmt.Sprintf("%v\x00%s\x00%v\x00%s", s.config.Frames, src, has, inline)
}

// rooted reports whether n is in doc.
func rooted(n *ui.Node, doc *ui.Document) bool {
	return doc != nil && rootOf(n) == rootOf(doc.HTML())
}

func rootOf(n *ui.Node) *ui.Node {
	for n != nil && n.Parent() != nil {
		n = n.Parent()
	}
	return n
}

// wanted is what a frame is asked to show, and by whom: the address
// ref read from the base of the page from, the markup inline when
// isInline, or the answer to form; user when a click asked.
type wanted struct {
	from     *web.Page
	ref      string
	inline   string
	isInline bool
	form     *web.Submission
	user     bool
	// how is what the tab's history keeps of it: a step, unless told.
	how stepping
}

// MixedError is a frame in the clear that a page loaded over https
// asked for.
type MixedError struct{ Top, Address string }

func (e *MixedError) Error() string {
	return fmt.Sprintf("Mixed Content: The page at '%s' was loaded over HTTPS, but requested an insecure frame "+
		"'%s'. This request has been blocked; the content must be served over HTTPS.", e.Top, e.Address)
}

// navigate has f show what q asks for, loaded off the frame: nothing
// when the settings load no frame, when the tab has no room for it, or
// when the address is one light shows in no frame.
func (s *Screen) navigate(f *frame, q wanted) {
	set := f.set
	f.stop()
	box, sandboxed := f.node.Attr("sandbox")
	f.sandbox = frames.ParseSandbox(box, sandboxed)
	if f.parent != nil {
		f.sandbox = f.sandbox.Under(f.parent.sandbox)
	}
	if own := frames.ParseSandbox(box, sandboxed); own.On && own.Runs() && !own.Opaque() && box != f.warned {
		f.warned = box
		set.session.said.Add(console.Line{Level: console.Warn, Text: "An iframe which has both allow-scripts and " +
			"allow-same-origin for its sandbox attribute can escape its sandboxing."})
	}
	kind := frames.Shown(q.ref, q.isInline)
	if q.form != nil {
		kind = frames.Remote
	}
	if !s.config.Frames {
		f.blank()
		return
	}
	if err := frames.Admit(f.depth(), set.count(f)); err != nil {
		f.admitted = false
		s.leftEmpty(f, err)
		return
	}
	f.admitted = true
	if kind == frames.Blank {
		f.blank()
		s.tellFrame(f, "load")
		return
	}
	address := ""
	if kind == frames.Remote && q.form == nil {
		to, err := frames.Resolve(q.from.Base, q.ref)
		if err == nil && frames.Recursive(to, f.documents()) {
			err = frames.ErrRecursive
		}
		if err != nil {
			s.leftEmpty(f, err)
			return
		}
		address = to.String()
	}
	s.stepped(f, q, address)
	loader, links := s.frameLoader(f), s.frameLinks(f)
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout())
	ctx, trail := netlog.Trailed(s.frameAsks(ctx, f, q, loader.Frame))
	l := &load{cancel: cancel}
	f.load = l
	go func() {
		defer cancel()
		var page *web.Page
		switch {
		case q.form != nil:
			page = loader.Post(ctx, *q.form, links)
		case kind == frames.Inline:
			page = loader.Inline(ctx, q.from.Base, q.from.Space, q.inline, links)
		default:
			page = loader.Load(ctx, address, links)
		}
		s.landed <- func() { s.landFrame(f, l, page, trail.Last()) }
	}()
}

// documents are the addresses of the documents f is in, the nearest
// first and the tab's page last, nil for one with no origin.
func (f *frame) documents() []*url.URL {
	above := f.above()
	documents := make([]*url.URL, 0, len(above))
	for _, a := range above {
		if a.Opaque {
			documents = append(documents, nil)
			continue
		}
		documents = append(documents, a.URL)
	}
	return documents
}

// frameLoader is the loader of the settings as f's documents are
// loaded: with their scripts unless its sandbox runs none, and let in
// only where they do not refuse the documents f is in.
func (s *Screen) frameLoader(f *frame) *web.Loader {
	loader := *s.current()
	above, documents := f.above(), f.documents()
	loader.Scripts, loader.Zoom = loader.Scripts && f.sandbox.Runs(), 0
	loader.Frame = &web.Frame{
		Top: f.set.top.Base, Chain: documents[:len(documents)-1], Opaque: f.sandbox.Opaque(),
		NoForms: !f.sandbox.Submits(),
		Admit:   func(header http.Header, at *url.URL) error { return frames.Allowed(header, at, above) },
	}
	return &loader
}

// frameAsks is ctx for the request of a document of f that q asks for:
// a frame's navigation from the page that asked, listed in the tab's
// requests, which reaches no further than that page does, carries the
// cookies of its tab's site alone, or none without an origin, and goes
// nowhere in the clear under a page that came over https.
func (s *Screen) frameAsks(ctx context.Context, f *frame, q wanted, as *web.Frame) context.Context {
	set := f.set
	ctx = netlog.With(ctx, set.session.log, byFrame, netlog.Document)
	ctx = fetch.Asking(ctx, identity.Request{Destination: identity.Iframe, Initiator: q.from.Base, User: q.user})
	ctx = localnet.From(ctx, q.from.Space)
	if as.Opaque {
		ctx = cookies.Without(ctx)
	} else {
		ctx = cookies.Framed(ctx, as.Top, frames.Partition(as.Top, as.Chain) != "")
	}
	top, documents := set.top.Address, f.documents()
	return fetch.Checked(ctx, func(to *url.URL) error {
		if frames.Mixed(documents, to) {
			return &MixedError{Top: top, Address: to.String()}
		}
		return nil
	})
}

// frameSize is the size of f's box in the pixels its document is laid
// out in, an iframe's own before it has one.
func (s *Screen) frameSize(f *frame) (float32, float32) {
	box, laid := f.in.Doc.BoxOf(f.node)
	zoom := max(float32(s.zoomOf(f.set.session))/100, 0.01)
	if !laid || box.W <= 0 || box.H <= 0 {
		return 300, 150
	}
	return box.W / zoom, box.H / zoom
}

// leftEmpty empties f and says why in the tab's console.
func (s *Screen) leftEmpty(f *frame, why error) {
	f.blank()
	f.set.session.said.Add(console.Line{Level: console.Warn, Text: "A frame was left empty: " + why.Error() + "."})
	s.tellFrame(f, "error")
}

// tellFrame tells the scripts of the document f's element is in what
// became of f: its load or its error event.
func (s *Screen) tellFrame(f *frame, event string) {
	live := f.set.session.scripts
	if f.parent != nil {
		live = f.parent.run
	}
	if live != nil && live.page == f.in && !f.gone {
		s.later = append(s.later, func() {
			if !live.rt.Closed() {
				live.rt.Fire(f.node, event, nil)
			}
		})
	}
}

// landFrame shows in f the document its load ended with, unless it was
// stopped meanwhile. One that refuses to be framed, and one that could
// not be had, leave light's notice in the frame, a line in the console
// and, for a refusal, its request marked as blocked.
func (s *Screen) landFrame(f *frame, l *load, page *web.Page, row int) {
	if f.gone || f.load != l {
		return
	}
	f.load = nil
	session := f.set.session
	var (
		refused *frames.RefusedError
		mixed   *MixedError
		local   *localnet.BlockedError
	)
	switch {
	case errors.As(page.Err, &refused):
		session.log.Refuse(row, "refused to be framed")
		session.said.Add(console.Line{Level: console.Error, Text: refused.Error()})
		s.showIn(f, &web.Page{Address: page.Address, Base: f.in.Base, Space: f.in.Space,
			Doc: web.Notice(hostOf(page.Address) + " refused to be shown in a frame.")})
		s.tellFrame(f, "load")
	case errors.As(page.Err, &mixed):
		session.log.Block("GET", mixed.Address, byFrame, netlog.Document, "mixed content")
		session.said.Add(console.Line{Level: console.Error, Text: mixed.Error()})
		s.unshown(f, page, "it is not sent over https, as its page is")
	case errors.As(page.Err, &local):
		session.said.Add(console.Line{Level: console.Error, Text: "A frame was not loaded: " + local.Blocked()})
		s.unshown(f, page, "a page may not frame the network around this machine")
	case page.Err != nil:
		session.said.Add(console.Line{Level: console.Warn, Text: "A frame was not loaded: " + page.Err.Error() + "."})
		s.unshown(f, page, page.Err.Error())
	case page.Empty:
	case page.Kind == content.Page:
		s.showIn(f, page)
		f.origin = f.originOf(page)
		if f.loads = s.runs(f); !f.loads {
			s.tellFrame(f, "load")
		}
	case page.Kind == content.Text && page.Tree != nil:
		page.Doc = web.Text(page.Source)
		s.showIn(f, page)
		f.origin = f.originOf(page)
		s.tellFrame(f, "load")
	case page.Kind == content.Text:
		page.Doc, page.Base = web.Text(page.Source), f.in.Base
		s.showIn(f, page)
		s.tellFrame(f, "load")
	default:
		session.said.Add(console.Line{Level: console.Warn, Text: "A frame was not loaded: light shows only pages " +
			"and text in a frame, and " + page.Address + " is neither."})
		s.unshown(f, page, "light shows only pages and text in a frame")
	}
}

// unshown leaves in f light's notice that the document at page's
// address is not shown, and why.
func (s *Screen) unshown(f *frame, page *web.Page, why string) {
	s.showIn(f, &web.Page{Address: page.Address, Base: f.in.Base, Space: f.in.Space,
		Doc: web.Notice("This frame is not shown: " + why + ".")})
	s.tellFrame(f, "error")
}

// showIn has f show page in place of what it showed.
func (s *Screen) showIn(f *frame, page *web.Page) {
	f.blank()
	f.page, f.held, f.origin, f.loads = page, f.sandbox, opaqueOrigin, false
	f.home, f.first, f.pushed, f.born = f.at, page.Address, nil, 0
	f.in.Doc.SetFrame(f.node, page.Doc)
	if web.Fragment(page.Address) != "" {
		web.Reveal(page.Doc, page.Address)
	}
}

// frameLinks is what the links and the forms of f's documents do: a
// link opens in the frame its target names, f itself when it names
// none, and a form is sent there.
func (s *Screen) frameLinks(f *frame) web.Links {
	said := f.set.session.said
	return web.Links{
		Open:  func(address string) { s.openFrom(f, address, "") },
		Aimed: func(address, target string) bool { s.openFrom(f, address, target); return true },
		Over:  s.point,
		Save: func(address, name string) {
			if !f.sandbox.Saves() {
				said.Add(console.Line{Level: console.Error, Text: "Download is disallowed. The frame initiating or " +
					"instantiating the download is sandboxed, but the flag 'allow-downloads' is not set."})
				return
			}
			s.download(address, name)
		},
		Post: func(sent web.Submission) {
			if s.pops(f, sent.Target) {
				s.fromPage(f.page.Address, func() { s.openTab(sent.Address, &sent) })
			} else if to, may := s.target(f, sent.Target, sent.Address); to != nil {
				s.navigate(to, wanted{from: f.page, form: &sent, user: true})
			} else if may && s.leaves(f, sent.Address) {
				s.fromPage(f.page.Address, func() { s.posted(sent) })
			}
		},
		Unsent: f.set.session.unsent,
	}
}

// openFrom opens the address a link of f's document leads to: in a tab
// of its own when the keys held say so, else where target says.
func (s *Screen) openFrom(f *frame, address, target string) {
	if f.gone || f.page == nil {
		return
	}
	if where := openingBy(s.system, s.win.Input(), false); where != inTab {
		s.openBeside(address, where)
		return
	}
	if s.pops(f, target) {
		s.fromPage(f.page.Address, func() { s.openTab(address, nil) })
		return
	}
	if to, may := s.target(f, target, address); to != nil {
		if !s.placeFrame(to, address) {
			s.navigate(to, wanted{from: f.page, ref: address, user: true})
		}
	} else if may && s.leaves(f, address) {
		s.fromPage(f.page.Address, func() { s.Open(address) })
	}
}

// reach is the frame of that name a link or a form of f may lead to:
// a sandboxed frame reaches only those in it.
func (s *Screen) reach(f *frame, name string) *frame {
	if to := f.set.named(name); to != nil && (!f.sandbox.On || to.within(f)) {
		return to
	}
	return nil
}

// target is the frame a link or a form of f with that target leads to,
// nil for the tab itself. It leads nowhere, false, when it asks for a
// window of its own that f's sandbox keeps it from, _blank or a name
// f reaches no frame by: the console is told.
func (s *Screen) target(f *frame, target, address string) (*frame, bool) {
	switch frames.TargetOf(target) {
	case frames.Top:
		return nil, true
	case frames.Parent:
		return f.parent, true
	case frames.Named:
		if to := s.reach(f, target); to != nil {
			return to, true
		}
	case frames.Self:
		if !asksWindow(target) {
			return f, true
		}
	}
	if !f.sandbox.On || !f.sandbox.Popups {
		f.set.session.said.Add(console.Line{Level: console.Error, Text: "Blocked opening '" + address + "' in a new " +
			"window because the request was made in a sandboxed frame whose 'allow-popups' permission is not set."})
	}
	return nil, false
}

// leaves reports whether f may send its tab to address, as a link or a
// form whose target is the tab asks; the console is told when its
// sandbox keeps it from it.
func (s *Screen) leaves(f *frame, address string) bool {
	if f.sandbox.NavigatesTop(true) {
		return true
	}
	f.set.session.said.Add(console.Line{Level: console.Error, Text: "Unsafe attempt to initiate navigation for " +
		"frame with URL '" + f.set.top.Address + "' from frame with URL '" + f.page.Address + "'. The frame " +
		"attempting navigation of the top-level window is sandboxed, but the flag of 'allow-top-navigation' or " +
		"'allow-top-navigation-by-user-activation' is not set. (" + address + ")"})
	return false
}

// aimedFromTop opens address in the frame of the tab's page that a
// link of the page names as its target, and reports whether there is
// one.
func (s *Screen) aimedFromTop(tab *tabs.Tab, address, target string, form *web.Submission) bool {
	top := s.sessionOf(tab).pages[tab.Current()]
	if top == nil || frames.TargetOf(target) != frames.Named {
		return false
	}
	to := s.framesOf(tab, top).named(target)
	if to == nil {
		return false
	}
	s.navigate(to, wanted{from: top, ref: address, form: form, user: true})
	return true
}

// innermost is what the shown page of the web has under at, a point of
// its Document: the element, the Document it is of, the address that
// one's links are read from, and the iframe of the page itself it is
// in, nil for an element of the page.
func (s *Screen) innermost(top *web.Page, at geom.Vec2) (*ui.Node, *ui.Document, string, *ui.Node) {
	doc, base := top.Doc, top.Address
	n := doc.ElementAt(at)
	var outer *ui.Node
	set := s.session().frames[top]
	for n != nil && doc.Frame(n) != nil {
		inner, ok := doc.FramePoint(n, at)
		if !ok {
			break
		}
		if outer == nil {
			outer = n
		}
		if f := set.frameOf(n); f != nil && f.page != nil && f.page.Base != nil {
			base = f.page.Base.String()
		}
		doc, at = doc.Frame(n), inner
		n = doc.ElementAt(at)
	}
	return n, doc, base, outer
}

// frameOf is of for a set that may not be.
func (set *framed) frameOf(n *ui.Node) *frame {
	if set == nil {
		return nil
	}
	return set.of(n)
}
