package app

import (
	"context"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/pkg/webdriver"
)

// errPrompted is what reading a document whose scripts wait for the
// answer to a question fails with.
var errPrompted = webdriver.Errorf(webdriver.UnexpectedAlertOpen, "the page waits for the answer to what it asked")

// prompted is the question the session's window waits the answer of.
func (d *Driver) prompted() (*modal, *question, error) {
	if _, err := d.window(); err != nil {
		return nil, nil, err
	}
	a, q := d.p.questionOf(d.tab)
	if q == nil {
		return nil, nil, webdriver.Errorf(webdriver.NoSuchAlert, "the page asks nothing: no alert, confirm or prompt is open")
	}
	return a, q, nil
}

// Prompt is the question the session's window waits the answer of.
func (d *Driver) Prompt(ctx context.Context) (webdriver.Prompt, error) {
	var open webdriver.Prompt
	err := d.once(ctx, func() error {
		_, q, err := d.prompted()
		if err == nil {
			open = webdriver.Prompt{Kind: q.Kind, Text: q.Message}
		}
		return err
	})
	return open, err
}

func (d *Driver) AlertText(ctx context.Context) (string, error) {
	open, err := d.Prompt(ctx)
	return open.Text, err
}

// answering answers the question the session's window waits the
// answer of, and gives its scripts a frame to go on.
func (d *Driver) answering(ctx context.Context, ok bool) error {
	return d.flowing(ctx, func(f *flow) {
		f.do(func() error {
			a, q, err := d.prompted()
			if err != nil {
				return err
			}
			a.s.answer(a, dom.Answer{OK: ok, Text: a.s.typed(q)})
			return nil
		}).frames(1)
	})
}

func (d *Driver) DismissAlert(ctx context.Context) error { return d.answering(ctx, false) }
func (d *Driver) AcceptAlert(ctx context.Context) error  { return d.answering(ctx, true) }

// SendAlertText puts text in the field of a prompt.
func (d *Driver) SendAlertText(ctx context.Context, text string) error {
	return d.once(ctx, func() error {
		a, q, err := d.prompted()
		switch {
		case err != nil:
			return err
		case q.Kind != dom.Prompt:
			return webdriver.Errorf(webdriver.ElementNotInteractable, "an "+q.Kind+" has no field to type in")
		}
		q.text = text
		if q.field != nil {
			a.s.doc.SetValue(q.field, text)
		}
		return nil
	})
}
