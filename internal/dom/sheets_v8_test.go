//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/internal/dom"
)

// styledPage is a box 100 wide by the page's own style element, for
// scripts to style otherwise.
const styledPage = `<head><style id="own">#box { width: 100px; height: 10px; }</style></head>
	<body style="margin: 0"><div id="box" class="box">box</div>`

// boxWidth is how wide the page's box is laid out, a frame after what
// scripts did.
func (s *scripted) boxWidth() float32 {
	s.t.Helper()
	s.rt.Frame()
	s.layout()
	return s.byID("box").Box().W
}

func (s *scripted) wide(want float32, when string) {
	s.t.Helper()
	if got := s.boxWidth(); got != want {
		s.t.Errorf("%s: the box is %v wide, want %v", when, got, want)
	}
}

func TestAStyleElementAScriptAddsIsApplied(t *testing.T) {
	s := run(t, styledPage+`<script>
		var added = document.createElement("style");
		added.textContent = "#box { width: 200px }";
		document.head.appendChild(added);</script>`)
	s.wide(200, "a style element appended to the head")

	s.eval(`added.textContent = "#box { width: 210px }"`)
	s.wide(210, "its textContent set")
	s.eval(`added.appendChild(document.createTextNode(" #box { width: 220px }"))`)
	s.wide(220, "a text node appended to it")
	s.eval(`added.firstChild.data = "#box { width: 225px }"`)
	s.wide(220, "its first text node changed, the second still after it")
	s.eval(`added.innerHTML = "#box { width: 230px }"`)
	s.wide(230, "its innerHTML set")
	s.eval(`added.media = "(min-width: 5000px)"`)
	s.wide(100, "its media set to one that does not hold")
	s.eval(`added.removeAttribute("media")`)
	s.wide(230, "its media removed")

	s.eval(`added.remove()`)
	s.wide(100, "the element removed")
	s.eval(`document.body.appendChild(added)`)
	s.wide(230, "the element put back, in the body")
	s.eval(`added.type = "text/plain"`)
	s.wide(100, "a style element of another type styles nothing")
	s.quiet()
}

//nolint:lll // a script is one line of its table
func TestStyleElementsCascadeInThePagesOrder(t *testing.T) {
	s := run(t, styledPage+`<script>
		var early = document.createElement("style");
		early.textContent = "#box { width: 200px }";
		document.head.insertBefore(early, own);
		var late = document.createElement("style");
		late.textContent = "#box { width: 300px }";
		document.head.appendChild(late);</script>`)
	s.wide(300, "the last style element of the page wins")
	s.eval(`late.remove()`)
	s.wide(100, "one put before the page's own loses to it")
	s.eval(`document.head.appendChild(early)`)
	s.wide(200, "moved after the page's own, it wins")
	s.eval(`document.head.insertBefore(early, own); own.sheet.insertRule("#box { width: 150px }", 1)`)
	s.wide(150, "moved back before it, it loses again, to a rule added to the page's own meanwhile")
	if got := s.eval(`[...document.styleSheets].map((sheet) => sheet.ownerNode.id || "early").join()`); got != "early,own" {
		t.Errorf("document.styleSheets are %s", got)
	}
	s.quiet()
}

func TestThePagesOwnStyleElementIsEditedAndRemovedByAScript(t *testing.T) {
	s := run(t, styledPage)
	s.wide(100, "as loaded")
	s.eval(`own.textContent = "#box { width: 140px }"`)
	s.wide(140, "its text changed")
	s.eval(`own.remove()`)
	if got := s.boxWidth(); got != 800 {
		t.Errorf("the page's style element removed, the box is %v wide, want the page's 800", got)
	}
	s.quiet()
}

