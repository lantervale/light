package app

import (
	"strings"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/config"
)

// handMade is the release a build made by hand counts as.
const handMade = "dev"

// release is the release a version is of: what stands before its first
// dash, so the builds made after a release are of it, and handMade for
// a build without one.
func release(version string) string {
	version, _, _ = strings.Cut(strings.TrimPrefix(version, "v"), "-")
	if version == "" {
		return handMade
	}
	return version
}

// warn shows, once a run and in its first window, that light is an
// early preview, until its user agrees: at the first start, and at the
// first of each release after it.
func (p *Profile) warn() {
	of := release(p.opts.Version)
	if !p.opts.Preview || p.warning || p.config.PreviewSeen == of || len(p.screens) == 0 {
		return
	}
	p.warning = true
	p.screens[0].preview(of)
}

// preview is the warning's dialog over the window: nothing under it
// takes the pointer or the keys until it is agreed to, which is noted.
func (s *Screen) preview(of string) {
	if !s.modals.styled[s.doc] {
		if s.modals.styled == nil {
			s.modals.styled = map[*ui.Document]bool{}
		}
		s.modals.styled[s.doc] = true
		s.doc.AddSheet(askingSheet)
	}
	what := "light " + of
	if of == handMade {
		what = "this build of light"
	}
	var node *ui.Node
	node = ui.Dialog(ui.ID("preview"), ui.Attr("role", "alertdialog"), ui.Attr("aria-label", "light is an early preview"),
		ui.H2(ui.Text("light is a very early preview")),
		ui.P(ui.Text("You are running "+what+". It is not even an alpha: its security is not hardened yet, and a "+
			"page may read or change more than it should.")),
		ui.P(ui.Strong(ui.Text("Do not open sensitive pages with it, and do not type a password, a card number or "+
			"any other secret anywhere in it."))),
		ui.P(ui.Text("No bank, no mail, no work account, no health or administration site. Use it to look at how "+
			"it draws the web, nothing more.")),
		ui.Div(ui.ID("preview-buttons"), ui.Button(ui.ID("preview-ok"), ui.Text("I understand"), ui.Value(answerOK),
			ui.Attr("formmethod", "dialog"), ui.Attr("autofocus", ""))),
		ui.OnClose(func(e ui.Event) {
			if e.Node != node {
				return
			}
			s.doc.Body().Remove(node)
			s.configure(func(c *config.Config) { c.PreviewSeen = of })
		}))
	toggle(node, "dark", s.dark())
	s.doc.Body().Append(node)
	s.doc.ShowModal(node)
}
