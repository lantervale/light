package dom

import (
	"errors"
	"unicode/utf8"
)

// The features of the Permissions Policy that gate the Translator and
// LanguageDetector APIs, and the kinds of what they ask.
const (
	FeatureTranslator = "translator"
	FeatureDetector   = "language-detector"
	AskTranslate      = "translate"
	AskDetect         = "detect"
)

// Asked is what a page's script asks of the language model on this
// device: a text to translate from one language into another, or to
// name the language of. Languages are BCP 47 tags.
type Asked struct {
	Kind       string
	From, Into string
	Text       string
}

// errNoModel is what a page is answered when light gives it no model.
var errNoModel = errors.New("NotSupportedError: no model on this device answers this page")

// assisting is what the page asked of the model: one ask at a time, the
// rest waiting their turn, so that a page holds one place at most in
// the model's queue.
type assisting struct {
	waiting []assist
	// running is the ask the model answers, 0 for none, and stop what
	// stops it.
	running int
	stop    func()
}

type assist struct {
	id    int
	asked Asked
}

// allows reports whether the page's document may use a feature of the
// Permissions Policy.
func (r *Runtime) allows(feature string) bool {
	return r.host.Allows == nil || r.host.Allows(feature)
}

// assistant answers what the Translator and LanguageDetector APIs ask of
// light: whether the model is there for them, the most a call holds,
// and the asks themselves.
func (r *Runtime) assistant(a arguments) (any, error) {
	switch a.text(0) {
	case "available":
		return r.available(a.text(1), a.text(2), a.text(3), a[min(4, len(a)):]), nil
	case "allowed":
		return r.allows(a.text(1)), nil
	case "quota":
		return float64(max(r.host.InputQuota, 0)), nil
	case "ask":
		asked := Asked{Kind: a.text(2), From: a.text(3), Into: a.text(4), Text: a.text(5)}
		if utf8.RuneCountInString(asked.Text) > r.host.InputQuota {
			//nolint:revive,staticcheck // a DOMException's message, as Chrome words it
			return nil, errors.New("QuotaExceededError: The input is too large.")
		}
		r.assisting.waiting = append(r.assisting.waiting, assist{id: int(a.number(1)), asked: asked})
		r.nextAssist()
	case "stop":
		r.stopAssist(int(a.number(1)))
	}
	return nothing()
}

// available reports whether the model does what a kind of ask needs
// for the page now: translate from one language into another, or name
// languages among those expected.
func (r *Runtime) available(kind, from, into string, expected arguments) bool {
	switch {
	case kind == AskTranslate:
		return r.allows(FeatureTranslator) && r.host.Translates != nil && r.host.Translates(from, into)
	case kind == AskDetect && r.allows(FeatureDetector) && r.host.Detects != nil:
		languages := make([]string, 0, len(expected))
		for i := range expected {
			languages = append(languages, expected.text(i))
		}
		return r.host.Detects(languages)
	}
	return false
}

// nextAssist hands the model the oldest ask that waits, once none runs.
func (r *Runtime) nextAssist() {
	s := &r.assisting
	if s.running != 0 || len(s.waiting) == 0 || r.closed {
		return
	}
	next := s.waiting[0]
	s.waiting = s.waiting[1:]
	if r.host.Assist == nil {
		r.waiting = append(r.waiting, func() { r.enter("assisted", next.id, []any{}, errNoModel.Error()) })
		return
	}
	s.running = next.id
	live := true
	stop := r.host.Assist(next.asked, func(piece string) {
		r.land(func() {
			if live {
				r.enter("assisted", next.id, piece)
			}
		})
	}, func(languages []string, err error) {
		r.land(func() {
			if !live {
				return
			}
			live = false
			named, why := []any{}, ""
			for _, language := range languages {
				named = append(named, language)
			}
			if err != nil {
				why = err.Error()
			}
			r.enter("assisted", next.id, named, why)
			r.endAssist(next.id)
		})
	})
	s.stop = func() {
		live = false
		if stop != nil {
			stop()
		}
	}
}

// stopAssist stops an ask, the one the model answers or one that waits.
func (r *Runtime) stopAssist(id int) {
	s := &r.assisting
	for i, w := range s.waiting {
		if w.id == id {
			s.waiting = append(s.waiting[:i], s.waiting[i+1:]...)
			return
		}
	}
	if s.running == id && s.stop != nil {
		s.stop()
		r.endAssist(id)
	}
}

// endAssist notes the ask the model answered ended, and starts the next.
func (r *Runtime) endAssist(id int) {
	if s := &r.assisting; s.running == id {
		s.running, s.stop = 0, nil
		r.nextAssist()
	}
}

// unassisted stops what the page asked of the model, for good.
func (r *Runtime) unassisted() {
	s := &r.assisting
	if s.stop != nil {
		s.stop()
	}
	*s = assisting{}
}
