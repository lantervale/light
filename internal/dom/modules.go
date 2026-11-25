package dom

import (
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/script"
)

// How a module script is fetched: with the cookies of the page's own
// origin alone, or with those of wherever it is.
const (
	sameOrigin = "same-origin"
	include    = "include"
)

// typeJSON is the type of a module that is a JSON text.
const typeJSON = "json"

// moduleKey names a module of the page: the address it is asked at,
// and its type, "" for a script.
type moduleKey struct{ address, kind string }

// moduleScript is a module of the page: one of its module map, or what
// a script element holds.
type moduleScript struct {
	key moduleKey
	// address is where it came from, after its redirects: what it
	// imports is read from there.
	address string
	module  script.Module
	// lost is one that was not fetched, and broken why one that was is
	// not run: its syntax, or an import that names no module.
	lost   bool
	broken error
	// needs are the modules it imports in its order, and named each by
	// what it calls it.
	needs   []moduleKey
	named   map[script.Import]moduleKey
	fetched bool
	waiting []func()
}

// moduleMap is the modules of a page, each fetched once.
type moduleMap struct {
	known map[moduleKey]*moduleScript
	of    map[script.Module]*moduleScript
	// names is what the page's import maps say.
	names importMap
	// inline counts the modules script elements hold, and asked the
	// requests made.
	inline, asked int
}

func newModuleMap() moduleMap {
	return moduleMap{known: map[moduleKey]*moduleScript{}, of: map[script.Module]*moduleScript{}}
}

// moduleElement is a module script element and the module it runs.
type moduleElement struct {
	element  *html.Node
	root     *moduleScript
	external bool
}

// turn is a script that waits for the page to be read, then for those
// before it: ready is whether it has all it runs.
type turn struct {
	ready bool
	run   func()
}

// How far the page is in its load.
const (
	reading = iota
	interactive
	complete
)

// begin has a module script element of the page fetched: one marked
// async runs as soon as it is, another in its turn.
func (r *Runtime) begin(s web.Script) {
	_, external := attribute(s.Node, "src")
	if s.Async {
		r.racing++
		r.load(s.Node, s.Source, s.Address, external, func(e *moduleElement) {
			r.execute(e)
			r.racing--
			r.proceed()
		})
		return
	}
	next := &turn{}
	r.later = append(r.later, next)
	r.load(s.Node, s.Source, s.Address, external, func(e *moduleElement) {
		next.ready, next.run = true, func() { r.execute(e) }
		r.proceed()
	})
}

// proceed runs the scripts whose turn has come, then tells the page it
// is ready, and loaded once the modules that run when they can have.
func (r *Runtime) proceed() {
	if r.closed || r.starting || r.proceeding {
		return
	}
	r.proceeding = true
	defer func() { r.proceeding = false }()
	for len(r.later) > 0 && r.later[0].ready {
		next := r.later[0]
		r.later = r.later[1:]
		next.run()
		if r.closed {
			return
		}
	}
	if len(r.later) > 0 {
		return
	}
	if r.state == reading {
		r.state = interactive
		r.enter("ready", "interactive")
	}
	if r.state == interactive && r.racing == 0 && r.scripts.waited == 0 && (r.host.Loading == nil || !r.host.Loading()) &&
		!r.picturesLoading() {
		r.state = complete
		r.enter("ready", "complete")
	}
}

// picturesLoading lays the page out, which asks for the pictures it
// shows, and reports whether one is still to show.
func (r *Runtime) picturesLoading() bool {
	r.layout()
	return r.page.PicturesLoading()
}

// load fetches what a module script element runs, its own text or what
// its src names, and every module that imports; then is called once
// all of it came, or could not.
func (r *Runtime) load(element *html.Node, source, address string, external bool, then func(*moduleElement)) {
	credentials := credentialsOf(element)
	e := &moduleElement{element: element, external: external}
	if !external {
		r.loaded.inline++
		e.root = &moduleScript{key: moduleKey{address: "inline:" + strconv.Itoa(r.loaded.inline)}, address: r.page.Address,
			fetched: true}
		r.compile(e.root, source)
		r.graph(e.root, credentials, func() { then(e) })
		return
	}
	r.module(moduleKey{address: address}, credentials, func(m *moduleScript) {
		e.root = m
		r.graph(m, credentials, func() { then(e) })
	})
}

// credentialsOf is how the modules of a script element or a link are
// fetched, by its crossorigin.
func credentialsOf(element *html.Node) string {
	if mode, _ := attribute(element, "crossorigin"); strings.EqualFold(strings.TrimSpace(mode), "use-credentials") {
		return include
	}
	return sameOrigin
}

