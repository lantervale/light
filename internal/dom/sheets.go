package dom

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"
	"github.com/lantervale/ember/ui/css"
)

// sheets are the page's stylesheets as scripts change them: one for
// each style element and each link to a stylesheet in the page, in the
// page's order, then those scripts made themselves (new CSSStyleSheet).
// The tree is read again for them once a frame, and before a script
// reads what a change it made may have moved (stale).
type sheets struct {
	by  map[*html.Node]*sheet
	ids map[int]*sheet
	// made are the sheets scripts made, adopted or not.
	made  []*sheet
	last  int
	stale bool
	// loading counts the links whose sheets are on their way.
	loading int
}

// sheet is one stylesheet: of the element owner, or a script's own.
type sheet struct {
	id    int
	owner *html.Node
	// has is the Document's sheet, nil for a link not loaded yet.
	has *ui.Sheet
	// text is the text a style element's sheet was last read from, and
	// version counts the times its rules were read again.
	text    string
	version int
	// href is the address a link's sheet is asked or was read from,
	// from where it came from after its redirects, and stop forgets a
	// request on its way.
	href, from string
	loaded     bool
	stop       func()
	// off is whether a script disabled it, adopted whether the page
	// uses one a script made.
	off, adopted bool
}

func newSheets() sheets {
	return sheets{by: map[*html.Node]*sheet{}, ids: map[int]*sheet{}}
}

func (s *sheets) add(owner *html.Node) *sheet {
	s.last++
	st := &sheet{id: s.last, owner: owner}
	s.ids[st.id] = st
	if owner != nil {
		s.by[owner] = st
	}
	return st
}

// styles reports whether n is an element whose text is a stylesheet.
func styles(n *html.Node) bool {
	if n.Type != html.ElementNode || n.DataAtom != atom.Style || n.Namespace != "" {
		return false
	}
	kind, _ := attribute(n, "type")
	return kind == "" || strings.EqualFold(strings.TrimSpace(kind), "text/css")
}

// links reports whether n is a link to a stylesheet the page uses.
func links(n *html.Node) bool {
	if n.Type != html.ElementNode || n.DataAtom != atom.Link || n.Namespace != "" {
		return false
	}
	rel, _ := attribute(n, "rel")
	names := strings.Fields(strings.ToLower(rel))
	_, off := attribute(n, "disabled")
	return slices.Contains(names, "stylesheet") && !slices.Contains(names, "alternate") && !off
}

// seedSheets takes the sheets the page was loaded with as those of
// their elements.
func (r *Runtime) seedSheets() {
	for n, styled := range r.tree.Sheets() {
		st := r.sheets.add(n)
		st.has, st.text, st.loaded = styled.Sheet, styled.Text, true
		if n.DataAtom == atom.Link {
			href, _ := attribute(n, "href")
			st.href, st.from = r.resolve(href), styled.Sheet.From()
		}
	}
	r.sheets.stale = true
}

// syncSheets reads the page's style elements and links again, and has
// the Document's sheets follow them: one added for an element that
// entered the page, removed for one that left, read again for a text
// that changed, fetched for an address, all in the page's order.
func (r *Runtime) syncSheets() {
	s := &r.sheets
	s.stale = false
	var found []*html.Node
	walk(r.tree.Root(), func(n *html.Node) bool {
		if styles(n) || links(n) {
			found = append(found, n)
		}
		return true
	})
	var next *ui.Sheet
	if len(s.made) > 0 {
		next = s.made[0].has
	}
	wanted := make([]*ui.Sheet, 0, len(found))
	for _, n := range slices.Backward(found) {
		st := s.by[n]
		if st == nil {
			st = s.add(n)
		}
		if n.DataAtom == atom.Style {
			r.restyle(st, next)
		} else {
			r.relink(st, next)
		}
		if st.has != nil {
			next = st.has
			wanted = append(wanted, st.has)
		}
	}
	for n, st := range s.by {
		if !slices.Contains(found, n) {
			r.drop(st)
		}
	}
	slices.Reverse(wanted)
	r.reorder(wanted)
}

