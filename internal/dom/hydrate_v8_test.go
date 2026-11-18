//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	_ "embed"
	"strings"
	"testing"
)

// React 18.3.1's production builds (testdata/react, MIT, see its
// LICENSE): what the page under test runs, as a site serves it.
var (
	//go:embed testdata/react/react.production.min.js
	react string
	//go:embed testdata/react/react-dom.production.min.js
	reactDOM string
)

// app is a small React tree: text beside values, a counter, an SVG and
// a field under a Suspense boundary, and nothing where a child is null.
const app = `
	const e = React.createElement;
	window.recovered = [];
	function Counter() {
		const [n, setN] = React.useState(0);
		return e("button", { id: "count", onClick: () => setN(n + 1) }, "Clicked ", n, " times");
	}
	function App() {
		React.useEffect(() => { window.hydrated = true; }, []);
		return e("div", { id: "app", className: "shell" },
			e("h1", null, "Hello ", "light"),
			e(Counter),
			e(React.Suspense, { fallback: e("p", null, "…") },
				e("section", { id: "lazy" },
					e("svg", { viewBox: "0 0 10 10", width: 10 }, e("path", { d: "M0 0h10" })),
					e("span", null, "inside"))),
			e("input", { id: "field", tabIndex: 3, defaultValue: "v" }),
			null,
			e("footer", null, "©"));
	}
	window.before = ["app", "count", "lazy", "field"].map((id) => document.getElementById(id));
	window.mounted = ReactDOM.hydrateRoot(document.getElementById("root"), e(App),
		{ onRecoverableError: (error) => recovered.push(String(error && error.message || error)) });
`

// served is app as React's server writes it: comments between the texts
// of one element and round the Suspense boundary.
const served = `<div id="app" class="shell"><h1>Hello <!-- -->light</h1>` +
	`<button id="count">Clicked <!-- -->0<!-- --> times</button>` +
	`<!--$--><section id="lazy"><svg viewBox="0 0 10 10" width="10"><path d="M0 0h10"></path></svg>` +
	`<span>inside</span></section><!--/$--><input id="field" tabindex="3" value="v"/><footer>©</footer></div>`

func hydrated(t *testing.T, markup string) *scripted {
	t.Helper()
	s := run(t, `<!DOCTYPE html><html><head><title>Hydration</title></head><body><div id="root">`+markup+`</div>`+
		`<script>`+react+`</script><script>`+reactDOM+`</script><script>`+app+`</script></body></html>`)
	s.frames("the hydration", func() bool { return s.eval(`window.hydrated === true`) == "true" })
	return s
}

func TestReactHydratesThePageItsServerWrote(t *testing.T) {
	s := hydrated(t, served)
	if got := s.eval(`recovered.join("; ")`); got != "" {
		t.Errorf("React recovered from %s", got)
	}
	if got := s.eval(
		`before.map((n) => n.isConnected && n === document.getElementById(n.id)).join()`,
	); got != "true,true,true,true" {
		t.Errorf("the server's elements are still the page's: %s", got)
	}
	if got := s.eval(`document.getElementById("root").innerHTML`); got != strings.Replace(served, `"v"/>`, `"v">`, 1) {
		t.Errorf("after hydration the page is\n     %s\nwant %s", got, served)
	}
	count := s.byID("count")
	s.rt.Click(count, 10, 10)
	s.frames("the click's render", func() bool { return count.Text() == "Clicked 1 times" })
	s.rt.Click(count, 10, 10)
	s.frames("the second click's render", func() bool { return count.Text() == "Clicked 2 times" })
	if got := s.eval(`count.innerHTML + "|" + count.childNodes.length`); got != "Clicked <!-- -->2<!-- --> times|5" {
		t.Errorf("the button holds %s", got)
	}
	if got := s.eval(`mounted.unmount(); document.getElementById("root").innerHTML`); got != "" {
		t.Errorf("unmounted, the root holds %s", got)
	}
	if len(s.byID("root").Children()) != 0 {
		t.Errorf("unmounted, the root shows %s", showing(s.byID("root")))
	}
	s.quiet()
}

func TestReactTellsOfAPageThatIsNotItsServers(t *testing.T) {
	s := hydrated(t, strings.Replace(served, `<span>inside</span>`, ``, 1))
	s.frames("the recovery", func() bool { return s.eval(`recovered.length > 0`) == "true" })
	if got := s.eval(`recovered.join("; ")`); !strings.Contains(got, "Minified React error #") {
		t.Errorf("React recovered from %q, want a hydration error", got)
	}
	if got := s.eval(`lazy.innerHTML`); !strings.HasSuffix(got, `<span>inside</span>`) {
		t.Errorf("rendered again, the section holds %s", got)
	}
	if got := showing(s.byID("lazy")); got != "span" {
		t.Errorf("rendered again, the section shows %s", got)
	}
}
