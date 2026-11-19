package web

import (
	"golang.org/x/net/html"

	"github.com/lantervale/ember/ui"
)

// Styled is one stylesheet of a page as it was loaded: the sheet the
// Document has of a style element or of a link, and the text it was
// read from.
type Styled struct {
	Sheet *ui.Sheet
	Text  string
}

// Sheets are the stylesheets the page was loaded with, by the style
// element or the link each is of.
func (t *Tree) Sheets() map[*html.Node]Styled { return t.b.styled }

// style gives doc the page's sheets one by one, in their order, so
// each can be changed apart once scripts run.
func (b *builder) style(doc *ui.Document) {
	b.styled = make(map[*html.Node]Styled, len(b.sheets))
	for _, sheet := range b.sheets {
		opts := []ui.SheetOption{ui.From(sheet.from.String())}
		if media := attr(sheet.node, "media"); media != "" {
			opts = append(opts, ui.Media(media))
		}
		b.styled[sheet.node] = Styled{Sheet: doc.AddSheet(sheet.text, opts...), Text: sheet.text}
	}
}