// restyle has a style element's sheet say what the element holds.
func (r *Runtime) restyle(st *sheet, next *ui.Sheet) {
	text := textOf(st.owner)
	media, _ := attribute(st.owner, "media")
	switch {
	case st.has == nil:
		st.has = r.doc.InsertSheet(next, text, ui.From(r.page.Address), ui.Media(media))
		st.has.SetDisabled(st.off)
		st.text = text
		st.version++
	case text != st.text:
		st.has.Replace(text)
		st.text = text
		st.version++
	}
	st.has.SetMedia(media)
}

// relink has a link's sheet be the one at its address: fetched when
// the address is new, the one before kept until it came.
func (r *Runtime) relink(st *sheet, next *ui.Sheet) {
	n := st.owner
	href, _ := attribute(n, "href")
	address := r.resolve(href)
	if strings.TrimSpace(href) == "" || address == "" {
		r.unlink(st)
		return
	}
	media, _ := attribute(n, "media")
	if address != st.href {
		r.unlinking(st)
		st.href, st.loaded = address, false
		r.sheets.loading++
		st.stop = r.request(Request{Method: methodGet, Address: address, Sheet: true}, func(answer Response) {
			if r.sheets.by[n] != st || st.href != address || st.stop == nil {
				return
			}
			st.stop = nil
			r.sheets.loading--
			if answer.Err != nil || answer.Status < 200 || answer.Status > 299 {
				r.say("warn", "A stylesheet was not loaded: "+failure(answer), address)
				r.fire(n, "error", nil)
				return
			}
			if st.has != nil {
				st.has.Remove()
				st.has = nil
			}
			st.text, st.from, st.loaded = answer.Text, answer.Address, true
			r.syncSheets()
			r.fire(n, "load", nil)
		})
	}
	if st.loaded && st.has == nil {
		st.has = r.doc.InsertSheet(next, st.text, ui.From(st.from), ui.Media(media))
		st.has.SetDisabled(st.off)
		st.version++
	}
	if st.has != nil {
		st.has.SetMedia(media)
	}
}

// unlink leaves a link with no sheet, and nothing on its way.
func (r *Runtime) unlink(st *sheet) {
	r.unlinking(st)
	if st.has != nil {
		st.has.Remove()
		st.has = nil
	}
	st.href, st.loaded = "", false
}

// unlinking forgets the request of a link's sheet on its way.
func (r *Runtime) unlinking(st *sheet) {
	if st.stop != nil {
		st.stop()
		st.stop = nil
		r.sheets.loading--
	}
}

// drop forgets the sheet of an element that left the page.
func (r *Runtime) drop(st *sheet) {
	r.unlink(st)
	delete(r.sheets.by, st.owner)
	delete(r.sheets.ids, st.id)
}

// reorder puts the Document's sheets of the page's elements in the
// order wanted, when an element moved among them: each is added again
// with its rules as they stand.
func (r *Runtime) reorder(wanted []*ui.Sheet) {
	var has []*ui.Sheet
	for _, s := range r.doc.StyleSheets() {
		if slices.Contains(wanted, s) {
			has = append(has, s)
		}
	}
	if slices.Equal(has, wanted) {
		return
	}
	var before *ui.Sheet
	if len(r.sheets.made) > 0 {
		before = r.sheets.made[0].has
	}
	for _, has := range slices.Backward(wanted) {
		st := r.sheetOf(has)
		st.has = r.again(has, before)
		before = st.has
	}
}

func (r *Runtime) sheetOf(has *ui.Sheet) *sheet {
	for _, st := range r.sheets.ids {
		if st.has == has {
			return st
		}
	}
	return nil
}

