// Package menu is a context menu drawn with ui elements: a list of
// entries opened at a point of a Document, kept inside it, which ui's
// menu role walks by the arrows, chooses by Enter or a click and closes
// by Escape or a press elsewhere.
package menu

import (
	"fmt"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"
)

// The menu's sizes in points, which place it before it is laid out.
const (
	width     = 272
	padding   = 5
	row       = 28
	separator = 9
)

// Sheet is the menu's look, for the Document it is added to.
//
//nolint:gochecknoglobals // a constant text, built from the sizes above
var Sheet = fmt.Sprintf(`.menu { position: absolute; z-index: 10; box-sizing: border-box; width: %dpx; padding: %dpx;
	border: 1px solid rgba(255, 255, 255, 0.14); border-radius: 8px; background: #2a2533; color: #ece8f2;
	font-size: 13px; box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45); }
.menu [role="menuitem"] { box-sizing: border-box; height: %dpx; padding: 0 12px; border-radius: 5px;
	line-height: %dpx; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; cursor: default;
	outline: none; }
.menu [role="menuitem"]:focus { background: #f15a2b; color: #ffffff; }
.menu [role="menuitem"][aria-disabled="true"] { color: rgba(236, 232, 242, 0.35); background: transparent; }
.menu [role="separator"] { box-sizing: border-box; height: 1px; margin: %dpx 6px;
	background: rgba(255, 255, 255, 0.12); }`, width, padding, row, row, (separator-1)/2)

// Entry is one line of a menu: what it reads, what choosing it does, and
// whether it can be chosen.
type Entry struct {
	Label    string
	Do       func()
	Disabled bool
}

// Separator is a line between two groups of entries.
func Separator() Entry { return Entry{} }

func (e Entry) separates() bool { return e.Label == "" }

// Menu is the one menu of a Document, closed until Open.
type Menu struct {
	doc  *ui.Document
	root *ui.Node
}

// New is a menu for doc, which shows it once Node is among its body's
// children, after what it opens over.
func New(doc *ui.Document) *Menu {
	return &Menu{doc: doc, root: ui.Div(ui.Class("menu"), ui.Attr("role", "menu"), ui.Attr("aria-label", "Context"))}
}

func (m *Menu) Node() *ui.Node { return m.root }

func (m *Menu) IsOpen() bool {
	_, open := m.root.Attr("open")
	return open
}

// Open shows entries with the menu's corner at at, moved as far as it
// takes to stay within, and gives the first that can be chosen the focus,
// so the arrows walk them.
func (m *Menu) Open(at geom.Vec2, within geom.Rect, entries []Entry) {
	for len(m.root.Children()) > 0 {
		m.root.Remove(m.root.Children()[0])
	}
	height := float32(2*padding + 2)
	var first *ui.Node
	for _, e := range entries {
		if e.separates() {
			m.root.Append(ui.Div(ui.Attr("role", "separator")))
			height += separator
			continue
		}
		item := ui.Div(ui.Attr("role", "menuitem"), ui.Text(e.Label), ui.OnClick(m.choose(e)))
		if e.Disabled {
			item.SetAttr("aria-disabled", "true")
		} else if first == nil {
			first = item
		}
		m.root.Append(item)
		height += row
	}
	x := max(within.X, min(at.X, within.X+within.W-width))
	y := max(within.Y, min(at.Y, within.Y+within.H-height))
	m.root.SetStyle(fmt.Sprintf("left: %gpx; top: %gpx", x, y))
	m.root.SetAttr("open", "")
	if first != nil {
		m.doc.Focus(first)
	}
}

// choose closes the menu, which ui leaves open for one no element opens,
// then does what the entry does.
func (m *Menu) choose(e Entry) func() {
	return func() {
		m.Close()
		e.Do()
	}
}

func (m *Menu) Close() { m.root.RemoveAttr("open") }
