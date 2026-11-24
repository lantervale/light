package dom

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/fetch"
)

// activation is the last click scripts were told of: what was clicked,
// whether a script prevented what the click does, and whether one
// held the form it submits.
type activation struct {
	target          *ui.Node
	prevented, held bool
}

// Fire tells the page's scripts of an event at n, the window for nil:
// one of the types a browser fires, with what it carries (key, code,
// clientX…). It reports whether a script prevented what the event does.
func (r *Runtime) Fire(n *ui.Node, kind string, init map[string]any) bool {
	prevented := false
	r.whenFree(func() { prevented = r.fired(n, kind, init) })
	return prevented
}

func (r *Runtime) fired(n *ui.Node, kind string, init map[string]any) bool {
	if n == nil {
		return r.fire(nil, kind, init)
	}
	if of := r.nodeOf(n); of != nil {
		return r.fire(of, kind, init)
	}
	return false
}

// fire tells the page's scripts of an event at a node of the tree, the
// window for nil.
func (r *Runtime) fire(n *html.Node, kind string, init map[string]any) bool {
	var target any = "window"
	if n != nil {
		target = r.ref(n)
	}
	if init == nil {
		init = map[string]any{}
	}
	prevented, _ := r.enter("event", target, kind, init).(bool)
	return prevented
}

// Keys are the keys held with a click.
type Keys struct{ Ctrl, Shift, Alt, Meta bool }

// Click tells the page's scripts the pointer was pressed and released
// on n, at a place of the page's view, with keys held. A link under it
// is then followed only if no script prevented the click; Frame forgets
// it, so Click comes after Frame in the frame of the click.
func (r *Runtime) Click(n *ui.Node, x, y float32, held Keys) bool {
	prevented := false
	r.whenFree(func() { prevented = r.clicked(n, x, y, held) })
	return prevented
}

func (r *Runtime) clicked(n *ui.Node, x, y float32, held Keys) bool {
	at := map[string]any{"clientX": x, "clientY": y, button: 0, "detail": 1, "ctrlKey": held.Ctrl,
		"shiftKey": held.Shift, "altKey": held.Alt, "metaKey": held.Meta}
	r.Fire(n, "mousedown", at)
	r.Fire(n, "mouseup", at)
	prevented := r.Fire(n, "click", at)
	r.activation = activation{target: n, prevented: prevented}
	if form, pressed := r.submitted(n); form != nil && !prevented {
		r.activation.held = r.Fire(form, submit, r.by(pressed))
	}
	return prevented
}

// submitted is the form a click on n submits and the button of it that
// was pressed: n is in one that submits, nil otherwise.
func (r *Runtime) submitted(n *ui.Node) (*ui.Node, *ui.Node) {
	for at := n; at != nil; at = at.Parent() {
		if form := web.SubmittedBy(at); form != nil {
			return form, at
		}
	}
	return nil, nil
}

// keyboard is what a click made without the pointer carries.
func keyboard() map[string]any { return map[string]any{"detail": 0} }

// by is what a submit event carries: the button that was pressed.
func (r *Runtime) by(submitter *ui.Node) map[string]any {
	if submitter == nil {
		return nil
	}
	return map[string]any{"submitter": r.refOf(submitter)}
}

// submit is asked to send a form whose submitter was activated. A
// click scripts were told of already sends it unless they prevented
// the click or the submit; anything else (the keyboard) is told to
// them first, at the next frame.
func (r *Runtime) submit(form, submitter *ui.Node) {
	if !r.asking.script && r.withheld() {
		r.asking.queued = append(r.asking.queued, func() { r.submit(form, submitter) })
		return
	}
	if r.closed {
		r.send(form, submitter)
		return
	}
	if clicked := r.activation; clicked.target != nil && inside(submitter, clicked.target) {
		if !clicked.prevented && !clicked.held {
			r.send(form, submitter)
		}
		return
	}
	r.waiting = append(r.waiting, func() {
		if !r.Fire(submitter, "click", keyboard()) && !r.Fire(form, submit, r.by(submitter)) {
			r.send(form, submitter)
		}
	})
}

// send sends form as the user asked, and tells the console of one
// light does not send.
func (r *Runtime) send(form, submitter *ui.Node) {
	if !r.sends("", true) {
		return
	}
	if err := r.tree.Send(form, submitter); err != nil {
		r.unsent(err)
	}
}