// execute runs a module script element whose modules came: one whose
// module was not fetched is told so, and what a module throws or is
// refused for is the console's.
func (r *Runtime) execute(e *moduleElement) {
	if r.closed {
		return
	}
	lost, broken := r.verdict(e.root)
	switch {
	case lost:
		r.fire(e.element, "error", nil)
		return
	case broken != nil:
		r.failed(broken)
	default:
		returned := false
		r.evaluate(e.root, func(err error) {
			if returned {
				r.waiting = append(r.waiting, func() { r.failed(err) })
				return
			}
			r.failed(err)
		})
		returned = true
	}
	if e.external {
		r.fire(e.element, "load", nil)
	}
}

// module gives the module at key, fetched and read the first time it
// is asked for: with credentials.
func (r *Runtime) module(key moduleKey, credentials string, then func(*moduleScript)) {
	if known := r.loaded.known[key]; known != nil {
		if known.fetched {
			then(known)
		} else {
			known.waiting = append(known.waiting, func() { then(known) })
		}
		return
	}
	m := &moduleScript{key: key, address: key.address}
	m.waiting = []func(){func() { then(m) }}
	r.loaded.known[key] = m
	r.fetchModule(key, credentials, func(got moduleAnswer) {
		m.fetched, m.address = true, got.address
		if got.why != "" {
			m.lost = true
			r.tell(Message{Level: "error", Text: got.why, Source: key.address})
		} else {
			r.compile(m, got.source)
		}
		waiting := m.waiting
		m.waiting = nil
		for _, told := range waiting {
			told()
		}
	})
}

// compile reads the text of a module, and finds the modules it imports.
func (r *Runtime) compile(m *moduleScript, source string) {
	var err error
	if m.key.kind == typeJSON {
		m.module, err = r.ctx.JSON(source, m.address)
	} else {
		m.module, err = r.ctx.Module(source, m.address)
	}
	if err != nil {
		m.broken = err
		return
	}
	r.loaded.of[m.module] = m
	m.named = map[script.Import]moduleKey{}
	for _, what := range m.module.Imports() {
		key, failed := r.locate(what, m.address)
		if failed != nil {
			m.broken, m.needs = failed, nil
			return
		}
		m.named[what] = key
		m.needs = append(m.needs, key)
	}
}

// locate is the module an import of the module at from names.
func (r *Runtime) locate(what script.Import, from string) (moduleKey, error) {
	address, err := r.specified(from, what.Specifier)
	switch {
	case err != nil:
		return moduleKey{}, err
	case what.Type != "" && what.Type != typeJSON:
		return moduleKey{}, &script.Error{Message: `TypeError: light imports no module of the type "` + what.Type +
			`"`, Source: from}
	}
	return moduleKey{address: address, kind: what.Type}, nil
}

// specified is the address a specifier names in the module at from,
// by the page's import maps.
func (r *Runtime) specified(from, specifier string) (string, error) {
	base, err := url.Parse(from)
	if err != nil {
		return "", &script.Error{Message: "TypeError: " + from + " is no address", Source: from}
	}
	address, err := r.loaded.names.resolve(specifier, base)
	if err != nil {
		return "", &script.Error{Message: "TypeError: " + err.Error(), Source: from}
	}
	return address, nil
}

// mapImports reads an import map of the page.
func (r *Runtime) mapImports(text string) {
	base, err := url.Parse(r.page.Address)
	if err != nil {
		return
	}
	refused, err := r.loaded.names.add(text, base)
	if err != nil {
		r.say("error", err.Error(), r.page.Address)
	}
	for _, why := range refused {
		r.say("warn", why, r.page.Address)
	}
}

// graph fetches every module root imports, and those they import; done
// is called once they all came or could not.
func (r *Runtime) graph(root *moduleScript, credentials string, done func()) {
	seen := map[moduleKey]bool{root.key: true}
	left := 1
	var came func(m *moduleScript)
	came = func(m *moduleScript) {
		for _, key := range m.needs {
			if !seen[key] {
				seen[key] = true
				left++
				r.module(key, credentials, came)
			}
		}
		if left--; left == 0 {
			done()
		}
	}
	came(root)
}

// verdict is how the modules root needs came: whether one was not
// fetched, else why the first that is not read is not.
func (r *Runtime) verdict(root *moduleScript) (bool, error) {
	seen := map[*moduleScript]bool{}
	lost, broken := false, error(nil)
	var visit func(m *moduleScript)
	visit = func(m *moduleScript) {
		if m == nil || seen[m] {
			return
		}
		seen[m] = true
		switch {
		case m.lost:
			lost = true
		case m.broken != nil && broken == nil:
			broken = m.broken
		case m.broken == nil:
			for _, key := range m.needs {
				visit(r.loaded.known[key])
			}
		}
	}
	visit(root)
	return lost, broken
}

