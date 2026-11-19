//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lantervale/light/internal/dom"
)

// line is the message of the console that begins with text.
func (s *scripted) line(text string) dom.Message {
	s.t.Helper()
	for _, m := range s.console {
		if strings.HasPrefix(m.Text, text) {
			return m
		}
	}
	s.t.Fatalf("the console has no line %q:\n%s", text, s.said())
	return dom.Message{}
}

func TestTheConsolePrintsValuesAsAConsoleDoes(t *testing.T) {
	s := run(t, `<p id="x" class="a b">hi</p>`)
	for _, c := range []struct{ said, want string }{
		{`"one", "two", 3`, `one two 3`},
		{`["a", 1], {s: "x"}, null, undefined`, `["a", 1] {s: "x"} null undefined`},
		{`"%cstyled%c plain %o %% %s %d", "color: red", "", {a: 1}, "str", 4.7, "more"`, `styled plain {a: 1} % str 4 more`},
		{`function named() {}, () => 1, class Thing {}`, `ƒ named() ƒ anonymous() class Thing`},
		{`Symbol("s"), 10n, -0, NaN`, `Symbol(s) 10n -0 NaN`},
		{`new Map([["k", 1]]), new Set([1]), new Uint8Array(2)`, `Map(1) {k: 1} Set(1) [1] Uint8Array(2) [0, 0]`},
		{`document.getElementById("x"), document.createTextNode("t"), document, window`,
			`<p#x.a.b> "t" #document Window`},
		{`{a: {b: {c: {d: {e: 1}}}}}, new Date(0), /re/g`, `{a: {b: {c: {d: {…}}}}} 1970-01-01T00:00:00.000Z /re/g`},
		{`new RangeError("far")`, `RangeError: far`},
		{`(() => { try { null.x } catch (e) { return e } })()`,
			"TypeError: Cannot read properties of null (reading 'x')"},
	} {
		s.console = nil
		if _, err := s.rt.Evaluate("console.log(" + c.said + ")"); err != nil {
			t.Fatalf("%s: %v", c.said, err)
		}
		if len(s.console) != 1 || s.console[0].Text != c.want {
			t.Errorf("console.log(%s) says %q, want %q", c.said, s.said(), c.want)
		}
	}
}

func TestTheConsolesOtherCallsSayWhatTheyDo(t *testing.T) {
	s := run(t, `<script>
		function outer() { inner(); }
		function inner() { console.trace("here", 1); }
		outer();
		console.group("Loading"); console.log("in"); console.groupCollapsed("Deeper"); console.warn("deep");
		console.groupEnd(); console.groupEnd(); console.groupEnd(); console.log("out");
		console.table([{name: "ember", stars: 12}, {name: "light"}]);
		console.table("not a table");
		console.assert(true, "fine"); console.assert(false); console.assert(1 === 2, "math %s", "broke");
		console.count(); console.count(); console.count("other"); console.countReset(); console.count();
		console.time("load"); console.timeLog("load", "half"); console.timeEnd("load"); console.timeEnd("load");
		console.dir({a: 1});
	</script>`)
	want := []string{
		"log: here 1", "log: Loading", "log: in", "log: Deeper", "warn: deep", "log: out",
		"log: (index)  name   stars\n0        ember  12\n1        light", "log: not a table",
		"error: Assertion failed", "error: Assertion failed: math broke",
		"log: default: 1", "log: default: 2", "log: other: 1", "log: default: 1",
		"log: load: ", "log: load: ", "warn: Timer 'load' does not exist", "log: {a: 1}",
	}
	if len(s.console) != len(want) {
		t.Fatalf("the console has %d lines, want %d:\n%s", len(s.console), len(want), s.said())
	}
	for i, m := range s.console {
		if got := m.Level + ": " + m.Text; !strings.HasPrefix(got, want[i]) {
			t.Errorf("line %d of the console is %q, want %q", i, got, want[i])
		}
	}
	if timed := s.console[14].Text; !strings.HasSuffix(timed, " ms half") {
		t.Errorf("timeLog says %q", timed)
	}
	var depths []int
	for _, m := range s.console[1:6] {
		depths = append(depths, m.Depth)
	}
	if got := fmt.Sprint(depths); got != "[0 1 1 2 0]" {
		t.Errorf("the lines of the groups are %s deep, want [0 1 1 2 0]", got)
	}
	trace := s.console[0]
	if trace.Line != 3 || !strings.HasPrefix(trace.Stack, "inner ("+address+":3:") ||
		!strings.Contains(trace.Stack, "\nouter ("+address+":2:") || strings.Contains(trace.Stack, "light:") {
		t.Errorf("the trace is from line %d by %q", trace.Line, trace.Stack)
	}
	if s.console[2].Stack != "" {
		t.Errorf("a plain line has a stack: %q", s.console[2].Stack)
	}
}