func (r *Runtime) unsent(err error) {
	r.say("warn", "A form was not sent: "+err.Error()+".", "")
}

// submitForm is a form sent by a script: the tab opens where it leads,
// or is sent there with what a POST carries.
func (r *Runtime) submitForm(form *ui.Node, a arguments) (any, error) {
	var submitter *ui.Node
	if pressed := r.nodes[int(a.number(1))]; pressed != nil {
		submitter = r.tree.Make(pressed)
	}
	sent, err := r.tree.Submission(form, submitter)
	switch {
	case err != nil:
		r.unsent(err)
	case !r.sends(sent.Address, true):
	case sent.Form != nil && r.host.Post != nil:
		r.host.Post(sent)
	case sent.Form == nil && r.host.Navigate != nil:
		r.host.Navigate(sent.Address, false)
	}
	return nothing()
}

// heard is what ui told a field's handlers, told to scripts at the
// next frame. A submit is Enter pressed in the field: scripts hear the
// key, then, unless they prevented it, the submit of the field's form
// when it has one.
func (r *Runtime) heard(field *ui.Node, event string) {
	if event == "input" && r.breaking == field {
		r.breaking = nil
		r.waiting = append(r.waiting, func() {
			r.Fire(field, event, map[string]any{"inputType": insertLineBreak})
			r.Fire(field, "keyup", enter())
		})
		return
	}
	r.waiting = append(r.waiting, func() {
		if event != submit {
			r.Fire(field, event, nil)
			return
		}
		prevented := r.Fire(field, "keydown", enter())
		r.Fire(field, "keyup", enter())
		if prevented {
			return
		}
		form := web.FormOf(field)
		if form == nil {
			r.Fire(field, submit, nil)
			return
		}
		pressed := web.Submitter(form)
		if !r.Fire(form, submit, r.by(pressed)) {
			r.send(form, pressed)
		}
	})
}

// breakLine is Enter pressed in a textarea, told to scripts before ui
// breaks its line, as a browser tells them: keydown, keypress, then
// beforeinput, each once no script prevented the one before. It reports
// whether one did; the keyup comes after the input of the line break,
// at the next frame.
func (r *Runtime) breakLine(field *ui.Node) bool {
	prevented := false
	r.whenFree(func() {
		pressed := enter()
		pressed["charCode"] = '\r'
		prevented = r.fired(field, "keydown", enter()) || r.fired(field, "keypress", pressed) ||
			r.fired(field, "beforeinput", map[string]any{"inputType": insertLineBreak})
	})
	if !prevented {
		r.breaking = field
		return false
	}
	r.waiting = append(r.waiting, func() { r.Fire(field, "keyup", enter()) })
	return true
}

// insertLineBreak is the inputType of a line Enter breaks.
const insertLineBreak = "insertLineBreak"

// enter is what the events of Enter carry.
func enter() map[string]any { return map[string]any{"key": "Enter", "code": "Enter"} }

// Key tells scripts, at the next frame, of a key pressed and released
// while the page had the keyboard: it goes to the element that has the
// focus, the body with none.
func (r *Runtime) Key(key, code string) {
	r.waiting = append(r.waiting, func() {
		target := r.holder()
		if target == nil {
			target = r.doc.Body()
		}
		init := map[string]any{"key": key, "code": code}
		r.Fire(target, "keydown", init)
		r.Fire(target, "keyup", init)
	})
}

// activate is asked to follow a link that was activated. A click
// scripts were told of already follows it unless they prevented it;
// anything else (the keyboard) is told to them first, at the next
// frame.
func (r *Runtime) activate(a *ui.Node, follow func()) {
	if !r.asking.script && r.withheld() {
		r.asking.queued = append(r.asking.queued, func() { r.activate(a, follow) })
		return
	}
	if r.closed {
		follow()
		return
	}
	if clicked := r.activation; clicked.target != nil && inside(a, clicked.target) {
		if !clicked.prevented {
			follow()
		}
		return
	}
	r.waiting = append(r.waiting, func() {
		if !r.Fire(a, "click", keyboard()) {
			follow()
		}
	})
}

