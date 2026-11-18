package inspector_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/ember"
	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/platform"
	"github.com/lantervale/ember/twod"
	"github.com/lantervale/ember/ui"
	"github.com/lantervale/ember/ui/align"
	"github.com/lantervale/ember/world"

	"github.com/lantervale/light/internal/devtools/inspector"
)

const screenSheet = `html, body { height: 100%; }
body { display: flex; flex-direction: column; margin: 0; }
#stage { position: relative; flex: 1; min-height: 0; }
#web { display: block; width: 100%; height: 100%; }
#own { height: 100%; overflow: auto; }
#tools { display: flex; flex: none; height: 320px; }`

//nolint:lll // a rule is one line of its sheet
const pageSheet = `body { margin: 0; color: #222222; }
.card { display: flex; box-sizing: border-box; margin: 10px 20px; padding: 5px; border: 2px solid #336699; width: 300px; height: 60px; }
.card p { color: red; margin: 0; }
#deep { color: green; }
p { color: blue; }
#tall { height: 900px; }`

// shown is a screen with a page and the Inspector under it.
type shown struct {
	t        *testing.T
	doc      *ui.Document
	page     inspector.Page
	in       *inspector.Inspector
	state    *inspector.State
	input    *input.State
	world    *world.World
	open     bool
	selected []*ui.Node
	sources  []string
	clicks   int
	web      *ui.Node

	card, deep, words, button, list, foot *ui.Node
}

type frames struct{ s *shown }

func (f frames) Update(world.Frame) {
	if f.s.web != nil && !f.s.in.Picking() {
		f.s.doc.Invalidate(f.s.web)
	}
	f.s.in.Update(f.s.open)
}

// content is a small page's elements.
func (s *shown) content() []*ui.Node {
	s.deep = ui.Span(ui.ID("deep"), ui.Text("deep"))
	s.words = ui.P(ui.Text("The lower vaults stay wet."))
	s.card = ui.Div(ui.ID("main"), ui.Class("card"), ui.Attr("data-kind", "crypt"), ui.P(s.deep, ui.Text(" water")))
	s.button = ui.Button(ui.ID("press"), ui.Text("Press"), ui.OnClick(func() { s.clicks++ }))
	s.list = ui.Ul(ui.ID("list"))
	s.foot = ui.P(ui.ID("foot"), ui.Text("The foot."))
	return []*ui.Node{s.card, s.words, s.button, s.list, ui.Div(ui.ID("tall")), s.foot}
}

func show(t *testing.T, own bool) *shown {
	t.Helper()
	s := &shown{t: t, open: true, state: &inspector.State{}}
	s.doc = ui.NewDocument(ui.CSS(screenSheet), ui.CSS(inspector.Sheet))
	stage := ui.Div(ui.ID("stage"))
	area := func() (geom.Rect, bool) { return s.doc.BoxOf(stage) }
	if own {
		s.doc = ui.NewDocument(ui.CSS(screenSheet), ui.CSS(pageSheet), ui.CSS(inspector.Sheet))
		root := ui.Article(ui.ID("own"))
		root.Append(s.content()...)
		stage.Append(root)
		s.page = inspector.Page{Doc: s.doc, Root: root, Area: area}
	} else {
		page := ui.NewDocument(ui.BrowserSheets(), ui.ScrollingViewport(),
			ui.CSS(pageSheet, ui.From("https://crypt.test/site.css")))
		page.Body().Append(s.content()...)
		page.Body().AddClass("home")
		head := ui.Div(ui.Div(ui.Attr("charset", "utf-8")))
		s.web = ui.Canvas(func(c *twod.Canvas) { page.Draw(c) }, ui.ID("web"))
		stage.Append(s.web)
		s.page = inspector.Page{Doc: page, Root: page.HTML(), Head: head, Doctype: true, Area: area,
			Tag: func(n *ui.Node) string {
				if n.Parent() == head {
					return "meta"
				}
				return n.Tag().String()
			}}
	}
	win := platform.NewWindow(platform.Size(1100, 700))
	s.input = win.Input()
	s.in = inspector.New(s.doc, inspector.Host{
		Page:     func() inspector.Page { return s.page },
		Input:    func() *input.State { return s.input },
		Selected: func(n *ui.Node) { s.selected = append(s.selected, n) },
		Source:   func(address string, line int) { s.sources = append(s.sources, fmt.Sprint(address, ":", line)) },
	})
	stage.Append(s.in.Overlay())
	s.doc.Body().Append(stage, ui.Div(ui.ID("tools"), s.in.Body()))
	win.Add(s.doc)
	s.world = ember.New().AddEntities(win).AddSystems(frames{s})
	s.in.Show(s.state)
	s.frames(3)
	t.Cleanup(func() {
		if t.Failed() {
			return
		}
		s.frames(2)
		if faults := align.Check(s.doc); len(faults) > 0 {
			t.Errorf("layout faults: %v", faults)
		}
		if warns := s.doc.Warnings(); len(warns) > 0 {
			t.Errorf("warnings: %v", warns)
		}
	})
	return s
}