// again is the sheet old added again before another, with its rules,
// its media and whether it is disabled.
func (r *Runtime) again(old, before *ui.Sheet) *ui.Sheet {
	fresh := r.doc.InsertSheet(before, old.Text(), ui.From(old.From()), ui.Media(old.Media()))
	fresh.SetDisabled(old.Disabled())
	old.Remove()
	return fresh
}

// fresh has the sheets follow the page before a script reads them,
// when it changed the page since they last did.
func (r *Runtime) fresh() {
	if r.sheets.stale {
		r.syncSheets()
	}
}

// mutates are the operations that can change which sheets the page
// has, or what they say.
//
//nolint:gochecknoglobals // a constant set
var mutates = map[string]bool{
	"insert": true, opRemove: true, "setText": true, "setHTML": true, "setAttr": true, "removeAttr": true,
}

// sheetOps are the operations scripts read and change stylesheets by.
func (r *Runtime) sheetOps() {
	named := func(do func(st *sheet, a arguments) (any, error)) op {
		return func(a arguments) (any, error) {
			r.fresh()
			st := r.sheets.ids[int(a.number(0))]
			if st == nil || st.has == nil {
				return nil, errors.New("InvalidStateError: the stylesheet is no longer the page's")
			}
			return do(st, a)
		}
	}
	for name, do := range map[string]op{
		"sheet":         r.of(r.sheetOfNode),
		"sheets":        r.sheetList,
		"sheetState":    r.sheetState,
		"sheetRules":    named(func(st *sheet, _ arguments) (any, error) { return st.has.Rules(), nil }),
		"insertRule":    named(r.insertRule),
		"deleteRule":    named(r.deleteRule),
		"setRule":       named(r.setRule),
		"replaceSheet":  named(r.replaceSheet),
		"disableSheet":  named(r.disableSheet),
		"setSheetMedia": named(r.setSheetMedia),
		"newSheet":      r.newSheet,
		"adoptSheets":   r.adoptSheets,
		"shownSrc":      r.on(shownSrc),
		"computed":      r.on(r.computed),
		"computedStyle": r.on(r.computedStyle),
		"supports":      supports,
		"supportsWhen":  supportsWhen,
	} {
		r.ops[name] = do
	}
}

// shownSrc is the address of the picture an img shows, the one ui chose
// among those of its srcset and its picture's sources.
func shownSrc(n *ui.Node, _ arguments) (any, error) {
	src, _ := ui.PictureOf(n)
	return src, nil
}

// sheetOfNode is the sheet of a style element or a link in the page:
// its number and its version, null for none.
func (r *Runtime) sheetOfNode(n *html.Node, _ arguments) (any, error) {
	r.fresh()
	if st := r.sheets.by[n]; st != nil && st.has != nil {
		return []int{st.id, st.version}, nil
	}
	return nothing()
}

// sheetList are the numbers of the sheets of the page's elements, in
// the page's order.
func (r *Runtime) sheetList(arguments) (any, error) {
	r.fresh()
	ids := []int{}
	for _, has := range r.doc.StyleSheets() {
		if st := r.sheetOf(has); st != nil && st.owner != nil {
			ids = append(ids, st.id)
		}
	}
	return ids, nil
}

// sheetState is what scripts read of a sheet beside its rules: its
// version, its owner's number, whether it is disabled, its media and
// its address; null for one the page has no more.
func (r *Runtime) sheetState(a arguments) (any, error) {
	r.fresh()
	st := r.sheets.ids[int(a.number(0))]
	if st == nil || st.has == nil {
		return nothing()
	}
	href := ""
	if st.owner != nil && st.owner.DataAtom == atom.Link {
		href = st.href
	}
	return []any{st.version, r.ref(st.owner), st.has.Disabled(), st.has.Media(), href}, nil
}

func (r *Runtime) insertRule(st *sheet, a arguments) (any, error) {
	if err := st.has.InsertRule(int(a.number(2)), a.text(1)); err != nil {
		return nil, errors.New("SyntaxError: Failed to parse the rule '" + a.text(1) + "'.")
	}
	return nothing()
}

