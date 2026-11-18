package web //nolint:testpackage // uses the package's unexported test helpers (built, box)

import (
	"math"
	"testing"

	"github.com/lantervale/ember/geom"
)

// searchPage is a search engine's page as it is sent to a browser it does not know: a
// table laid out by its attributes in a center, hidden fields, sizes in points.
const searchPage = `<body bgcolor="#fff" style="margin: 0"><center><form action="/search" id="form">
<table cellpadding="0" cellspacing="0" id="table"><tr valign="top">
<td width="25%" id="left">&nbsp;</td>
<td align="center" nowrap=""><input name="ie" value="ISO-8859-1" type="hidden" id="ie"><input value="fr"
name="hl" type="hidden" id="hl"><div style="height: 32px"><input id="q" name="q" style="width: 496px; height: 25px"
></div><input value="Search" type="submit" id="go"></td>
<td width="25%" align="left" nowrap="" id="right"><a href="/advanced" id="more">More</a></td>
</tr></table><input id="gbv" name="gbv" type="hidden" value="1"></form>
<p style="font-size: 8pt; margin: 0" id="small">Terms</p></center></body>`

func TestAPageLaidOutByItsAttributesShowsAsWritten(t *testing.T) {
	doc, _ := built(t, searchPage, nil)
	doc.Layout(geom.V2(1024, 600))
	for _, id := range []string{"ie", "hl", "gbv"} {
		if at, ok := doc.BoxOf(doc.ByID(id)); ok {
			t.Errorf("the hidden field #%s has a box, %v", id, at)
		}
	}
	table, field, submit := box(t, doc, "table"), box(t, doc, "q"), box(t, doc, "go")
	for id, r := range map[string]geom.Rect{"the table": table, "the field": field, "the button": submit} {
		if centre := r.X + r.W/2; math.Abs(float64(centre-512)) > 1 {
			t.Errorf("%s is centred at %v, want the middle of the page, 512", id, centre)
		}
	}
	if left, right := box(t, doc, "left"), box(t, doc, "right"); table.W < 2*field.W ||
		math.Abs(float64(left.W-table.W/4)) > 1 || math.Abs(float64(right.W-table.W/4)) > 1 {
		t.Errorf("the side cells are %v and %v wide, want a quarter each of a table %v wide", left.W, right.W, table.W)
	}
	if more := box(t, doc, "more"); more.Y > field.Y+2 || more.X < field.X+field.W {
		t.Errorf("the link stands at %v, want the top of its cell, right of the field at %v", more, field)
	}
	if small := box(t, doc, "small"); small.H > 14 {
		t.Errorf("text of 8pt makes a line %v high, want one of 10.67px text", small.H)
	}
	if warns := doc.Warnings(); len(warns) > 0 {
		t.Errorf("the page warns: %v", warns)
	}
}