// activated is what a click made by a script does: a link is followed.
func (r *Runtime) activated(n *ui.Node, _ arguments) (any, error) {
	if r.tagOf(n) != "a" || r.host.Navigate == nil {
		return nothing()
	}
	href, _ := n.Attr("href")
	to := r.resolve(href)
	if to != "" && strings.HasSuffix(strings.TrimSpace(href), "#") {
		to += "#"
	}
	if to != "" && r.sends(to, false) {
		r.host.Navigate(to, false)
	}
	return nothing()
}

// resolve is a reference of the page as an address, "" for none.
func (r *Runtime) resolve(reference string) string {
	base, err := url.Parse(r.page.Address)
	if err != nil {
		return ""
	}
	to, err := base.Parse(strings.TrimSpace(reference))
	if err != nil {
		return ""
	}
	return to.String()
}

// watchFocus tells scripts of the focus moving from an element of the
// page to another.
func (r *Runtime) watchFocus() {
	now := r.holder()
	if now == r.focused {
		return
	}
	was := r.focused
	r.focused = now
	if was != nil && r.ids[r.tree.Of(was)] != 0 {
		r.Fire(was, "blur", nil)
		r.Fire(was, "focusout", nil)
	}
	if now != nil {
		r.Fire(now, "focus", nil)
		r.Fire(now, "focusin", nil)
	}
}

// added is a script element a script put in the page: it runs at the
// next frame, once fetched when it names a source.
func (r *Runtime) added(element *html.Node) {
	kind, _ := attribute(element, "type")
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "text/javascript", "application/javascript":
		if _, fallback := attribute(element, "nomodule"); fallback {
			return
		}
	case "module":
		source, external := attribute(element, "src")
		r.load(element, textOf(element), r.resolve(source), external, r.execute)
		return
	case "importmap":
		r.mapImports(textOf(element))
		return
	default:
		return
	}
	source, external := attribute(element, "src")
	if !external {
		r.waiting = append(r.waiting, func() { r.run(textOf(element), r.page.Address, element) })
		return
	}
	address := r.resolve(source)
	r.request(Request{Method: methodGet, Address: address, Script: true}, func(answer Response) {
		if answer.Err != nil || answer.Status < 200 || answer.Status > 299 {
			r.say("warn", "A script was not run: "+failure(answer), address)
			r.fire(element, "error", nil)
			return
		}
		r.run(answer.Text, answer.Address, element)
		r.fire(element, "load", nil)
	})
}

func failure(answer Response) string {
	if answer.Err != nil {
		return answer.Err.Error() + "."
	}
	return "the server answered " + answer.StatusText + "."
}

// methodGet is the method a script element's source is asked for with.
const methodGet = "GET"

var errNoFetch = errors.New("light fetches nothing for this page's scripts")

// request asks the browser for r, and has done run at the frame after
// the answer came. What it returns forgets the request.
func (r *Runtime) request(q Request, done func(Response)) func() {
	if r.host.Fetch == nil {
		r.waiting = append(r.waiting, func() { done(Response{Err: errNoFetch}) })
		return func() {}
	}
	live := true
	stop := r.host.Fetch(q, func(answer Response) {
		r.land(func() {
			if live {
				done(answer)
			}
		})
	})
	return func() {
		live = false
		if stop != nil {
			stop()
		}
	}
}

// maxKept is the most a request that outlives its page carries.
const maxKept = 64 << 10

// fetch asks for what a script's fetch, XMLHttpRequest or sendBeacon
// names: the browser sends it by the rules of the web, and says what
// the script may see of its answer.
func (r *Runtime) fetch(a arguments) (any, error) {
	id := int(a.number(0))
	q, api := asked(a)
	fail := func(why string) {
		r.waiting = append(r.waiting, func() { r.enter("answered", id, why, 0, "", "", map[string]string{}, 0, "", false) })
	}
	to, err := url.Parse(q.Address)
	switch {
	case err != nil || (!onWeb(to) && originOf(to) != r.origin()):
		fail("light fetches http:// and https:// addresses only")
		return nothing()
	case q.Keepalive && len(q.Body) > maxKept:
		fail("a request that outlives its page carries no more than 64 KiB")
		return nothing()
	}
	r.requests[id] = r.request(q, func(answer Response) {
		delete(r.requests, id)
		delete(r.kept, id)
		r.answer(id, api, to, answer)
	})
	if q.Keepalive {
		r.kept[id] = true
	}
	return nothing()
}