func (s *shown) frames(n int) { s.world.RunFrames(n) }

func (s *shown) node(id string) *ui.Node {
	s.t.Helper()
	n := s.doc.ByID(id)
	if n == nil {
		s.t.Fatalf("no element %s", id)
	}
	return n
}

func (s *shown) box(n *ui.Node) geom.Rect {
	s.t.Helper()
	box, ok := s.doc.BoxOf(n)
	if !ok {
		s.t.Fatalf("%s#%s is not shown", n.Tag(), n.ID())
	}
	return box
}

func (s *shown) clickAt(at geom.Vec2) {
	s.input.SetMouse(at)
	s.frames(1)
	s.input.SetMouseButton(input.MouseLeft, true)
	s.frames(1)
	s.input.SetMouseButton(input.MouseLeft, false)
	s.frames(2)
}

func (s *shown) click(n *ui.Node) {
	s.t.Helper()
	s.clickAt(s.box(n).Center())
}

func (s *shown) press(k input.Key) {
	s.input.SetKey(k, true)
	s.frames(1)
	s.input.SetKey(k, false)
	s.frames(2)
}

func (s *shown) typeIn(field *ui.Node, text string) {
	s.t.Helper()
	s.click(field)
	for _, r := range text {
		s.input.AddText(r)
		s.frames(1)
	}
	s.frames(1)
}

// retype types text in a field in place of what it holds.
func (s *shown) retype(field *ui.Node, text string) {
	s.t.Helper()
	s.doc.Focus(s.node("dt-insp-pick"))
	s.frames(1)
	s.doc.SetValue(field, "")
	s.typeIn(field, text)
}

// row is the row of a page's node in the tree.
func (s *shown) row(n *ui.Node) *ui.Node {
	s.t.Helper()
	row := s.in.RowOf(n)
	if row == nil {
		s.t.Fatalf("the tree has no row for %s#%s", n.Tag(), n.ID())
	}
	return row
}

// twist is the triangle of a node's row.
func (s *shown) twist(n *ui.Node) *ui.Node { return s.row(n).Children()[0] }

// rows are the rows the tree shows, each as its text.
func (s *shown) rows() []string {
	var out []string
	s.node("dt-insp-tree").Walk(func(n *ui.Node) bool {
		if n.HasClass("dt-insp-row") {
			if _, ok := s.doc.BoxOf(n); ok {
				out = append(out, n.Text())
			}
			return false
		}
		return true
	})
	return out
}

// texts are the texts of the elements of that class under the element
// named id.
func (s *shown) texts(id, class string) []string {
	var out []string
	s.node(id).Walk(func(n *ui.Node) bool {
		if n.HasClass(class) {
			out = append(out, n.Text())
			return false
		}
		return true
	})
	return out
}