func TestANestedRuleIsOneRuleOfItsSheet(t *testing.T) {
	s := run(t, `<head><style id="own">body { margin: 0; .box { width: 120px; } }</style></head>
		<body><div id="box" class="box">box</div>`)
	s.wide(120, "a rule nested in the page's own")
	if got := s.eval(`own.sheet.cssRules.length + " " + own.sheet.cssRules[0].selectorText`); got != "1 body" {
		t.Errorf("the sheet's rules are %q", got)
	}
	s.eval(`own.sheet.insertRule("#box { & { width: 60px; } }", 1)`)
	s.wide(60, "a nested rule inserted")
	if got := s.eval(`own.sheet.cssRules.length`); got != "2" {
		t.Errorf("with a nested rule inserted the sheet has %s rules", got)
	}
	s.eval(`own.sheet.deleteRule(1)`)
	s.wide(120, "the nested rule deleted")
}

//nolint:lll // a script is one line of its table
func TestRulesAreInsertedAndDeletedThroughASheet(t *testing.T) {
	s := run(t, styledPage+`<script>var sheet = own.sheet;</script>`)
	for _, c := range []struct{ name, script, want string }{
		{"a style element has a sheet", `sheet instanceof CSSStyleSheet && own.sheet === sheet && sheet.ownerNode === own`, "true"},
		{"its rules", `sheet.cssRules.length + " " + sheet.cssRules[0].cssText + " " + sheet.rules.item(0).selectorText`,
			"1 #box { width: 100px; height: 10px; } #box"},
		{"a rule's style", `sheet.cssRules[0].style.width + sheet.cssRules[0].style.getPropertyValue("height")`, "100px10px"},
		{"a rule is a CSSStyleRule", `sheet.cssRules[0] instanceof CSSStyleRule && sheet.cssRules[0].type === CSSRule.STYLE_RULE &&
			sheet.cssRules[0].parentStyleSheet === sheet`, "true"},
		{"insertRule answers the index", `sheet.insertRule("#box { width: 250px }", 1)`, "1"},
		{"insertRule defaults to the first place", `sheet.insertRule(".none { color: red }") + " " + sheet.cssRules[0].selectorText +
			" " + sheet.cssRules.length`, "0 .none 3"},
		{"the list is live", `(list => { sheet.insertRule("@media (min-width: 1px) { .a { color: red } }", 3);
			return list.length + " " + (list[3] instanceof CSSMediaRule) + " " + list[3].media.mediaText + " " +
			list[3].cssRules[0].selectorText })(sheet.cssRules)`, "4 true (min-width: 1px) .a"},
		{"a rule no browser reads throws", `try { sheet.insertRule("#box { width: 1px") } catch (e) { e.name + (e instanceof DOMException) }`,
			"SyntaxErrortrue"},
		{"two rules at once throw", `try { sheet.insertRule("a { } b { }") } catch (e) { e.name }`, "SyntaxError"},
		{"an index past the end throws", `try { sheet.insertRule("a { }", 9) } catch (e) { e.name }`, "IndexSizeError"},
		{"a property ui does not know is taken", `sheet.insertRule("#box { -x-unknown: 1; height: 33px }", sheet.cssRules.length)`, "4"},
		{"deleteRule out of range throws", `try { sheet.deleteRule(9) } catch (e) { e.name }`, "IndexSizeError"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s: %s is %q, want %q", c.name, c.script, got, c.want)
		}
	}
	s.wide(250, "a rule inserted after the page's own")
	if got := s.byID("box").Box().H; got != 33 {
		t.Errorf("the box is %v high, want 33", got)
	}
	s.eval(`sheet.deleteRule(2)`)
	s.wide(100, "the rule deleted")
	s.eval(`sheet.cssRules[1].style.width = "175px"`)
	s.wide(175, "a rule's style changed")
	s.eval(`sheet.cssRules[1].style.setProperty("width", "176px", "important")`)
	s.wide(176, "a rule's style set with setProperty")
	s.eval(`sheet.cssRules[1].selectorText = ".elsewhere"`)
	if got := s.boxWidth(); got != 800 {
		t.Errorf("a rule's selector changed, the box is %v wide, want the page's 800", got)
	}
	s.eval(`sheet.cssRules[2].insertRule("#box { width: 180px !important }", 1)`)
	s.wide(180, "a rule inserted in a media rule")
	s.eval(`sheet.cssRules[2].cssRules[1].style.width = "181px"`)
	s.wide(181, "a rule's style in a media rule changed")
	s.eval(`sheet.cssRules[2].deleteRule(1); sheet.cssRules[2].cssRules[0].selectorText = "#box"; sheet.cssRules[2].cssRules[0].style.cssText = "width: 190px !important"`)
	s.wide(190, "a rule of a media rule rewritten")
	s.eval(`sheet.disabled = true`)
	if got := s.boxWidth(); got != 800 {
		t.Errorf("the sheet disabled, the box is %v wide, want the page's 800", got)
	}
	s.eval(`sheet.disabled = false`)
	s.wide(190, "the sheet enabled again")
	if got := s.eval(`own.textContent = "#box { width: 120px }"; own.sheet === sheet && sheet.cssRules.length + sheet.cssRules[0].cssText`); got != "1#box { width: 120px }" {
		t.Errorf("the element's text set, its sheet says %q", got)
	}
	if got := s.eval(`(e => e.sheet)(document.createElement("style"))`); got != "null" {
		t.Errorf("a style element out of the page has the sheet %s", got)
	}
	for _, m := range s.console {
		if m.Level == "error" {
			t.Errorf("the console says %s", m.Text)
		}
	}
}

