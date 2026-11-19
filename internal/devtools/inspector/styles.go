package inspector

import (
	"fmt"
	"math"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"
	"github.com/lantervale/ember/ui/css"
)

// element is the selected node when it is an element of the page the
// tree shows, nil otherwise.
func (in *Inspector) element() *ui.Node {
	n := in.state.selected
	if n == nil || n.IsText() || in.isHead(n) || in.items[n] == nil || in.page.Doc == nil {
		return nil
	}
	return n
}

// restyle fills the rules and the computed pane again when what they
// are made from changed: the element selected, its attributes and those
// of what stands over it, and its box.
func (in *Inspector) restyle() {
	var b strings.Builder
	if n := in.element(); n != nil {
		fmt.Fprintf(&b, "%p", n)
		for at := n; at != nil; at = at.Parent() {
			b.WriteString(" #" + at.ID() + " ." + strings.Join(at.Classes(), " ") + " ;" + at.Style())
			for _, a := range at.Attrs() {
				b.WriteString(" " + a.Name + "=" + a.Value)
			}
		}
		boxes, _ := in.docOf(n).BoxesOf(n)
		fmt.Fprint(&b, boxes)
	}
	if styled := b.String(); styled != in.styled || styled == "" {
		in.styled = styled
		in.style()
	}
}

// style fills the rules, the box and the computed pane for the element
// selected.
func (in *Inspector) style() {
	n := in.element()
	if n == nil {
		why := "Select an element to see its styles."
		if in.state.selected != nil {
			why = "A text has no style of its own."
		}
		fill(in.rules, ui.Div(ui.Class("dt-insp-none"), ui.Text(why)))
		fill(in.box)
		fill(in.computed)
		return
	}
	in.listRules(n)
	in.showBox(n)
	in.listComputed(n)
}

// listRules fills the rules pane: what applies to n, the strongest
// first, ember's own sheet last.
func (in *Inspector) listRules(n *ui.Node) {
	filter := strings.ToLower(strings.TrimSpace(in.state.Rules))
	var out []*ui.Node
	for _, m := range in.docOf(n).MatchedRules(n) {
		var decls []*ui.Node
		for _, d := range m.Declarations {
			if filter != "" && !strings.Contains(strings.ToLower(d.Name+": "+d.Value), filter) {
				continue
			}
			value := d.Value
			if d.Important {
				value += " !important"
			}
			decl := ui.Div(ui.Class("dt-insp-decl"), span("dt-insp-name", d.Name), span("dt-insp-p", ": "))
			decl.Append(valued(d.Name, value)...)
			decl.Append(span("dt-insp-p", ";"))
			toggle(decl, "overridden", d.Overridden)
			decls = append(decls, decl)
		}
		if filter != "" && len(decls) == 0 {
			continue
		}
		rule := ui.Div(ui.Class("dt-insp-rule"))
		toggle(rule, "dt-insp-agent", m.Origin == css.Agent)
		if m.Layer != "" {
			rule.Append(ui.Div(ui.Class("dt-insp-at"), ui.Text("@layer "+m.Layer)))
		}
		for _, media := range m.Media {
			rule.Append(ui.Div(ui.Class("dt-insp-at"), ui.Text("@media "+media)))
		}
		selector, from := m.Selector, in.source(m)
		switch m.Origin {
		case css.Inline:
			selector = "element"
		case css.Hint:
			selector = "attributes"
		case css.Author, css.Agent:
		}
		rule.Append(ui.Div(ui.Class("dt-insp-head"),
			ui.Span(ui.Class("dt-insp-selector"), ui.Text(selector), span("dt-insp-p", " {")), from))
		rule.Append(decls...)
		rule.Append(ui.Div(ui.Class("dt-insp-p"), ui.Text("}")))
		out = append(out, rule)
	}
	if len(out) == 0 {
		out = append(out, ui.Div(ui.Class("dt-insp-none"), ui.Text("No rule passes the filter.")))
	}
	fill(in.rules, out...)
}