// onWeb reports whether u is an http or an https address.
func onWeb(u *url.URL) bool { return u.Scheme == "http" || u.Scheme == "https" }

// asked is the request a script described, and what of the page's API
// it asked through: "fetch" or "XMLHttpRequest".
func asked(a arguments) (Request, string) {
	said, _ := a[1].(map[string]any)
	text := func(name string) string {
		value, _ := said[name].(string)
		return value
	}
	q := Request{Method: text("method"), Address: text("url"), Header: map[string]string{}, Mode: text("mode"),
		Credentials: text("credentials"), Redirect: text("redirect")}
	q.Keepalive, _ = said["keepalive"].(bool)
	q.Source, q.Line, _ = where(a.text(2))
	header, _ := said["headers"].(map[string]any)
	for name, value := range header {
		q.Header[name], _ = value.(string)
	}
	if len(a) < 4 {
		return q, text("api")
	}
	if carried, has := a[3].([]byte); has {
		q.Body = append([]byte{}, carried...)
	}
	return q, text("api")
}

// answer gives a script what its request came to.
func (r *Runtime) answer(id int, api string, asked *url.URL, answer Response) {
	if answer.Err == nil && answer.Kind == "" {
		answer = r.vetted(asked, answer)
	}
	var blocked *fetch.BlockedError
	switch {
	case errors.As(answer.Err, &blocked):
		r.say("error", blocked.Console(api), "")
	case answer.Err != nil:
		r.say("error", "Failed to load "+asked.String()+": "+failure(answer), "")
	}
	if answer.Err != nil {
		r.enter("answered", id, answer.Err.Error(), 0, "", "", map[string]string{}, 0, "", false)
		return
	}
	if kind := fetch.Kind(answer.Kind); kind == fetch.Opaque || kind == fetch.OpaqueRedirect {
		answer = Response{Kind: answer.Kind}
	}
	header := map[string]string{}
	for name, value := range answer.Header {
		header[strings.ToLower(name)] = value
	}
	if answer.Status >= http.StatusBadRequest {
		r.say("error", "Failed to load "+asked.String()+": "+failure(answer), "")
	}
	r.answering = answer.Body
	if r.answering == nil {
		r.answering = []byte(answer.Text)
	}
	r.enter("answered", id, "", answer.Status, answer.StatusText, answer.Address, header, len(r.answering), answer.Kind,
		answer.Redirected)
	r.answering = nil
}

// located is the address of the page's document, that of the page it
// is written in for one that has none of its own.
func (r *Runtime) located() *url.URL {
	u, err := url.Parse(r.page.Address)
	if err != nil || r.page.Address == "about:srcdoc" {
		return r.page.Base
	}
	return u
}

// vetted is an answer whose host did not say what the script may see
// of it, as a fetch that names no mode sees it: its own origin's whole,
// another's only when that origin shares it, and then the headers it
// exposes.
func (r *Runtime) vetted(asked *url.URL, answer Response) Response {
	at, _ := url.Parse(r.page.Address)
	policy := fetch.Policy{Origin: fetch.OriginOf(at), Mode: fetch.ModeCORS, Credentials: fetch.CredentialsSameOrigin}
	if r.host.Origin != nil || r.host.Sandboxed.Opaque {
		policy.Origin = r.origin()
	}
	from := asked
	if final, err := url.Parse(answer.Address); err == nil && answer.Address != "" {
		from = final
	}
	if originOf(from) == r.origin() && !onWeb(from) {
		answer.Kind = string(fetch.Basic)
		return answer
	}
	header := http.Header{}
	for name, value := range answer.Header {
		header.Set(name, value)
	}
	kind, err := policy.Decide(from, !policy.Same(asked), header)
	var blocked *fetch.BlockedError
	if errors.As(err, &blocked) {
		blocked.Address, blocked.Origin = from.String(), policy.Origin
	}
	if err != nil {
		return Response{Err: err}
	}
	answer.Kind, answer.Header = string(kind), map[string]string{}
	for name := range policy.Visible(kind, header) {
		answer.Header[name] = header.Get(name)
	}
	answer.Redirected = answer.Redirected || from.String() != asked.String()
	return answer
}

func (r *Runtime) abort(a arguments) (any, error) {
	id := int(a.number(0))
	if cancel, asked := r.requests[id]; asked {
		delete(r.requests, id)
		cancel()
	}
	return nothing()
}
