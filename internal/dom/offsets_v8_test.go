//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import "testing"

// offsetPage is a page of the offset parents CSSOM View names: a
// positioned ancestor, a table and its cells, a transformed box, and
// the body for the rest.
const offsetPage = `<head><style>body { margin: 8px }
	.rel { position: relative; margin: 20px; border: solid 4px; padding: 3px; width: 100px; height: 100px }</style></head>
	<body><div id="outer" style="margin: 5px; border: 2px solid; padding: 6px">
	<div id="rel" class="rel">
	<div id="abs" style="position: absolute; left: 10px; top: 7px; width: 5px; height: 5px"></div>
	<p id="inner" style="margin: 0; height: 10px">x</p>
	<div id="fixed" style="position: fixed; top: 3px; left: 4px; width: 1px; height: 1px"></div></div>
	<table id="table" style="border-spacing: 0; border: 3px solid">` +
	`<tr><td id="cell" style="padding: 5px; border: 1px solid">` +
	`<div id="incell" style="height: 4px"></div></td></tr></table>
	<div id="none" style="display: none"></div>
	<div id="turned" style="transform: translateX(0); border: 1px solid; padding: 2px">
	<div id="inturned" style="height: 4px"></div></div>
	</div></body>`

// TestOffsetsAreFromTheOffsetParent checks offsetParent, offsetLeft and
// offsetTop against what headless Chrome 158 reads of offsetPage: an
// element's offsets are from its offset parent's padding edge, and
// from the page where that parent is the body or there is none.
func TestOffsetsAreFromTheOffsetParent(t *testing.T) {
	s := run(t, offsetPage)
	const read = `["outer", "rel", "abs", "inner", "fixed", "table", "cell", "incell", "none", "turned", "inturned"]
		.map(id => { const e = document.getElementById(id); const p = e.offsetParent;
			return id + ":" + (p ? (p.id || p.localName) : "null") + "," + e.offsetLeft + "," + e.offsetTop; }).join(" ") +
		" body:" + document.body.offsetParent + " html:" + document.documentElement.offsetParent`
	const chrome = "outer:body,13,8 rel:body,41,36 abs:rel,10,7 inner:rel,3,3 fixed:null,4,3 table:body,21,170 " +
		"cell:table,0,0 incell:cell,5,5 none:null,0,0 turned:body,21,192 inturned:turned,2,2 body:null html:null"
	if got := s.eval(read); got != chrome {
		t.Errorf("offsets read\n%s\nwant, as Chrome reads them,\n%s", got, chrome)
	}
	s.quiet()
}