func TestWhatNothingCaughtIsAnErrorWithItsPlace(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{
		"https://site.test/there.json": {Status: 200, StatusText: "200 OK", Text: "{}"},
	}}
	s.start(`<script>
		function deep() { null.x; }
		setTimeout(function later() { deep(); }, 0);
		setTimeout(() => { throw "a string"; }, 0);
		window.addEventListener("load", () => { throw new RangeError("on load"); });
	</script><script>var broken = ;</script>
	<script>Promise.reject(new Error("lost"));
	async function ask() { throw new TypeError("asked"); }
	ask();</script><script src="/gone.js"></script>
	<script>fetch("/missing.json"); fetch("/there.json")</script>`, dom.Host{})
	s.frames("the timers ran", func() bool {
		return strings.Contains(s.said(), "a string") && strings.Contains(s.said(), "missing.json")
	})
	null := s.line("Uncaught TypeError: Cannot read properties of null")
	if null.Level != "error" || null.Source != address || null.Line != 2 ||
		!strings.HasPrefix(null.Stack, "deep ("+address+":2:") || !strings.Contains(null.Stack, "\nlater ("+address+":3:") ||
		strings.Contains(null.Stack, "light:") {
		t.Errorf("the exception is %s from %s:%d by %q", null.Level, null.Source, null.Line, null.Stack)
	}
	if text := s.line("Uncaught a string"); text.Level != "error" || text.Stack != "" {
		t.Errorf("a thrown string is %+v", text)
	}
	if load := s.line("Uncaught RangeError: on load"); load.Source != address || load.Line != 5 {
		t.Errorf("the handler's exception is from %s:%d", load.Source, load.Line)
	}
	if syntax := s.line("Uncaught SyntaxError: Unexpected token ';'"); syntax.Source != address || syntax.Line != 1 {
		t.Errorf("the syntax error is from %s:%d", syntax.Source, syntax.Line)
	}
	lost := s.line("Uncaught (in promise) Error: lost")
	if lost.Level != "error" || lost.Source != address || lost.Line != 1 {
		t.Errorf("the rejection is %s from %s:%d", lost.Level, lost.Source, lost.Line)
	}
	if asked := s.line("Uncaught (in promise) TypeError: asked"); asked.Line != 2 || strings.Contains(asked.Text, "\n") {
		t.Errorf("the async function's rejection is %q from line %d", asked.Text, asked.Line)
	}
	if gone := s.line("A script was not run"); gone.Level != "warn" || !strings.HasSuffix(gone.Source, "/gone.js") {
		t.Errorf("the script that did not load is %+v", gone)
	}
	missing := s.line("Failed to load https://site.test/missing.json: the server answered 404 Not Found.")
	if missing.Level != "error" {
		t.Errorf("the request that failed is said as %s", missing.Level)
	}
	if strings.Contains(s.said(), "there.json") {
		t.Errorf("a request that was answered is in the console:\n%s", s.said())
	}
}

func TestTheConsolesLineGivesValuesAsAConsoleDoes(t *testing.T) {
	s := run(t, `<p id="x">hi</p><script>var n = 2</script>`)
	for _, c := range []struct{ typed, want string }{
		{`"a" + "b"`, `"ab"`}, {`n + 40`, `42`}, {`$_ * 2`, `84`}, {`undefined`, `undefined`}, {`null`, `null`},
		{`[1, "a"]`, `[1, "a"]`}, {`({a: "b"})`, `{a: "b"}`}, {`document.getElementById("x")`, `<p#x>`},
		{`new Error("made")`, `Error: made`}, {`const kept = 5`, `undefined`}, {`kept + 1`, `6`},
		{` let { a, b } = {a: 1, b: 2}`, `undefined`}, {`a + b`, `3`}, {`$_`, `3`},
		{"1 +\n2", `3`}, {`console.log("said"), 7`, `7`},
	} {
		if got, err := s.rt.Evaluate(c.typed); err != nil || got != c.want {
			t.Errorf("%s gives %s and fails with %v, want %s", c.typed, got, err, c.want)
		}
	}
	if said := s.line("said"); said.Source != "" || said.Line != 0 {
		t.Errorf("a line said from the console is from %s:%d", said.Source, said.Line)
	}
	for _, c := range []struct{ typed, want string }{
		{`nope`, `Uncaught ReferenceError: nope is not defined`},
		{`throw new TypeError("thrown")`, `Uncaught TypeError: thrown`},
		{`var = `, `Uncaught SyntaxError: Unexpected token '='`}, {`throw 1`, `Uncaught 1`},
	} {
		if _, err := s.rt.Evaluate(c.typed); err == nil || err.Error() != c.want {
			t.Errorf("%s fails with %v, want %s", c.typed, err, c.want)
		}
	}
	if got, _ := s.rt.Evaluate(`$_`); got != `7` {
		t.Errorf("after a line that threw $_ is %s, want the last value, 7", got)
	}
}