// onPage is a point of a page's element in the window.
func (s *shown) onPage(n *ui.Node) geom.Vec2 {
	s.t.Helper()
	boxes, ok := s.page.Doc.BoxesOf(n)
	if !ok {
		s.t.Fatalf("the page does not show %s#%s", n.Tag(), n.ID())
	}
	at := boxes.Border.Center()
	if s.web != nil {
		area := s.box(s.web)
		at = at.Add(geom.V2(area.X, area.Y))
	}
	return at
}

func same(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestTheTreeShowsThePageItsFirstTwoLevelsOpen(t *testing.T) {
	s := show(t, false)
	same(t, s.rows(), []string{
		"<!DOCTYPE html>",
		"<html>",
		"<head>…</head>",
		`<body class="home">`,
		`<div id="main" class="card" data-kind="crypt">…</div>flex`,
		"<p>The lower vaults stay wet.</p>",
		`<button id="press">Press</button>`,
		`<ul id="list"></ul>`,
		`<div id="tall"></div>`,
		`<p id="foot">The foot.</p>`,
		"</body>",
		"</html>",
	})
	if s.in.RowOf(s.deep) != nil {
		t.Fatal("the tree made a row for a node under a closed one")
	}
}

func TestATriangleOpensAndClosesAnElement(t *testing.T) {
	s := show(t, false)
	s.click(s.twist(s.card))
	got := s.rows()
	same(t, got[4:8], []string{
		`<div id="main" class="card" data-kind="crypt">flex`,
		"<p>…</p>",
		"</div>",
		"<p>The lower vaults stay wet.</p>",
	})
	s.click(s.twist(s.deep.Parent()))
	same(t, s.rows()[5:9], []string{"<p>", `<span id="deep">deep</span>`, "water", "</p>"})
	if len(s.selected) != 0 {
		t.Fatalf("a press on a triangle selected %v", s.selected)
	}

	s.click(s.twist(s.card))
	if s.in.RowOf(s.deep) != nil || len(s.rows()) != 12 {
		t.Fatalf("closed, the tree still shows %v", s.rows())
	}
	s.click(s.twist(s.card))
	if s.in.RowOf(s.deep) == nil {
		t.Fatal("opened again, the tree forgot what was open under it")
	}
	s.click(s.twist(s.page.Head))
	same(t, s.rows()[2:5], []string{"<head>", `<meta charset="utf-8">`, "</head>"})
}

func TestAPressOnARowSelectsItsNode(t *testing.T) {
	s := show(t, false)
	s.click(s.row(s.words))
	if s.in.Selected() != s.words || len(s.selected) != 1 || s.selected[0] != s.words {
		t.Fatalf("selected %v, told %v", s.in.Selected(), s.selected)
	}
	if !s.row(s.words).HasClass("selected") {
		t.Fatal("the row of the selected node is not marked")
	}
	s.clickAt(s.node("dt-insp-tree").Children()[0].Box().Center())
	if s.in.Selected() != s.words {
		t.Fatal("a press on the doctype, which is no node, moved the selection")
	}
	s.click(s.row(s.card))
	if s.row(s.words).HasClass("selected") || !s.row(s.card).HasClass("selected") || len(s.selected) != 2 {
		t.Fatal("the selection did not move to the row pressed")
	}
	if at, _ := s.page.Doc.BoxesOf(s.foot); at.Border.Y < s.box(s.web).H {
		t.Fatalf("the foot of the page shows before it is selected, at %v", at.Border)
	}
	s.click(s.row(s.foot))
	if at, _ := s.page.Doc.BoxesOf(s.foot); at.Border.Y+at.Border.H > s.box(s.web).H {
		t.Fatalf("selected, the foot of the page is left out of sight at %v", at.Border)
	}
	s.click(s.row(s.card))
	row, pane := s.row(s.card).Box(), s.box(s.node("dt-insp-tree"))
	if row.W < pane.W-20 {
		t.Fatalf("the selected row is %v wide in a pane %v wide", row.W, pane.W)
	}
}

func TestTheArrowsWalkTheTree(t *testing.T) {
	s := show(t, false)
	s.click(s.row(s.card))
	s.press(input.KeyDown)
	if s.in.Selected() != s.words {
		t.Fatalf("down selected %v", s.in.Selected())
	}
	s.press(input.KeyUp)
	s.press(input.KeyRight)
	if s.in.RowOf(s.deep.Parent()) == nil || s.in.Selected() != s.card {
		t.Fatal("right did not open the selected element")
	}
	s.press(input.KeyRight)
	if s.in.Selected() != s.deep.Parent() {
		t.Fatalf("right on an open element selected %v", s.in.Selected())
	}
	s.press(input.KeyLeft)
	if s.in.Selected() != s.card {
		t.Fatalf("left on a closed element selected %v", s.in.Selected())
	}
	s.press(input.KeyLeft)
	if s.in.RowOf(s.deep.Parent()) != nil {
		t.Fatal("left did not close the selected element")
	}
	s.press(input.KeyUp)
	if s.in.Selected() != s.page.Doc.Body() {
		t.Fatalf("up selected %v", s.in.Selected())
	}

	s.typeIn(s.node("dt-insp-search"), "zz")
	s.press(input.KeyDown)
	if s.in.Selected() != s.page.Doc.Body() {
		t.Fatal("the arrows walked the tree while a field had the keys")
	}
}

func TestTheRowUnderThePointerOutlinesItsElementOnThePage(t *testing.T) {
	for _, own := range []bool{false, true} {
		s := show(t, own)
		if _, ok := s.in.Highlighted(); ok {
			t.Fatal("something is outlined before a row is pointed at")
		}
		s.input.SetMouse(s.box(s.row(s.card)).Center())
		s.frames(3)
		want, _ := s.page.Doc.BoxesOf(s.card)
		origin := geom.Vec2{}
		if !own {
			area := s.box(s.web)
			origin = geom.V2(area.X, area.Y)
		}
		if want.Border.W != 300 || want.Border.H != 60 {
			t.Fatalf("the card's border box is %v", want.Border)
		}
		border := s.node("dt-insp-hl-border").Box()
		if border.X != want.Border.X+origin.X || border.Y != want.Border.Y+origin.Y || border.W != 300 || border.H != 60 {
			t.Fatalf("own %t: the border box is outlined at %v, the page has it at %v from %v", own, border,
				want.Border, origin)
		}
		margin, content := s.node("dt-insp-hl-margin").Box(), s.box(s.node("dt-insp-hl-content"))
		if margin.W != 340 || margin.H != 80 || content.W != 286 || content.H != 46 {
			t.Fatalf("own %t: the margin box is %v and the content box %v", own, margin, content)
		}
		if got := s.node("dt-insp-label").Text(); got != "div#main.card  300 × 60" {
			t.Fatalf("the label says %q", got)
		}
		if got, ok := s.in.Highlighted(); !ok || got != border {
			t.Fatalf("Highlighted is %v, %t", got, ok)
		}
		if s.doc.ElementAt(border.Center()) == s.node("dt-insp-hl-content") {
			t.Fatal("the highlight takes the pointer")
		}

		s.input.SetMouse(geom.V2(5, 5))
		s.frames(3)
		if _, ok := s.doc.BoxOf(s.node("dt-insp-hl-border")); ok {
			t.Fatal("the outline stays once the pointer left the row")
		}
	}
}

func TestTheSelectedElementStaysOutlinedWhileTheInspectorShows(t *testing.T) {
	s := show(t, false)
	s.click(s.row(s.words))
	want, _ := s.page.Doc.BoxesOf(s.words)
	area := s.box(s.web)
	outline := s.node("dt-insp-outline").Box()
	if outline.X != want.Border.X+area.X || outline.Y != want.Border.Y+area.Y || outline.W != want.Border.W {
		t.Fatalf("the outline is at %v, the paragraph at %v", outline, want.Border)
	}
	s.open = false
	s.frames(2)
	if _, ok := s.doc.BoxOf(s.in.Overlay()); ok {
		t.Fatal("the overlay stays over the page once the tools show something else")
	}
	s.open = true
	s.frames(2)
	if _, ok := s.doc.BoxOf(s.node("dt-insp-outline")); !ok {
		t.Fatal("the outline did not come back with the Inspector")
	}
}

func TestPickingSelectsTheElementOfThePageUnderAPress(t *testing.T) {
	for _, own := range []bool{false, true} {
		s := show(t, own)
		s.click(s.node("dt-insp-pick"))
		if !s.in.Picking() {
			t.Fatal("the button did not start picking")
		}
		s.input.SetMouse(s.onPage(s.button))
		s.frames(3)
		if got, ok := s.in.Highlighted(); !ok || got.Center() != s.onPage(s.button) {
			t.Fatalf("own %t: picking outlines %v, %t; the button is at %v", own, got, ok, s.onPage(s.button))
		}
		s.clickAt(s.onPage(s.button))
		if s.in.Selected() != s.button || s.in.Picking() {
			t.Fatalf("own %t: the press selected %v, picking %t", own, s.in.Selected(), s.in.Picking())
		}
		if s.clicks != 0 {
			t.Fatalf("own %t: the page had the press that picked", own)
		}
		s.clickAt(s.onPage(s.button))
		if s.clicks != 1 {
			t.Fatalf("own %t: once picked, the page has %d presses of the next one", own, s.clicks)
		}

		s.click(s.node("dt-insp-pick"))
		s.clickAt(s.onPage(s.deep))
		if s.in.Selected() != s.deep || !s.row(s.deep).HasClass("selected") {
			t.Fatalf("own %t: picking a node under closed ones selected %v", own, s.in.Selected())
		}
		tree, row := s.box(s.node("dt-insp-tree")), s.box(s.row(s.deep))
		if row.Y < tree.Y || row.Y+row.H > tree.Y+tree.H {
			t.Fatalf("the picked row at %v is out of the tree at %v", row, tree)
		}

		s.click(s.node("dt-insp-pick"))
		s.press(input.KeyEscape)
		if s.in.Picking() {
			t.Fatal("Escape did not end picking")
		}
		s.clickAt(s.onPage(s.words))
		if s.in.Selected() != s.deep {
			t.Fatal("a press on the page selected once picking had ended")
		}
	}
}

func TestTheTreeFollowsThePage(t *testing.T) {
	s := show(t, false)
	s.click(s.row(s.words))
	added := ui.Li(ui.Text("added"))
	s.list.Append(added)
	s.card.SetAttr("data-kind", "vault")
	s.frames(40)
	rows := s.rows()
	if rows[4] != `<div id="main" class="card" data-kind="vault">…</div>flex` {
		t.Fatalf("the row of a changed element reads %q", rows[4])
	}
	if rows[7] != `<ul id="list">…</ul>` {
		t.Fatalf("the row of an element given a child reads %q", rows[7])
	}
	s.click(s.twist(s.list))
	same(t, s.rows()[7:10], []string{`<ul id="list">`, "<li>added</li>", "</ul>"})
	second := ui.Li(ui.Text("second"))
	s.list.Append(second)
	s.frames(40)
	same(t, s.rows()[7:11], []string{`<ul id="list">`, "<li>added</li>", "<li>second</li>", "</ul>"})
	if s.in.Selected() != s.words || !s.row(s.words).HasClass("selected") {
		t.Fatal("the selection did not survive a change elsewhere")
	}

	s.click(s.row(second))
	s.list.Remove(second)
	s.frames(40)
	if s.in.Selected() != s.list || !s.row(s.list).HasClass("selected") {
		t.Fatalf("its node gone, the selection is %v", s.in.Selected())
	}

	next := ui.NewDocument(ui.BrowserSheets())
	next.Body().Append(ui.H1(ui.Text("Vaults")))
	canvas := s.web
	s.page = inspector.Page{Doc: next, Root: next.HTML(), Doctype: true, Area: s.page.Area}
	s.frames(1)
	_ = canvas
	same(t, s.rows(), []string{"<!DOCTYPE html>", "<html>", "<body>", "<h1>Vaults</h1>", "</body>", "</html>"})
	if s.in.Selected() != nil {
		t.Fatal("the selection outlived its page")
	}
	s.page = inspector.Page{}
	s.frames(1)
	if got := s.node("dt-insp-tree").Text(); got != "This tab shows no page to inspect." {
		t.Fatalf("with no page the tree reads %q", got)
	}
}

func TestTheRulesOfTheSelectedElementAreListedTheStrongestFirst(t *testing.T) {
	s := show(t, false)
	s.in.Select(s.deep)
	s.frames(2)
	selectors := s.texts("dt-insp-rules", "dt-insp-selector")
	if len(selectors) < 2 || selectors[0] != "#deep {" {
		t.Fatalf("the rules are %v", selectors)
	}
	s.in.Select(s.deep.Parent())
	s.deep.Parent().SetStyle("color: purple; margin-top: 1px")
	s.frames(40)
	selectors = s.texts("dt-insp-rules", "dt-insp-selector")
	same(t, selectors[:3], []string{"element {", ".card p {", "p {"})
	from := s.texts("dt-insp-rules", "dt-insp-from")
	same(t, from[:3], []string{"inline", "site.css:3", "site.css:5"})

	var lost []string
	s.node("dt-insp-rules").Walk(func(n *ui.Node) bool {
		if n.HasClass("overridden") {
			lost = append(lost, n.Text())
		}
		return true
	})
	same(t, lost[:2], []string{"color: red;", "color: blue;"})
	if swatches := s.texts("dt-insp-rules", "dt-insp-swatch"); len(swatches) < 3 {
		t.Fatalf("%d colours have a swatch", len(swatches))
	}

	s.typeIn(s.node("dt-insp-rules-filter"), "margin")
	selectors = s.texts("dt-insp-rules", "dt-insp-selector")
	if len(selectors) < 2 || selectors[0] != "element {" || selectors[1] != ".card p {" {
		t.Fatalf("filtered, the rules are %v", selectors)
	}
	for _, decl := range s.texts("dt-insp-rules", "dt-insp-decl") {
		if !strings.Contains(decl, "margin") {
			t.Fatalf("the filter keeps %q", decl)
		}
	}

	var link *ui.Node
	s.node("dt-insp-rules").Walk(func(n *ui.Node) bool {
		if link == nil && n.HasClass("dt-insp-link") {
			link = n
		}
		return true
	})
	s.click(link)
	same(t, s.sources, []string{"https://crypt.test/site.css:3"})
}

func TestTheComputedPaneShowsTheBoxAndTheDeclaredProperties(t *testing.T) {
	s := show(t, false)
	s.in.Select(s.card)
	s.frames(2)
	same(t, s.texts("dt-insp-box", "dt-insp-edges"), []string{"margin1020border22padding55286 × 4655222010"})
	if got := s.node("dt-insp-box-at").Text(); got != "300 × 60 at 20, 10 · static · border-box" {
		t.Fatalf("the box is said to be %q", got)
	}
	names := s.texts("dt-insp-computed", "dt-insp-name")
	values := map[string]string{}
	for i, value := range s.texts("dt-insp-computed", "dt-insp-computed") {
		values[names[i]] = value
	}
	for name, want := range map[string]string{"display": "flex", "width": "300px", "margin-left": "20px",
		"border-top-color": "#336699", "color": "#222222"} {
		if values[name] != want {
			t.Errorf("%s is computed %q, want %q", name, values[name], want)
		}
	}
	if _, listed := values["z-index"]; listed {
		t.Error("a property no declaration gave is listed")
	}
	declared := len(names)

	s.click(s.node("dt-insp-all"))
	names = s.texts("dt-insp-computed", "dt-insp-name")
	if len(names) <= declared*2 {
		t.Fatalf("with the browser's styles the pane lists %d properties, %d without", len(names), declared)
	}
	s.typeIn(s.node("dt-insp-computed-filter"), "z-ind")
	same(t, s.texts("dt-insp-computed", "dt-insp-prop"), []string{"z-indexauto"})

	s.in.Select(s.deep.Parent().Children()[1])
	s.frames(2)
	if got := s.node("dt-insp-rules").Text(); got != "A text has no style of its own." {
		t.Fatalf("for a text the rules read %q", got)
	}
}

func TestSearchFindsNodesAndEnterGoesToTheNext(t *testing.T) {
	s := show(t, false)
	s.typeIn(s.node("dt-insp-search"), "<p")
	if got := s.node("dt-insp-count").Text(); got != "3 matches" {
		t.Fatalf("the search counts %q", got)
	}
	s.retype(s.node("dt-insp-search"), "DATA-kind")
	if got := s.node("dt-insp-count").Text(); got != "1 match" {
		t.Fatalf("the search counts %q", got)
	}
	s.retype(s.node("dt-insp-search"), "#deep")
	s.press(input.KeyEnter)
	if s.in.Selected() != s.deep || s.node("dt-insp-count").Text() != "1 of 1" {
		t.Fatalf("Enter selected %v, %q", s.in.Selected(), s.node("dt-insp-count").Text())
	}
	s.retype(s.node("dt-insp-search"), "vaults")
	s.press(input.KeyEnter)
	if s.in.Selected() != s.words {
		t.Fatalf("a search for a text shown on its element's row selected %v", s.in.Selected())
	}
	s.retype(s.node("dt-insp-search"), " water")
	s.press(input.KeyEnter)
	if water := s.deep.Parent().Children()[1]; s.in.Selected() != water || !s.row(water).HasClass("selected") {
		t.Fatalf("a search for a text selected %v", s.in.Selected())
	}
	s.retype(s.node("dt-insp-search"), ".card")
	s.press(input.KeyEnter)
	s.press(input.KeyEnter)
	if s.in.Selected() != s.card {
		t.Fatalf("the search for a class selected %v", s.in.Selected())
	}
	s.retype(s.node("dt-insp-search"), "nothing like it")
	if got := s.node("dt-insp-count").Text(); got != "No match" {
		t.Fatalf("with no match the search says %q", got)
	}
}

func TestALargePageStaysQuick(t *testing.T) {
	s := show(t, false)
	var last *ui.Node
	for i := range 3000 {
		last = ui.Li(ui.Class("entry"), ui.A(ui.Attr("href", fmt.Sprint("/entry/", i)), ui.Text(fmt.Sprint("Entry ", i))),
			ui.Span(ui.Text(" and its note")))
		s.list.Append(last)
	}
	s.frames(40)
	began := time.Now()
	s.click(s.twist(s.list))
	opened := time.Since(began)
	if rows := s.rows(); len(rows) > 300 || rows[len(rows)-6] != "… 2800 more nodes" {
		t.Fatalf("an element of 3000 children shows %d rows at once, the last %q", len(rows), rows[len(rows)-6])
	}
	began = time.Now()
	s.in.Select(last)
	s.frames(2)
	revealed := time.Since(began)
	if rows := s.rows(); !s.row(last).HasClass("selected") || len(rows) > 300 {
		t.Fatalf("the last of 3000 children selected, the tree shows %d rows", len(rows))
	}
	earlier := s.node("dt-insp-tree")
	earlier.Walk(func(n *ui.Node) bool {
		if n.HasClass("dt-insp-earlier") {
			earlier = n
		}
		return true
	})
	if earlier.Text() != "… 2899 more nodes" {
		t.Fatalf("the children before those shown read %q", earlier.Text())
	}
	s.doc.ScrollIntoView(earlier)
	s.frames(2)
	s.click(earlier)
	if rows := s.rows(); len(rows) < 300 || len(rows) > 520 {
		t.Fatalf("asked for the children before, the tree shows %d rows", len(rows))
	}
	began = time.Now()
	s.frames(60)
	idle := time.Since(began) / 60
	t.Logf("opened in %v, the last child revealed in %v, %v a frame after", opened, revealed, idle)
	if opened > 2*time.Second || revealed > 5*time.Second || idle > 50*time.Millisecond {
		t.Fatalf("too slow: opened in %v, revealed in %v, %v a frame", opened, revealed, idle)
	}
}
