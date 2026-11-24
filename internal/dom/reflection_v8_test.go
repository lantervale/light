//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import "testing"

const reflecting = `const make = (tag, attrs = {}) => { const e = document.createElement(tag);
	for (const k of Object.keys(attrs)) e.setAttribute(k, attrs[k]); return e };
	const got = (tag, idl, attr, ...values) => { const e = document.createElement(tag);
		return values.map((v) => { v === null ? e.removeAttribute(attr) : e.setAttribute(attr, v); return e[idl] }) };
	const put = (tag, idl, attr, ...values) => { const e = document.createElement(tag);
		return values.map((v) => { try { e[idl] = v } catch (x) { return x.name }
			return JSON.stringify(e.getAttribute(attr)) + "=" + e[idl] }) };`

func TestAnAttributeIsReadAsItsKind(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><p id="p">p</p>`)
	for _, c := range []struct{ name, script, want string }{
		{"text is kept as written", `[got("a", "target", "target", null, "", " X y ", "\0"), put("a", "target", "target",
			" A ", null, undefined, 7, { toString() { return "s" } })].join("|")`,
			",, X y ,\x00|\" A \"= A ,\"null\"=null,\"undefined\"=undefined,\"7\"=7,\"s\"=s"},
		{"a legacy colour takes null as nothing", `[put("body", "text", "text", null, "red"), put("table", "cellPadding",
			"cellpadding", null), put("font", "color", "color", null), make("meta", { "http-equiv": "x" }).httpEquiv,
			make("col", { char: "c", charoff: "2" }).ch + make("col", { char: "c", charoff: "2" }).chOff].join("|")`,
			`""=,"red"=red|""=|""=|x|c2`},
		{"a flag is there or not", `[got("ol", "reversed", "reversed", null, "", "false", "reversed"), put("ol", "reversed",
			"reversed", true, 0, "x", "", null, NaN, {}), typeof make("hr").noShade, make("hr", { noshade: "" }).noShade,
			make("video", { muted: "" }).defaultMuted, make("option", { selected: "" }).defaultSelected].join("|")`,
			`false,true,true,true|""=true,null=false,""=true,null=false,null=false,null=false,""=true|boolean|true|true|true`},
		{"a whole number", `[got("li", "value", "value", null, "7", " \t-36x", "+5", "-0", "x", "", "1.9", "2147483647",
			"2147483648", "-2147483649", "\u00A07"), put("li", "value", "value", -36, 1.9, "12", 2147483648, NaN),
			make("ol").start, make("pre").width].join("|")`,
			`0,7,-36,5,0,0,0,1,2147483647,0,0,0|"-36"=-36,"1"=1,"12"=12,"-2147483648"=-2147483648,"0"=0|1|0`},
		{"a number that is not negative", `[got("input", "maxLength", "maxlength", null, "5", "-5", "x", "0", "2147483648"),
			put("input", "maxLength", "maxlength", 3, 0, -1, -2147483648), put("textarea", "minLength", "minlength", 2, -1)]
			.join("|")`, `-1,5,-1,-1,0,-1|"3"=3,"0"=0,IndexSizeError,IndexSizeError|"2"=2,IndexSizeError`},
		{"a count", `[got("img", "hspace", "hspace", null, "3", "-1", "+4", "-0", "2147483648", "x"), put("img", "hspace",
			"hspace", 5, 2147483647, 2147483648, 4294967295, "-0", -1), make("canvas").width, make("canvas").height,
			make("marquee").scrollAmount, make("marquee").scrollDelay, make("select").size].join("|")`,
			`0,3,0,4,0,0,0|"5"=5,"2147483647"=2147483647,"0"=0,"0"=0,"0"=0,"0"=0|300|150|6|85|0`},
		{"a count above zero", `[got("input", "size", "size", null, "5", "0", "-1", "x", "2147483648"), put("input", "size",
			"size", 3, 0, 2147483648), got("textarea", "cols", "cols", null, "0", "9"), put("textarea", "cols", "cols", 4, 0,
			2147483648), make("textarea").rows].join("|")`,
			`20,5,20,20,20,20|"3"=3,IndexSizeError,"20"=20|20,20,9|"4"=4,"20"=20,"20"=20|2`},
		{"a count within bounds", `[got("td", "colSpan", "colspan", null, "0", "5", "1000", "1001", "x", "-3"), got("th",
			"rowSpan", "rowspan", null, "0", "65534", "65535", "x"), put("td", "colSpan", "colspan", 0, 2000, 2147483648),
			got("col", "span", "span", null, "0", "7", "5000")].join("|")`,
			`1,1,5,1000,1000,1,1|1,0,65534,65534,1|"0"=1,"2000"=1000,"1"=1|1,1,7,1000`},
		{"a number with a fraction", `[got("progress", "max", "max", null, "2.5", "0", "-1", "x", ".5", "1e2", "1.", " 3 ",
			"1.8e308"), put("progress", "max", "max", 2.5, 0, -1, 1e-10, Infinity, NaN), got("meter", "high", "high", "0.5",
			"-.5e1", "+2"), put("meter", "low", "low", -0, 1.5, "x")].join("|")`,
			`1,2.5,1,1,1,0.5,100,1,3,1|"2.5"=2.5,"2.5"=2.5,"2.5"=2.5,"1e-10"=1e-10,TypeError,TypeError|0.5,-5,2|` +
				`"0"=0,"1.5"=1.5,TypeError`},
		{"one word of a few", `[got("p", "dir", "dir", null, "", "RTL", "auto", "x", "ltr\0"), put("p", "dir", "dir", "RTL",
			"x", null), got("form", "method", "method", null, "POST", "x", "dialog", ""), got("form", "enctype", "enctype",
			null, "text/plain", "x"), make("form", { enctype: "TEXT/PLAIN" }).encoding, got("input", "type", "type", null,
			"CHECKBOX", "x", "datetime-local"), got("button", "type", "type", null, "reset", "x", "BUTTON"), got("td", "scope",
			"scope", null, "Row", "x"), got("link", "as", "as", null, "Font", "x"), got("a", "referrerPolicy",
			"referrerpolicy", null, "ORIGIN", "x", ""), got("img", "decoding", "decoding", null, "SYNC", "x"), got("img",
			"loading", "loading", null, "LAZY", "x"), got("track", "kind", "kind", null, "captions", "x"), got("input",
			"formMethod", "formmethod", null, "POST", "x"), got("button", "formEnctype", "formenctype", null, "x"),
			got("form", "autocomplete", "autocomplete", null, "OFF", "x"), got("p", "dir", "dir", "\u212A")].join("|")`,
			",,rtl,auto,,|\"RTL\"=rtl,\"x\"=,\"null\"=|get,post,get,dialog,get|application/x-www-form-urlencoded," +
				"text/plain,application/x-www-form-urlencoded|text/plain|text,checkbox,text,datetime-local|submit,reset," +
				"submit,button|,row,|,font,|,origin,,|auto,sync,auto|eager,lazy,eager|subtitles,captions,metadata|,post,get|" +
				",application/x-www-form-urlencoded|on,off,on|"},
		{"a word that may be missing", `[got("img", "crossOrigin", "crossorigin", null, "", "USE-CREDENTIALS", "x"),
			put("script", "crossOrigin", "crossorigin", "anonymous", "", "x", null, undefined), typeof make("link")
			.crossOrigin].join("|")`,
			`,anonymous,use-credentials,anonymous|"anonymous"=anonymous,""=anonymous,"x"=anonymous,null=null,null=null|` +
				"object"},
		{"an address is read whole", `[got("a", "href", "href", null, "", "x?y#z", "//o.test/p", "http://[", " /a b "),
			put("a", "href", "href", "/n", ""), make("form").action, got("form", "action", "action", "", "post"),
			make("button").formAction, got("input", "formAction", "formaction", "", "f"), got("img", "longDesc", "longdesc",
			null, "d"), got("q", "cite", "cite", null, "c"), got("del", "cite", "cite", "c"), got("object", "data", "data",
			"d.swf"), got("video", "poster", "poster", null, "p.png"), make("form", { action: "x" }).getAttribute("action")]
			.join("|")`,
			",https://site.test/dir/page.html?q=1,https://site.test/dir/x?y#z,https://o.test/p,http://[," +
				"https://site.test/a%20b|\"/n\"=https://site.test/n,\"\"=https://site.test/dir/page.html?q=1|" +
				"https://site.test/dir/page.html?q=1#top|https://site.test/dir/page.html?q=1#top," +
				"https://site.test/dir/post|https://site.test/dir/page.html?q=1#top|" +
				"https://site.test/dir/page.html?q=1#top,https://site.test/dir/f|,https://site.test/dir/d|," +
				"https://site.test/dir/c|https://site.test/dir/c|https://site.test/dir/d.swf|," +
				"https://site.test/dir/p.png|x"},
	} {
		if got := s.eval(reflecting + c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}

func TestEveryElementOfHTMLHasItsAttributes(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><p id="p">p</p><a id="a" href="/x">a</a><input id="i" value="v">`)
	for _, c := range []struct{ name, script, want string }{
		{"what every element says of itself", `(() => { const e = make("p", { title: "t", lang: "en", accesskey: "k",
			autofocus: "", inert: "", id: "i", class: "c d", slot: "s" });
			const was = [e.title, e.lang, e.accessKey, e.autofocus, e.inert, e.id, e.className, e.slot];
			e.title = 1; e.lang = "fr"; e.accessKey = "z"; e.autofocus = false; e.inert = 0; e.id = "j"; e.className = "x";
			e.slot = "u";
			return was.concat(e.outerHTML).join() })()`,
			`t,en,k,true,true,i,c d,s,<p title="1" lang="fr" accesskey="z" id="j" class="x" slot="u"></p>`},
		{"hidden may wait to be found", `[got("p", "hidden", "hidden", null, "", "x", "UNTIL-FOUND"), put("p", "hidden",
			"hidden", true, false, "until-Found", "", "x", 0, NaN, null, 1)].join("|")`,
			`false,true,true,until-found|""=true,null=false,"until-found"=until-found,null=false,""=true,null=false,` +
				`null=false,null=false,""=true`},
		{"what may be dragged, translated, checked and edited", `[got("p", "draggable", "draggable", null, "true", "FALSE",
			"x"), make("img").draggable, make("a", { href: "x" }).draggable, make("a").draggable, put("p", "draggable",
			"draggable", true, false, "x"), got("p", "translate", "translate", null, "no", "", "YES", "x"), put("p",
			"translate", "translate", false, true), (() => { const d = make("div", { translate: "no" }), i = make("i");
			d.append(i); return i.translate })(), got("p", "spellcheck", "spellcheck", null, "false", "", "x"), put("p",
			"spellcheck", "spellcheck", false, true), got("p", "contentEditable", "contenteditable", null, "", "TRUE",
			"false", "plaintext-only", "x"), put("p", "contentEditable", "contenteditable", "TRUE", "false", "inherit",
			"plaintext-only", "x"), make("p", { contenteditable: "" }).isContentEditable].join("|")`,
			`false,true,false,false|true|true|false|"true"=true,"false"=false,"true"=true|true,false,true,true,true|` +
				`"no"=false,"yes"=true|false|false,false,true,false|"false"=false,"true"=true|` +
				`inherit,true,true,false,plaintext-only,inherit|"true"=true,"false"=false,null=inherit,` +
				`"plaintext-only"=plaintext-only,SyntaxError|false`},
		{"hints for what is typed", `[got("p", "autocapitalize", "autocapitalize", null, "ON", "off", "words", "x",
			"Characters", "sentences", "none"), got("p", "enterKeyHint", "enterkeyhint", null, "GO", "x"), got("p",
			"inputMode", "inputmode", null, "Numeric", "x"), got("p", "popover", "popover", null, "", "AUTO", "manual", "x"),
			put("p", "popover", "popover", "auto", null)].join("|")`,
			`,sentences,none,words,,characters,sentences,none|,go,|,numeric,|,auto,auto,manual,manual|"auto"=auto,null=null`},
		{"where the tab key stops", `[make("p").tabIndex, make("a").tabIndex, document.getElementById("a").tabIndex,
			make("button").tabIndex, make("input").tabIndex, make("select").tabIndex, make("textarea").tabIndex,
			make("iframe").tabIndex, make("summary").tabIndex, make("p",
			{ tabindex: "3" }).tabIndex, make("a", { tabindex: "x", href: "y" }).tabIndex, put("p", "tabIndex", "tabindex",
			-1, 2.7), document.createElementNS("http://www.w3.org/2000/svg", "g").tabIndex].join("|")`,
			`-1|-1|0|0|0|0|0|0|0|3|0|"-1"=-1,"2"=2|-1`},
		{"a nonce is kept off the markup once a script sets it", `(() => { const e = make("script", { nonce: "a" });
			const seen = [e.nonce]; e.nonce = "b"; seen.push(e.nonce, e.getAttribute("nonce")); e.setAttribute("nonce", "c");
			seen.push(e.nonce); return seen.join() })()`, "a,b,a,c"},
		{"a document says its root's direction and its body's colours", `(() => { const d = document, seen = [d.dir,
			d.fgColor, d.bgColor]; d.dir = "RTL"; d.fgColor = "red"; d.linkColor = "blue"; d.vlinkColor = null;
			d.alinkColor = "a"; d.bgColor = "white";
			seen.push(d.dir, d.documentElement.getAttribute("dir"), d.fgColor, d.linkColor, d.body.outerHTML.slice(0, 68));
			const bare = new Document(); bare.dir = "ltr"; seen.push(bare.dir, bare.bgColor); return seen.join("|") })()`,
			`|||rtl|RTL|red|blue|<body text="red" link="blue" vlink="" alink="a" bgcolor="white"><p i||`},
		{"each element is of its interface", `[["area", "HTMLAreaElement"], ["map", "HTMLMapElement"], ["param",
			"HTMLParamElement"], ["track", "HTMLTrackElement"], ["ins", "HTMLModElement"], ["del", "HTMLModElement"],
			["caption", "HTMLTableCaptionElement"], ["col", "HTMLTableColElement"], ["colgroup", "HTMLTableColElement"],
			["datalist", "HTMLDataListElement"], ["marquee", "HTMLMarqueeElement"], ["frameset", "HTMLFrameSetElement"],
			["frame", "HTMLFrameElement"], ["dir", "HTMLDirectoryElement"], ["listing", "HTMLPreElement"], ["xmp",
			"HTMLPreElement"], ["acronym", "HTMLElement"], ["tt", "HTMLElement"], ["strike", "HTMLElement"], ["rb",
			"HTMLElement"], ["section", "HTMLElement"], ["undefinedelement", "HTMLUnknownElement"], ["blink",
			"HTMLUnknownElement"], ["x-own", "HTMLElement"]].map(([tag, name]) => { const e = document.createElement(tag);
			return Object.getPrototypeOf(e) === window[name].prototype && e instanceof HTMLElement &&
				window[name].name === name }).join()`,
			"true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true," +
				"true,true,true"},
		{"an attribute is an accessor of its interface", `[Object.getOwnPropertyDescriptor(HTMLElement.prototype, "title")
			.get !== undefined, "title" in HTMLParagraphElement.prototype, Object.hasOwn(HTMLParagraphElement.prototype,
			"title"), Object.hasOwn(HTMLTableCellElement.prototype, "colSpan"), "colSpan" in HTMLElement.prototype,
			Object.hasOwn(document.getElementById("p"), "title"), Object.keys(HTMLMapElement.prototype)].join()`,
			"true,true,false,true,false,false,name"},
		{"what light already drove still drives it", `(() => { const i = document.getElementById("i"); i.defaultValue = "d";
			const a = document.getElementById("a"), box = make("input", { type: "checkbox" }); box.checked = true;
			i.value = "typed"; i.disabled = true; i.type = "search";
			return [i.value, i.getAttribute("value"), i.defaultValue, i.disabled, i.matches(":disabled"), i.type, a.href,
				a.pathname, box.checked, box.hasAttribute("checked"), box.defaultChecked, make("img", { width: "40" }).width,
				make("option", { label: "l" }).label, make("progress", { value: "0.5" }).value].join() })()`,
			"typed,d,d,true,true,search,https://site.test/x,/x,true,false,false,40,l,0.5"},
	} {
		if got := s.eval(reflecting + c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}
