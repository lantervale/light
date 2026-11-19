//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/netlog"
)

const saying = `<script>window.order = []; const say = (what) => {
	order.push(what);
	document.getElementById("said").textContent = order.join(" ");
};</script>`

func TestAModuleIsAskedForWithThePagesCredentialsOnlyAtItsOrigin(t *testing.T) {
	var mu sync.Mutex
	asked := map[string]http.Header{}
	n := names(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked[r.Host+r.URL.Path] = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "text/javascript")
		switch r.Host + r.URL.Path {
		case "cdn.site.test/":
			w.Header().Set("Set-Cookie", "cdn=1; Path=/")
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<title>CDN</title><p>cdn</p>`))
		case "www.site.test/dir/page":
			w.Header().Set("Set-Cookie", "site=1; Path=/")
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<title>Modules</title><p id="said"></p>` + saying + `
				<script type="module" src="/own.js"></script>
				<script type="module" src="http://cdn.site.test/anonymous.js"></script>
				<script type="module" crossorigin="use-credentials" src="http://cdn.site.test/credentialed.js"></script>
				<script type="module" src="http://cdn.site.test/kept.js"></script>
				<script type="module">say("done")</script>`))
		case "www.site.test/own.js":
			_, _ = w.Write([]byte(`say("own")`))
		case "cdn.site.test/anonymous.js":
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Set-Cookie", "anonymous=1; Path=/")
			_, _ = w.Write([]byte(`import "./dep.js"; say("anonymous")`))
		case "cdn.site.test/dep.js":
			w.Header().Set("Access-Control-Allow-Origin", "http://www.site.test")
			_, _ = w.Write([]byte(`say("dep")`))
		case "cdn.site.test/credentialed.js":
			w.Header().Set("Access-Control-Allow-Origin", "http://www.site.test")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			_, _ = w.Write([]byte(`say("credentialed")`))
		case "cdn.site.test/kept.js":
			_, _ = w.Write([]byte(`say("kept")`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
	s.visit("http://cdn.site.test/")
	s.visit("http://www.site.test/dir/page")
	s.until("the modules ran", func() bool { return strings.HasSuffix(s.pageText("said"), "done") })
	if got, want := s.pageText("said"), "own dep anonymous credentialed done"; got != want {
		t.Errorf("the modules ran as %q, want %q", got, want)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, c := range []struct{ to, cookie, origin, referer string }{
		{"www.site.test/own.js", "site=1", "", "http://www.site.test/dir/page"},
		{"cdn.site.test/anonymous.js", "", "http://www.site.test", "http://www.site.test/"},
		{"cdn.site.test/dep.js", "", "http://www.site.test", "http://www.site.test/"},
		{"cdn.site.test/credentialed.js", "cdn=1", "http://www.site.test", "http://www.site.test/"},
		{"cdn.site.test/kept.js", "", "http://www.site.test", "http://www.site.test/"},
	} {
		h := asked[c.to]
		if h == nil {
			t.Errorf("%s was not asked for", c.to)
			continue
		}
		if h.Get("Cookie") != c.cookie || h.Get("Origin") != c.origin || h.Get("Referer") != c.referer {
			t.Errorf("%s was asked for with the cookies %q, the origin %q and the referrer %q; want %q, %q and %q",
				c.to, h.Get("Cookie"), h.Get("Origin"), h.Get("Referer"), c.cookie, c.origin, c.referer)
		}
	}
	want := "error: Access to script at 'http://cdn.site.test/kept.js' from origin 'http://www.site.test' has been " +
		"blocked by CORS policy: No 'Access-Control-Allow-Origin' header is present on the requested resource."
	if got := s.said(); got != want {
		t.Errorf("the console says %q, want %q", got, want)
	}
}

func TestModulesAreListedAsScriptsAndTheirErrorsSayWhere(t *testing.T) {
	srv, seen := answers(t, map[string]answer{
		"/": typed("text/html", `<title>Modules</title><p id="said"></p>`+saying+`
			<script type="module" src="/app.js"></script><script type="module">say("after")</script>`),
		"/app.js": typed("text/javascript", `import { n } from "./lib/dep.js"; say("app" + n);
			import("./lib/chunk.js");`),
		"/lib/dep.js": typed("text/javascript", "export const n = 1;\n\n"+
			"setTimeout(() => { throw new Error('later') });"),
		"/lib/chunk.js": typed("text/javascript", "say('chunk');\nnull.chunk;"),
	})
	s := show(t)
	s.toggleTools()
	s.visit(srv.URL + "/")
	s.until("the modules ran", func() bool { return s.pageText("said") == "app1 after chunk" })
	s.until("their errors were said", func() bool { return len(s.session().said.Lines()) == 2 })
	listed := map[string]netlog.Row{}
	for _, r := range s.session().log.Rows() {
		listed[strings.TrimPrefix(r.Address, srv.URL)] = r
	}
	for _, path := range []string{"/app.js", "/lib/dep.js", "/lib/chunk.js"} {
		if r := listed[path]; r.Type != netlog.Script || r.Status != http.StatusOK || r.Initiator != "script" ||
			seen.count(path) != 1 {
			t.Errorf("%s is listed as %s by %q, answered %d, and was asked for %d times", path, r.Type, r.Initiator,
				r.Status, seen.count(path))
		}
	}
	var where []string
	for _, l := range s.session().said.Lines() {
		if l.Level != console.Error {
			t.Errorf("the console says %s: %s", l.Level, l.Text)
		}
		where = append(where, l.Where()+" "+l.Text)
	}
	if got, want := strings.Join(where, " | "), "dep.js:3 Uncaught Error: later | chunk.js:2 Uncaught (in promise) "+
		"TypeError: Cannot read properties of null (reading 'chunk')"; got != want {
		t.Errorf("the console says %q, want %q", got, want)
	}
	if value, err := s.evaluate("order.length + 1"); err != nil || value != "4" {
		t.Errorf("the console's line gave %q, %v", value, err)
	}
	s.clean()
}

func TestLeavingAPageWhileItsModulesComeEndsThem(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	n := names(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<title>Slow</title><p id="said"></p>` + saying + `
				<script type="module" src="/app.js"></script>`))
		case "/app.js":
			w.Header().Set("Content-Type", "text/javascript")
			_, _ = w.Write([]byte(`import "./slow.js"; say("app")`))
		case "/slow.js":
			select {
			case <-release:
			case <-r.Context().Done():
			}
			w.Header().Set("Content-Type", "text/javascript")
			_, _ = w.Write([]byte(`say("slow")`))
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<title>Next</title><p id="said">next</p>`))
		}
	})
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
	s.visit("http://site.test/")
	s.until("the slow module was asked for", func() bool { return n.came("site.test/slow.js") })
	first := s.session().scripts
	if first == nil || s.pageText("said") != "" {
		t.Fatalf("the page runs no script, or ran its modules before they came")
	}
	s.visit("http://site.test/next")
	s.frame()
	if !first.rt.Closed() {
		t.Fatal("the page left still runs its scripts")
	}
	once.Do(func() { close(release) })
	for range 20 {
		s.frame()
	}
	if got := s.pageText("said"); got != "next" || s.said() != "" {
		t.Errorf("after the page was left the shown one says %q and the console %q", got, s.said())
	}
}