// evaluate links a module whose modules all came, and runs it.
func (r *Runtime) evaluate(root *moduleScript, done func(error)) {
	err := root.module.Link(func(from script.Module, what script.Import) (script.Module, error) {
		if m := r.loaded.of[from]; m != nil {
			if to := r.loaded.known[m.named[what]]; to != nil && to.module != nil {
				return to.module, nil
			}
		}
		return nil, errors.New("it was not fetched")
	})
	if err != nil {
		done(err)
		return
	}
	root.module.Run(done)
}

// imported answers an import() of the script or the module at from.
func (r *Runtime) imported(from string, what script.Import, done func(script.Module, error)) {
	if r.closed {
		return
	}
	key, err := r.locate(what, from)
	if err != nil {
		done(nil, err)
		return
	}
	lost := &script.Error{Message: "TypeError: Failed to fetch dynamically imported module: " + key.address}
	r.module(key, sameOrigin, func(m *moduleScript) {
		r.graph(m, sameOrigin, func() {
			switch missing, broken := r.verdict(m); {
			case missing:
				done(nil, lost)
			case broken != nil:
				done(nil, broken)
			default:
				r.evaluate(m, func(err error) { done(m.module, err) })
			}
		})
	})
}

// preload fetches the modules the page's links say it will need.
func (r *Runtime) preload() {
	walk(r.tree.Root(), func(n *html.Node) bool {
		rel, _ := attribute(n, "rel")
		href, linked := attribute(n, "href")
		if n.DataAtom != atom.Link || !linked || !strings.EqualFold(strings.TrimSpace(rel), "modulepreload") {
			return true
		}
		if address := r.resolve(href); address != "" {
			r.module(moduleKey{address: address}, credentialsOf(n), func(*moduleScript) {})
		}
		return true
	})
}

// moduleAnswer is what asking for a module script came to: its text,
// where it came from after its redirects and its type, or why the page
// does not run it.
type moduleAnswer struct {
	source, address, media string
	why                    string
}

// fetchModule asks for the script of a module as a browser does: for
// the page's origin to read (CORS), with credentials: the host sends it
// as it sends a fetch, and says whether its origin shares the answer.
// It is the one place a module script is fetched and judged.
func (r *Runtime) fetchModule(key moduleKey, credentials string, done func(moduleAnswer)) {
	asked, err := url.Parse(key.address)
	if err != nil || (!web.Fetched(key.address) && originOf(asked) != r.origin()) {
		done(moduleAnswer{address: key.address,
			why: "Failed to load module script " + key.address + ": light fetches http:// and https:// addresses only."})
		return
	}
	r.loaded.asked--
	id := r.loaded.asked
	q := Request{Method: methodGet, Address: key.address, Script: true, Mode: string(fetch.ModeCORS),
		Credentials: credentials}
	r.requests[id] = r.request(q, func(answer Response) {
		delete(r.requests, id)
		done(r.judge(key, answer))
	})
}

// judge is the answer to a module script's request as the page takes
// it: a failure, an answer of another origin that does not share it
// and one of another type than asked are all refused.
func (r *Runtime) judge(key moduleKey, answer Response) moduleAnswer {
	got := moduleAnswer{address: answer.Address}
	if got.address == "" {
		got.address = key.address
	}
	var blocked *fetch.BlockedError
	header := map[string]string{}
	for name, value := range answer.Header {
		header[strings.ToLower(name)] = value
	}
	got.media, _, _ = mime.ParseMediaType(header["content-type"])
	wanted := "JavaScript-or-Wasm"
	if key.kind == typeJSON {
		wanted = "JSON"
	}
	switch {
	case errors.As(answer.Err, &blocked):
		got.why = blocked.Console("script")
	case answer.Err != nil:
		got.why = "Failed to load module script " + key.address + ": " + answer.Err.Error() + "."
	case answer.Status < 200 || answer.Status > 299:
		got.why = "Failed to load module script " + key.address + ": the server answered " +
			strconv.Itoa(answer.Status) + " " + http.StatusText(answer.Status) + "."
	case !typed(key.kind, got.media):
		got.why = "Failed to load module script: Expected a " + wanted + " module script but the server responded " +
			`with a MIME type of "` + got.media + `". Strict MIME type checking is enforced for module scripts per ` +
			"HTML spec."
	default:
		got.source = moduleText(answer)
	}
	return got
}

// moduleText is the text of a module script's answer: its bytes read
// as UTF-8 whatever charset it says, without their byte order mark.
func moduleText(answer Response) string {
	if answer.Body == nil {
		return answer.Text
	}
	return strings.ToValidUTF8(strings.TrimPrefix(string(answer.Body), "\ufeff"), "\ufffd")
}

// typed reports whether media is a type a module of that kind is sent
// as: one of JavaScript's, or of JSON's.
func typed(kind, media string) bool {
	if kind == typeJSON {
		return media == "application/json" || media == "text/json" || strings.HasSuffix(media, "+json")
	}
	return web.JavaScriptType(media)
}