// source names where a match is from: its sheet's file and the line its
// rule opens at, which opens that sheet's source when the screen shows
// sources.
func (in *Inspector) source(m css.Match) *ui.Node {
	switch m.Origin {
	case css.Inline:
		return span("dt-insp-from", "inline")
	case css.Hint:
		return span("dt-insp-from", "attributes")
	case css.Agent:
		return span("dt-insp-from", "(user agent)")
	case css.Author:
	}
	address := m.Sheet.From()
	name := m.Sheet.Name
	if at, err := url.Parse(address); err == nil && address != "" {
		if name = path.Base(at.Path); name == "." || name == "/" {
			name = at.Host
		}
	}
	from := span("dt-insp-from", name+":"+strconv.Itoa(m.Pos.Line))
	if address != "" && in.host.Source != nil {
		from.AddClass("dt-insp-link")
		ui.TabIndex(0).Apply(from)
		ui.OnClick(in.guarded(func() { in.host.Source(address, m.Pos.Line) })).Apply(from)
	}
	return from
}

// valued is a value as a pane shows it: a swatch before it when it is,
// or holds, a colour light can read.
func valued(name, value string) []*ui.Node {
	text := span("dt-insp-value", value)
	if r, g, b, a, ok := colourIn(name, value); ok {
		return []*ui.Node{ui.Span(ui.Class("dt-insp-swatch"),
			ui.Style(fmt.Sprintf("background-color: rgba(%d, %d, %d, %.3g)", r, g, b, a))), text}
	}
	return []*ui.Node{text}
}

//nolint:gochecknoglobals // tables
var (
	hexColour = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	rgbColour = regexp.MustCompile(`rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)\s*(?:[,/]\s*([\d.]+)(%?)\s*)?\)`)
	named     = map[string][3]uint8{
		"black": {0, 0, 0}, "white": {255, 255, 255}, "red": {255, 0, 0}, "green": {0, 128, 0}, "blue": {0, 0, 255},
		"yellow": {255, 255, 0}, "orange": {255, 165, 0}, "purple": {128, 0, 128}, "gray": {128, 128, 128},
		"grey": {128, 128, 128}, "silver": {192, 192, 192}, "maroon": {128, 0, 0}, "navy": {0, 0, 128},
		"teal": {0, 128, 128}, "olive": {128, 128, 0}, "lime": {0, 255, 0}, "aqua": {0, 255, 255},
		"cyan": {0, 255, 255}, "fuchsia": {255, 0, 255}, "magenta": {255, 0, 255}, "pink": {255, 192, 203},
		"brown": {165, 42, 42}, "gold": {255, 215, 0}, "indigo": {75, 0, 130}, "violet": {238, 130, 238},
		"crimson": {220, 20, 60}, "tomato": {255, 99, 71}, "coral": {255, 127, 80}, "salmon": {250, 128, 114},
		"khaki": {240, 230, 140}, "beige": {245, 245, 220}, "ivory": {255, 255, 240}, "tan": {210, 180, 140},
		"lightgray": {211, 211, 211}, "lightgrey": {211, 211, 211}, "darkgray": {169, 169, 169},
		"darkgrey": {169, 169, 169}, "dimgray": {105, 105, 105}, "whitesmoke": {245, 245, 245},
		"steelblue": {70, 130, 180}, "royalblue": {65, 105, 225}, "skyblue": {135, 206, 235},
		"rebeccapurple": {102, 51, 153}, "darkblue": {0, 0, 139}, "darkred": {139, 0, 0},
		"darkgreen": {0, 100, 0},
	}
)

// colourIn is the first colour a value holds that light reads: a hex
// one, an rgb() of whole numbers, or the name of one where the property
// takes a colour.
func colourIn(name, value string) (uint8, uint8, uint8, float64, bool) {
	if hex := hexColour.FindString(value); hex != "" {
		return hexed(hex[1:])
	}
	if m := rgbColour.FindStringSubmatch(value); m != nil {
		var c [3]uint8
		for i := range c {
			v, _ := strconv.Atoi(m[i+1])
			c[i] = uint8(min(v, 255)) //nolint:gosec // kept under 256
		}
		a := 1.0
		if m[4] != "" {
			a, _ = strconv.ParseFloat(m[4], 64)
			if m[5] == "%" {
				a /= 100
			}
		}
		return c[0], c[1], c[2], min(max(a, 0), 1), true
	}
	if !strings.Contains(name, "color") && name != "background" && !strings.HasPrefix(name, "border") &&
		name != "outline" && !strings.HasPrefix(name, "--") {
		return 0, 0, 0, 0, false
	}
	for _, word := range strings.Fields(strings.ToLower(value)) {
		if c, ok := named[word]; ok {
			return c[0], c[1], c[2], 1, true
		}
	}
	return 0, 0, 0, 0, false
}

