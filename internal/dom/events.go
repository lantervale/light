package dom

import (
	"errors"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
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

// Click tells the page's scripts the pointer was pressed and released
// on n, at a place of the page's view. A link under it is then followed
// only if no script prevented the click; Frame forgets it, so Click
// comes after Frame in the frame of the click.
func (r *Runtime) Click(n *ui.Node, x, y float32) bool {
	at := map[string]any{"clientX": x, "clientY": y, button: 0, "detail": 1}
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
	var pressed *ui.Node
	for at := n; at != nil; at = at.Parent() {
		kind, _ := at.Attr("type")
		switch tag := r.tagOf(at); {
		case pressed == nil && tag == button && (kind == "" || kind == submit),
			pressed == nil && tag == input && (kind == submit || kind == "image"):
			pressed = at
		case tag == "form" && pressed != nil:
			return at, pressed
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
	if err := r.tree.Send(form, submitter); err != nil {
		r.unsent(err)
	}
}

func (r *Runtime) unsent(err error) {
	r.say("warn", "A form was not sent: "+err.Error()+".", "")
}

// submitForm is a form sent by a script: the tab opens where it leads.
func (r *Runtime) submitForm(form *ui.Node, a arguments) (any, error) {
	var submitter *ui.Node
	if pressed := r.nodes[int(a.number(1))]; pressed != nil {
		submitter = r.tree.Make(pressed)
	}
	to, err := r.tree.Submission(form, submitter)
	switch {
	case err != nil:
		r.unsent(err)
	case r.host.Navigate != nil:
		r.host.Navigate(to, false)
	}
	return nothing()
}

// heard is what ui told a field's handlers, told to scripts at the
// next frame. A submit is Enter pressed in the field: scripts hear the
// key, then, unless they prevented it, the submit of the field's form
// when it has one.
func (r *Runtime) heard(field *ui.Node, event string) {
	r.waiting = append(r.waiting, func() {
		if event != submit {
			r.Fire(field, event, nil)
			return
		}
		enter := map[string]any{"key": "Enter", "code": "Enter"}
		prevented := r.Fire(field, "keydown", enter)
		r.Fire(field, "keyup", enter)
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
	if to := r.resolve(href); to != "" && !strings.HasPrefix(strings.TrimSpace(href), "#") {
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
	case "module":
		r.say("warn", "A script was not run: it is a module, which light does not read.", r.page.Address)
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

// methodGet is the method of the requests light sends for a script.
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

// fetch asks for what a script's fetch or XMLHttpRequest names. light
// sends nothing for scripts but a GET, and gives them an answer of
// another site only when that site allows it.
func (r *Runtime) fetch(a arguments) (any, error) {
	id, method, address := int(a.number(0)), a.text(1), a.text(2)
	fail := func(why string) {
		r.waiting = append(r.waiting, func() { r.enter("answered", id, why, 0, "", "", map[string]string{}, "") })
	}
	source, line, _ := where(a.text(4))
	to, err := url.Parse(address)
	switch {
	case err != nil || (to.Scheme != "http" && to.Scheme != "https" && originOf(to) != r.origin()):
		fail("light fetches http:// and https:// addresses only")
		return nothing()
	case method != methodGet && method != "HEAD":
		r.say("warn", "light sends nothing but a GET for a script: "+method+" "+address+" was not sent.", "")
		if r.host.Refused != nil {
			r.host.Refused(Request{Method: method, Address: address, Source: source, Line: line})
		}
		fail("light sends no " + method)
		return nothing()
	}
	header, _ := a[3].(map[string]any)
	q := Request{Method: method, Address: address, Header: map[string]string{}, Source: source, Line: line}
	for name, value := range header {
		q.Header[name], _ = value.(string)
	}
	r.requests[id] = r.request(q, func(answer Response) {
		delete(r.requests, id)
		r.answer(id, to, answer)
	})
	return nothing()
}

// answer gives a script what its request came to.
func (r *Runtime) answer(id int, asked *url.URL, answer Response) {
	header := map[string]string{}
	for name, value := range answer.Header {
		header[strings.ToLower(name)] = value
	}
	why := ""
	switch {
	case answer.Err != nil:
		why = answer.Err.Error()
		r.say("error", "Failed to load "+asked.String()+": "+failure(answer), "")
	case !r.shared(asked, answer.Address, header):
		why = "the answer is another site's, which does not share it (CORS)"
		r.say("error", "Access to "+asked.String()+" from "+r.origin()+" was blocked: "+why+".", "")
	}
	if why != "" {
		r.enter("answered", id, why, 0, "", "", map[string]string{}, "")
		return
	}
	if answer.Status >= 400 {
		r.say("error", "Failed to load "+asked.String()+": "+failure(answer), "")
	}
	r.enter("answered", id, "", answer.Status, answer.StatusText, answer.Address, header, answer.Text)
}

func (r *Runtime) origin() string {
	u, err := url.Parse(r.page.Address)
	if err != nil {
		return "null"
	}
	return originOf(u)
}

// shared reports whether an answer is the page's site's, or another
// site's that allows the page to read it.
func (r *Runtime) shared(asked *url.URL, from string, header map[string]string) bool {
	own := r.origin()
	if final, err := url.Parse(from); err == nil && from != "" {
		asked = final
	}
	if originOf(asked) == own {
		return true
	}
	allowed := strings.TrimSpace(header["access-control-allow-origin"])
	return allowed == "*" || allowed == own
}

func (r *Runtime) abort(a arguments) (any, error) {
	id := int(a.number(0))
	if cancel, asked := r.requests[id]; asked {
		delete(r.requests, id)
		cancel()
	}
	return nothing()
}