func TestALinkAScriptAddsIsFetchedAndApplied(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{
		"https://site.test/dir/wide.css":  {Status: 200, StatusText: "200 OK", Text: `#box { width: 300px }`},
		"https://site.test/dir/wider.css": {Status: 200, StatusText: "200 OK", Text: `#box { width: 400px }`},
	}}
	s.start(styledPage+`<script>var said = [];
		var link = document.createElement("link");
		link.rel = "stylesheet";
		link.href = "wide.css";
		link.onload = () => said.push("load " + link.sheet.cssRules.length + " " + link.sheet.href);
		link.onerror = () => said.push("error");
		document.head.appendChild(link);
		said.push("sheet " + link.sheet);</script>`, dom.Host{})
	s.frames("the link's load", func() bool { return strings.HasPrefix(s.eval(`said.join()`), "sheet null,load") })
	if got := s.eval(`said.join()`); got != "sheet null,load 1 https://site.test/dir/wide.css" {
		t.Errorf("the link told %q", got)
	}
	s.wide(300, "a linked sheet added by a script")
	if len(s.fetched) != 1 || !s.fetched[0].Sheet || s.fetched[0].Address != "https://site.test/dir/wide.css" {
		t.Errorf("the page fetched %+v", s.fetched)
	}

	s.eval(`link.media = "(max-width: 10px)"`)
	s.wide(100, "the link's media set to one that does not hold")
	s.eval(`link.media = "all"`)
	s.wide(300, "the link's media set to all")
	s.eval(`link.disabled = true`)
	s.wide(100, "the link disabled")
	s.eval(`link.disabled = false`)
	s.frames("the link's sheet again", func() bool { return s.boxWidth() == 300 })

	s.eval(`said.length = 0; link.href = "wider.css"`)
	s.frames("the link's second load", func() bool { return s.eval(`said.join()`) != "" })
	s.wide(400, "the link's href changed")

	s.eval(`said.length = 0; link.href = "missing.css"`)
	s.frames("the link's error", func() bool { return s.eval(`said.join()`) == "error" })
	s.wide(400, "a sheet that did not load leaves the one before")
	s.eval(`link.remove()`)
	s.wide(100, "the link removed")
	if !strings.Contains(s.said(), "A stylesheet was not loaded") {
		t.Errorf("the console says %q", s.said())
	}
}