func (r *Runtime) deleteRule(st *sheet, a arguments) (any, error) {
	if err := st.has.DeleteRule(int(a.number(1))); err != nil {
		return nil, errors.New("IndexSizeError: " + err.Error())
	}
	return nothing()
}

// setRule gives a rule of the sheet another text, as a script that
// changed its selector or its style does.
func (r *Runtime) setRule(st *sheet, a arguments) (any, error) {
	at := int(a.number(1))
	if err := st.has.InsertRule(at, a.text(2)); err != nil {
		return nil, errors.New("SyntaxError: Failed to parse the rule '" + a.text(2) + "'.")
	}
	if err := st.has.DeleteRule(at + 1); err != nil {
		return nil, errors.New("IndexSizeError: " + err.Error())
	}
	return nothing()
}

func (r *Runtime) replaceSheet(st *sheet, a arguments) (any, error) {
	st.has.Replace(a.text(1))
	st.version++
	return st.version, nil
}

func (r *Runtime) disableSheet(st *sheet, a arguments) (any, error) {
	st.off = a.flag(1)
	st.has.SetDisabled(st.off || st.owner == nil && !st.adopted)
	return nothing()
}

func (r *Runtime) setSheetMedia(st *sheet, a arguments) (any, error) {
	st.has.SetMedia(a.text(1))
	return nothing()
}

// newSheet is a sheet a script made: the page uses it once adopted.
func (r *Runtime) newSheet(a arguments) (any, error) {
	st := r.sheets.add(nil)
	st.has = r.doc.AddSheet("", ui.From(r.page.Address), ui.Media(a.text(0)))
	st.off = a.flag(1)
	st.has.SetDisabled(true)
	r.sheets.made = append(r.sheets.made, st)
	return st.id, nil
}

// adoptSheets has the page use the sheets scripts made that are named,
// in that order, after those of its elements.
func (r *Runtime) adoptSheets(a arguments) (any, error) {
	ids, _ := a[0].([]any)
	var adopted []*sheet
	for _, id := range ids {
		number, _ := id.(float64)
		if st := r.sheets.ids[int(number)]; st != nil && st.owner == nil && !slices.Contains(adopted, st) {
			adopted = append(adopted, st)
		}
	}
	var order []*sheet
	for _, st := range r.sheets.made {
		st.adopted = slices.Contains(adopted, st)
		if st.adopted {
			order = append(order, st)
		}
	}
	if !slices.Equal(order, adopted) {
		for _, st := range adopted {
			st.has = r.again(st.has, nil)
			r.sheets.made = append(slices.DeleteFunc(r.sheets.made, func(o *sheet) bool { return o == st }), st)
		}
	}
	for _, st := range r.sheets.made {
		st.has.SetDisabled(st.off || !st.adopted)
	}
	return nothing()
}

// computed is one property of an element's computed style as scripts
// read it, null for an element ui has not styled or a name it does not
// compute.
func (r *Runtime) computed(n *ui.Node, a arguments) (any, error) {
	r.fresh()
	r.reflow(n)
	name := a.text(1)
	value, ok := r.doc.ComputedValue(n, name)
	if !ok {
		return nothing()
	}
	if name == "width" || name == "height" {
		if used, laid := r.used(n, name); laid {
			return used, nil
		}
	}
	return resolved(name, value, r.fontSizes(n, name)), nil
}

// reflow has ui style and lay the page out at once, as a browser does
// when a script reads a size: ui keeps the boxes when nothing changed
// since the last layout, and lays out what a script made, moved or
// restyled since.
func (r *Runtime) reflow(n *ui.Node) {
	if r.connected(n) {
		r.layout()
	}
}

// layout has ui style and lay the page out in its viewport.
func (r *Runtime) layout() {
	var size geom.Vec2
	if r.host.Viewport != nil {
		size.X, size.Y = r.host.Viewport()
	}
	r.doc.Layout(size.Scale(r.doc.Zoom()))
}