func hexed(digits string) (uint8, uint8, uint8, float64, bool) {
	if len(digits) == 3 || len(digits) == 4 {
		var wide strings.Builder
		for _, d := range digits {
			wide.WriteRune(d)
			wide.WriteRune(d)
		}
		digits = wide.String()
	}
	if len(digits) != 6 && len(digits) != 8 {
		return 0, 0, 0, 0, false
	}
	v, err := strconv.ParseUint(digits, 16, 32)
	if err != nil {
		return 0, 0, 0, 0, false
	}
	a := 1.0
	if len(digits) == 8 {
		a = float64(v&0xff) / 255
		v >>= 8
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v), a, true //nolint:gosec // a byte each
}

// listComputed fills the computed pane: the properties a declaration
// gave n, or all of them, by name.
func (in *Inspector) listComputed(n *ui.Node) {
	filter := strings.ToLower(strings.TrimSpace(in.state.Computed))
	var out []*ui.Node
	for _, p := range in.docOf(n).ComputedStyle(n) {
		if !p.Declared && !in.state.All ||
			filter != "" && !strings.Contains(strings.ToLower(p.Name+": "+p.Value), filter) {
			continue
		}
		row := ui.Div(ui.Class("dt-insp-prop"), span("dt-insp-name", p.Name))
		toggle(row, "dt-insp-initial", !p.Declared)
		value := ui.Span(ui.Class("dt-insp-computed"))
		value.Append(valued(p.Name, p.Value)...)
		row.Append(value)
		out = append(out, row)
	}
	if len(out) == 0 {
		out = append(out, ui.Div(ui.Class("dt-insp-none"), ui.Text("No property passes the filter.")))
	}
	fill(in.computed, out...)
}

// number is a size as the box shows it: whole when it is, with one
// decimal otherwise.
func number(v float32) string {
	if v == float32(math.Trunc(float64(v))) {
		return strconv.Itoa(int(v))
	}
	return strconv.FormatFloat(float64(v), 'f', 1, 32)
}

// showBox draws the box of n: its margin, border and padding round its
// content's size, then where its border box is and how it is
// positioned.
func (in *Inspector) showBox(n *ui.Node) {
	doc := in.docOf(n)
	boxes, ok := doc.BoxesOf(n)
	if !ok {
		fill(in.box, ui.Div(ui.Class("dt-insp-none"), ui.Text("This element has no box: it is not shown.")))
		return
	}
	inner := ui.Div(ui.ID("dt-insp-box-content"),
		ui.Text(number(boxes.Content.W)+" × "+number(boxes.Content.H)))
	for _, level := range []struct {
		name          string
		outer, within geom.Rect
	}{{"padding", boxes.Padding, boxes.Content}, {"border", boxes.Border, boxes.Padding},
		{"margin", boxes.Margin, boxes.Border}} {
		side := func(class string, v float32) *ui.Node { return span(class, number(v)) }
		inner = ui.Div(ui.ID("dt-insp-box-"+level.name), ui.Class("dt-insp-edges"),
			span("dt-insp-edge-name", level.name),
			side("dt-insp-top", level.within.Y-level.outer.Y),
			side("dt-insp-left", level.within.X-level.outer.X),
			inner,
			side("dt-insp-right", level.outer.X+level.outer.W-level.within.X-level.within.W),
			side("dt-insp-bottom", level.outer.Y+level.outer.H-level.within.Y-level.within.H))
	}
	position, _ := doc.ComputedValue(n, "position")
	sizing, _ := doc.ComputedValue(n, "box-sizing")
	fill(in.box, inner, ui.Div(ui.ID("dt-insp-box-at"),
		ui.Text(fmt.Sprintf("%s × %s at %s, %s · %s · %s", number(boxes.Border.W), number(boxes.Border.H),
			number(boxes.Border.X), number(boxes.Border.Y), position, sizing))))
}