//nolint:lll // a script is one line of its table
func TestComputedStylesAreThoseUIComputed(t *testing.T) {
	s := run(t, `<head><style>
		body { margin: 0; color: #336699; font-size: 20px; }
		#box { width: 50%; padding: 1em; display: flex; position: relative; --accent: #ff0000; background-color: var(--accent);
			border: 2px solid rgba(0, 0, 0, 0.5); opacity: 0.5; z-index: 3; }
		.hidden { display: none; }
		</style></head><body><div id="box" class="box" style="margin-top: 4px"><span id="in">in</span></div>
		<p id="gone" class="hidden">gone</p>`)
	for _, c := range []struct{ name, script, want string }{
		{"a declared keyword", `getComputedStyle(box).display + " " + getComputedStyle(box).position`, "flex relative"},
		{"an inherited colour, as rgb()", `getComputedStyle(document.getElementById("in")).color`, "rgb(51, 102, 153)"},
		{"a length in px", `getComputedStyle(box).paddingLeft + " " + getComputedStyle(box).getPropertyValue("padding-top")`, "20px 20px"},
		{"the style attribute", `getComputedStyle(box).marginTop`, "4px"},
		{"a var() replaced", `getComputedStyle(box).backgroundColor`, "rgb(255, 0, 0)"},
		{"a custom property", `getComputedStyle(box).getPropertyValue("--accent").trim()`, "#ff0000"},
		{"a width as laid out", `getComputedStyle(box).width + " " + getComputedStyle(box).height`, "400px 23px"},
		{"numbers", `getComputedStyle(box).opacity + " " + getComputedStyle(box).zIndex`, "0.5 3"},
		{"an initial value", `getComputedStyle(document.getElementById("in")).position + " " + getComputedStyle(box).float`, "static none"},
		{"display: none", `getComputedStyle(gone).display`, "none"},
		{"font size", `getComputedStyle(box).fontSize`, "20px"},
		{"a class a script adds is read at once", `gone.className = ""; getComputedStyle(gone).display`, "block"},
		{"a style a script sets is read at once", `box.style.display = "grid"; getComputedStyle(box).display`, "grid"},
		{"a sheet a script adds is read at once", `(e => { e.textContent = "#in { position: absolute }";
			document.head.appendChild(e); return getComputedStyle(document.getElementById("in")).position })(
			document.createElement("style"))`, "absolute"},
		{"an element a script made is read at once", `(made => made.map(e => getComputedStyle(e).display).join(" "))(
			["div", "span", "p"].map(tag => document.body.appendChild(document.createElement(tag))).map((e, i) => {
				if (i === 2) e.className = "hidden"; return e }))`, "block inline none"},
		{"the properties are listed", `(cs => cs.length > 50 && cs.item(0) !== "" && Object.keys(cs).includes("display"))(
			getComputedStyle(box))`, "true"},
		{"an unknown property", `getComputedStyle(box).getPropertyValue("no-such-thing")`, ""},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s: %s is %q, want %q", c.name, c.script, got, c.want)
		}
	}
	s.eval(`box.style.boxSizing = "border-box"`)
	s.layout()
	if got := s.eval(`getComputedStyle(box).width`); got != "400px" {
		t.Errorf("a box sized by its border is %s wide once laid out again, want 400px", got)
	}
	s.quiet()
}

func TestAnElementsStyleAndClassesRestyleIt(t *testing.T) {
	s := run(t, styledPage+`<style>.wide { width: 300px !important } .box.on { min-height: 44px }</style>`)
	s.eval(`box.style.width = "150px"`)
	s.wide(150, "element.style set")
	s.eval(`box.classList.add("wide", "on")`)
	s.wide(300, "a class added")
	if got := s.byID("box").Box().H; got != 44 {
		t.Errorf("the box is %v high, want 44", got)
	}
	s.eval(`box.className = "box"; box.style.cssText = ""; box.removeAttribute("style")`)
	s.wide(100, "the class and the style removed")
	s.eval(`box.setAttribute("style", "width: 60px")`)
	s.wide(60, "the style attribute set")
	s.quiet()
}

