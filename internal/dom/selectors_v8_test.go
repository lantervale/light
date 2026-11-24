//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import "testing"

func TestSelectorsAreReadAsCSSWritesThem(t *testing.T) {
	s := run(t, `<!DOCTYPE html><html lang="en"><body><div id="0start" class="a:b" title="T" data-v="é">x</div>
		<p id="p" lang="fr-CA"><span id="in">y</span></p><svg id="svg"><a id="sa" xlink:href="#p"></a></svg>
		<span id="-a" class="5cm"></span><a id="link" href="#p"></a>`)
	const tools = `const ids = (selector) => [...document.querySelectorAll(selector)].map((e) => e.id).join(" "),
		thrown = (run) => { try { run(); return "none" } catch (e) { return e.name } };`
	for _, c := range []struct{ selector, want string }{
		{`#\30 start`, "0start"},
		{`#\30start`, "0start"},
		{`#\000030 start`, "0start"},
		{`.a\:b`, "0start"},
		{`[data-v="\e9"]`, "0start"},
		{`[data-v="\e9 "]`, "0start"},
		{`[data-v=\e9]`, "0start"},
		{`[title="T"`, "0start"},
		{`[title="T`, "0start"},
		{`body :not(#p`, "0start in svg sa -a link"},
		{`body > :is(#p, #nope`, "p"},
		{`[*|title]`, "0start"},
		{`[|title]`, "0start"},
		{`[*|href]`, "sa link"},
		{`*|p, |nothing, *|*#in`, "p in"},
		{`body |*`, ""},
		{`body :lang(en)`, "0start svg sa -a link"},
		{`:lang(fr)`, "p in"},
		{`:lang(fr-CA), :lang("FR-ca")`, "p in"},
		{`:lang(f)`, ""},
		{`#p::before, #p:after, #p::first-line, #p:first-letter, #p::marker, ::slotted(span), #in`, "in"},
		{`#\-a, [class="5cm"]`, "-a"},
		{`:root`, ""},
		{`:any-link`, "sa link"},
	} {
		if got := s.eval(tools + "ids(String.raw`" + c.selector + "`)"); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.selector, got, c.want)
		}
	}
	for script, want := range map[string]string{
		"[String.raw`.5cm`, String.raw`#-5`, `ns|p`, `[ns|title]`, `::nope`, `:nope`, `p:lang()`, `p::before(x)`, " +
			"`div,`, `[a=b c]`, `p >`, `:not()`].map((q) => thrown(() => document.querySelector(q))).join()": "SyntaxError," +
			"SyntaxError,SyntaxError,SyntaxError,SyntaxError,SyntaxError,SyntaxError,SyntaxError,SyntaxError,SyntaxError," +
			"SyntaxError,SyntaxError",
		`(() => { const d = document.createElement("div"), f = document.createDocumentFragment(), e =
			document.createElement("i"); f.append(e); return [d.matches(":root"), e.matches(":root"),
			document.documentElement.matches(":root"), f.querySelector(":root")].join() })()`: "false,false,true,",
		`[thrown(() => document.querySelector()), thrown(() => document.body.matches()),
			document.querySelector(undefined), document.querySelector(null)].join()`: "TypeError,TypeError,,",
		`(() => { const d = document.createElement("div"); d.innerHTML = "<i class='a\u00a0b'></i><i class='a\tb c\fd'>" +
			"</i>"; return [d.getElementsByClassName("a").length, d.getElementsByClassName("a\u00a0b").length,
			d.getElementsByClassName("b\nd").length, d.querySelectorAll(".a").length].join() })()`: "1,1,1,1",
	} {
		if got := s.eval(tools + script); got != want {
			t.Errorf("%s:\n got %s\nwant %s", script, got, want)
		}
	}
	s.quiet()
}

func TestTheTargetOfAnAddressMatches(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><p id="a b">x</p><p id="other">y</p>`)
	s.eval(`location.hash = "#a b"`)
	if got := s.eval(`[...document.querySelectorAll(":target")].map((e) => e.id).join() + "|" +
		document.getElementById("other").matches(":target")`); got != "a b|false" {
		t.Errorf(":target matched %s, want the element the address names", got)
	}
	s.quiet()
}