// used is the width or the height of an element as it is laid out, in
// px: its content's, or its border box's when that is what it sizes.
func (r *Runtime) used(n *ui.Node, name string) (string, bool) {
	boxes, laid := r.doc.BoxesOf(n)
	if !laid {
		return "", false
	}
	of := boxes.Content
	if sizing, _ := r.doc.ComputedValue(n, "box-sizing"); sizing == "border-box" {
		of = boxes.Border
	}
	size := of.W
	if name == "height" {
		size = of.H
	}
	return strconv.FormatFloat(float64(size/r.doc.Zoom()), 'f', -1, 32) + "px", true
}

// computedStyle is every property of an element's computed style, a
// name and a value each, null for an element ui has not styled.
func (r *Runtime) computedStyle(n *ui.Node, _ arguments) (any, error) {
	r.fresh()
	r.reflow(n)
	all := r.doc.ComputedStyle(n)
	if len(all) == 0 {
		return nothing()
	}
	out := make([][2]string, len(all))
	own, inherited := r.fontSizes(n, ""), r.fontSizes(n, "font-size")
	for i, p := range all {
		fonts := own
		if p.Name == "font-size" {
			fonts = inherited
		}
		out[i] = [2]string{p.Name, resolved(p.Name, p.Value, fonts)}
	}
	return out, nil
}

// fontSizes are what em and rem stand for in n's property name: the
// font size of n, of its parent for font-size itself, and of the root.
func (r *Runtime) fontSizes(n *ui.Node, name string) fontSizes {
	px := func(n *ui.Node) float64 {
		if n == nil {
			return 0
		}
		size, _ := r.doc.ComputedValue(n, "font-size")
		v, _ := strconv.ParseFloat(strings.TrimSuffix(size, "px"), 64)
		return v
	}
	of := n
	if name == "font-size" {
		of = n.Parent()
	}
	return fontSizes{em: px(of), rem: px(r.doc.HTML())}
}

var hexColour = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

// resolved is a computed value as a browser's getComputedStyle writes
// it: its math functions worked out and a colour given in hexadecimal
// as rgb() or rgba(), but a custom property's, which stays as written.
func resolved(name, value string, fonts fontSizes) string {
	if strings.HasPrefix(name, "--") {
		return value
	}
	if value = worked(name, value, fonts); !hexColour.MatchString(value) {
		return value
	}
	digits := value[1:]
	if len(digits) <= 4 {
		var wide strings.Builder
		for _, c := range digits {
			wide.WriteRune(c)
			wide.WriteRune(c)
		}
		digits = wide.String()
	}
	channel := func(at int) int {
		n, _ := strconv.ParseUint(digits[at:at+2], 16, 8)
		return int(n)
	}
	rgb := strconv.Itoa(channel(0)) + ", " + strconv.Itoa(channel(2)) + ", " + strconv.Itoa(channel(4))
	if len(digits) == 6 || channel(6) == 255 {
		return "rgb(" + rgb + ")"
	}
	return "rgba(" + rgb + ", " + strconv.FormatFloat(float64(channel(6))/255, 'g', 3, 64) + ")"
}

// supports answers CSS.supports(property, value): whether ui keeps the
// declaration of a sheet written for browsers.
func supports(a arguments) (any, error) {
	decls, warns := css.ParseInline(a.text(0)+": "+a.text(1), css.Browser())
	return len(decls) == 1 && len(warns) == 0, nil
}

// supportsWhen answers CSS.supports(condition), as an @supports rule
// of a sheet written for browsers would.
func supportsWhen(a arguments) (any, error) {
	condition := strings.TrimSpace(a.text(0))
	if !strings.HasPrefix(condition, "(") && !strings.Contains(condition, "(") {
		return false, nil
	}
	sheet, _ := css.Parse("", "@supports "+condition+" { a { color: red } }", css.Browser())
	return len(sheet.Rules) == 1, nil
}