//nolint:lll // a script is one line of its table
func TestSheetsScriptsMakeAreAdopted(t *testing.T) {
	s := run(t, styledPage+`<script>
		var made = new CSSStyleSheet();
		made.replaceSync("#box { width: 260px }");
		var other = new CSSStyleSheet({ media: "(min-width: 1px)" });
		other.insertRule("#box { width: 270px }");</script>`)
	s.wide(100, "a sheet a script made styles nothing until adopted")
	s.eval(`document.adoptedStyleSheets = [made]`)
	s.wide(260, "a sheet adopted")
	s.eval(`document.adoptedStyleSheets = [...document.adoptedStyleSheets, other]`)
	s.wide(270, "a second one adopted after it")
	s.eval(`document.adoptedStyleSheets = [other, made]`)
	s.wide(260, "their order turned")
	s.eval(`document.head.appendChild(Object.assign(document.createElement("style"), { textContent: "#box { width: 280px }" }))`)
	s.wide(260, "an adopted sheet wins over a style element added after it")
	s.eval(`made.replace("#box { width: 290px }")`)
	s.wide(290, "replace")
	s.eval(`document.adoptedStyleSheets = []`)
	s.wide(280, "none adopted any more")
	for _, c := range []struct{ script, want string }{
		{`document.adoptedStyleSheets.length + " " + made.cssRules.length + " " + made.ownerNode + " " + other.media.mediaText`,
			"0 1 null (min-width: 1px)"},
		{`try { own.sheet.replaceSync("") } catch (e) { e.name }`, "NotAllowedError"},
		{`try { document.adoptedStyleSheets = [own.sheet] } catch (e) { e.name }`, "NotAllowedError"},
		{`[CSS.supports("display", "grid"), CSS.supports("display", "nonsense"), CSS.supports("(display: flex) and (not (display: nonsense))"),
			CSS.supports("no-such-property: 1"), CSS.escape("a:b")].join()`, `true,false,true,false,a\:b`},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s is %q, want %q", c.script, got, c.want)
		}
	}
	s.quiet()
}

// TestStyledComponentsRulesAreApplied does what styled-components does
// as a page starts: a style element in the head, its sheet read back,
// then a rule inserted for each component, hundreds of them, each at
// the end of its group; the rules a server sent are read from their
// element's text, inserted again, and the element removed.
func TestStyledComponentsRulesAreApplied(t *testing.T) {
	const components = 800
	var page, server strings.Builder
	for i := range components {
		fmt.Fprintf(&page, `<div id="c%d" class="sc-%d">%d</div>`, i, i, i)
	}
	for i := range 100 {
		fmt.Fprintf(&server, ".sc-%d{height:3px;}/*!sc*/\n", i)
	}
	s := run(t, `<head><style data-styled="true" data-styled-version="6.1.0">`+server.String()+`</style></head>
		<body style="margin: 0"><div id="box">box</div>`+page.String()+`<script>
		function getSheet(tag) {
			if (tag.sheet) return tag.sheet;
			for (const sheet of document.styleSheets) if (sheet.ownerNode === tag) return sheet;
			throw new Error("styled-components error #17: CSSStyleSheet could not be found on HTMLStyleElement");
		}
		function start() {
			const began = performance.now();
			const tag = document.createElement("style");
			tag.setAttribute("data-styled", "active");
			tag.appendChild(document.createTextNode(""));
			document.head.appendChild(tag);
			const sheet = getSheet(tag);
			let length = 0;
			const insert = (index, rule) => {
				try { sheet.insertRule(rule, index); length++; return true; } catch (e) { return false; }
			};
			const old = document.querySelector('style[data-styled="true"]');
			for (const rule of old.textContent.split("/*!sc*/\n").filter(Boolean)) insert(length, rule);
			old.parentNode.removeChild(old);
			for (let i = 0; i < `+fmt.Sprint(components)+`; i++) {
				insert(length, ".sc-" + i + "{width:" + (10 + i % 50) + "px;color:red;}");
				insert(Math.floor(length / 2), ".sc-" + i + ":hover{color:blue;}");
			}
			window.inserted = length;
			window.refused = insert(length, ".sc-0::-moz-thing{") === false;
			window.read = sheet.cssRules[0].cssText + " " + sheet.cssRules.length;
			window.took = performance.now() - began;
		}
		</script>`)
	s.eval(`start()`)
	began := time.Now()
	s.rt.Frame()
	s.layout()
	frame := time.Since(began)
	if got := s.eval(`inserted + " " + refused + " " + read`); got != "1700 true .sc-0{height:3px;} 1700" {
		t.Errorf("the page inserted %s", got)
	}
	for i, want := range map[int][2]float32{0: {10, 3}, 49: {59, 3}, 99: {59, 3}, 100: {10, 0}, 799: {59, 0}} {
		box := s.byID(fmt.Sprint("c", i)).Box()
		if box.W != want[0] || want[1] != 0 && box.H != want[1] {
			t.Errorf("component %d is %v by %v, want %v by %v", i, box.W, box.H, want[0], want[1])
		}
	}
	took := s.eval(`took`)
	t.Logf("1700 rules inserted by the script in %s ms, the frame after in %v", took, frame)
	if frame > 500*time.Millisecond {
		t.Errorf("the frame after the rules took %v", frame)
	}
	s.eval(`if (took > 1000) throw new Error("inserting took " + took + " ms")`)
	s.quiet()
}

