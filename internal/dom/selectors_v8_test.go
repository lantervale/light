//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"testing"

	"github.com/lantervale/ember"
	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
)

func TestPseudoClassesFollowWhatAPersonDoesToThePage(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body style="margin:0"><details id="d" open><summary>s</summary></details>
		<details id="c"></details><dialog id="g" open>g</dialog><select id="s" style="display:block;height:30px">
		<option>a</option><option>b</option></select><div id="p" popover>p</div><input id="req" required>
		<input id="opt">`)
	win := platform.NewWindow(platform.Size(800, 600))
	win.Add(s.page.Doc)
	w := ember.New().AddEntities(win)
	in := win.Input()
	in.SetMouse(geom.V2(700, 500))
	w.RunFrames(1)
	const ids = `const ids = (selector) => [...document.querySelectorAll(selector)].map((e) => e.id).join(" ");`
	matches := func(selector, want string) {
		t.Helper()
		if got := s.eval(ids + "ids(`" + selector + "`)"); got != want {
			t.Errorf("%s:\n got %q\nwant %q", selector, got, want)
		}
	}
	matches(":open", "d g")
	matches(":popover-open, :modal, :user-valid, :user-invalid, body :hover, :active, :focus-visible, :focus-within", "")
	matches(":is(:open, :popover-open", "d g")
	if got := s.eval(`[document.createElement("details"), document.createElement("dialog")].map((e) => {
		e.open = true; return e.matches(":open") }).join()`); got != "true,true" {
		t.Errorf("a details and a dialog open out of the page match :open %s", got)
	}

	at, shown := s.page.Doc.BoxOf(s.byID("s"))
	if !shown {
		t.Fatal("the select is not shown")
	}
	in.SetMouse(geom.V2(at.X+at.W/2, at.Y+at.H/2))
	w.RunFrames(1)
	matches("body :hover", "s")
	in.SetMouseButton(input.MouseLeft, true)
	w.RunFrames(1)
	matches("body :active", "s")
	in.SetMouseButton(input.MouseLeft, false)
	w.RunFrames(1)
	matches(":open", "d g s")
	matches("body :focus-within", "s")

	s.page.Doc.ShowPopover(s.byID("p"), nil)
	matches(":popover-open", "p")

	s.page.Doc.Focus(s.byID("req"))
	in.AddText('x')
	w.RunFrames(1)
	s.page.Doc.Blur()
	w.RunFrames(1)
	matches(":user-valid", "req")
	matches(":user-invalid", "")
	s.quiet()
}

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