func TestThePagesOwnLinksAreEditedAndRemovedByAScript(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{
		"https://site.test/dir/wider.css": {Status: 200, StatusText: "200 OK", Text: `#box { width: 400px }`},
		"https://site.test/dir/print.css": {Status: 200, StatusText: "200 OK", Text: `#box { height: 77px }`},
	}}
	s.start(`<head><style>#box { width: 100px; height: 10px }</style>
		<link id="late" rel="stylesheet" href="print.css" media="print" onload="this.media = 'all'">
		<link id="own" rel="stylesheet" href="http://127.0.0.1:1/own.css"></head>
		<body style="margin: 0"><div id="box">box</div>`, dom.Host{})
	s.frames("the sheet a page loads late", func() bool { return s.byID("box").Box().H == 77 })
	s.wide(100, "as loaded")
	s.eval(`own.href = "wider.css"`)
	s.frames("the sheet of the link's new address", func() bool { return s.boxWidth() == 400 })
	if got := s.eval(
		`[...document.styleSheets].map((sheet) => sheet.ownerNode.id || "style").join()`,
	); got != "style,late,own" {
		t.Errorf("document.styleSheets are %s", got)
	}
	s.eval(`own.remove(); late.rel = "alternate stylesheet"`)
	s.wide(100, "the link removed")
	if got := s.byID("box").Box().H; got != 10 {
		t.Errorf("a link that is no stylesheet any more leaves the box %v high, want 10", got)
	}
}

func TestSizesAreReadAsLaidOutAtOnce(t *testing.T) {
	s := run(t, `<head><style>body { margin: 0 } .half { width: 50vw; height: 10vh }</style></head>
		<body><div id="box" style="width: 100px; height: 20px">box</div>`)
	for _, c := range []struct{ name, script, want string }{
		{"an element a script made", `(e => { document.body.appendChild(e); e.className = "half";
			return getComputedStyle(e).width + " " + e.offsetWidth })(document.createElement("div"))`, "400px 400"},
		{"markup a script set", `document.body.innerHTML = '<div id="made" style="width: 30px; height: 5px"></div>';
			getComputedStyle(made).width + " " + made.offsetHeight`, "30px 5"},
		{"a style a script changed", `made.style.width = "70px"; getComputedStyle(made).width + " " + made.offsetWidth`,
			"70px 70"},
		{"a sheet a script added", `document.head.appendChild(Object.assign(document.createElement("style"),
			{ textContent: "#made { height: 9px !important }" }));
			getComputedStyle(made).height + " " + made.offsetHeight`, "9px 9"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s: read %q, want %q", c.name, got, c.want)
		}
	}
	s.quiet()
}
