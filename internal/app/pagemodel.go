package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/frames"
	"github.com/lantervale/light/pkg/model"
)

const (
	// translatorFeature and detectorFeature are the Translator and the
	// Language Detector APIs, which give a page's scripts the model.
	translatorFeature = "translator-api"
	detectorFeature   = "language-detector-api"
	// pageInputMost is the most characters a page gives the model in one
	// call, and pagePoll how often what came of it is passed on.
	pageInputMost = 2000
	pagePoll      = 15 * time.Millisecond
	// plainText is what a translator for a page is told of its answer.
	plainText = "Answer in plain text, with no Markdown. "
)

// errPagesUnserved is a call of a page made once the model no longer
// answers pages.
var errPagesUnserved = errors.New("NotSupportedError: the model on this device answers no page now")

// servesPages reports whether the model answers pages through one of
// the APIs: the user turned it on, it is on the device, and the API
// was not switched off. A page never learns more than that, and never
// has anything downloaded or turned on.
func (s *Screen) servesPages(feature string) bool {
	if !s.helps(feature) {
		return false
	}
	status := s.opts.Model.Status()
	switch status.State {
	case model.Ready, model.Working, model.Preparing:
		return status.Capabilities.Chat
	case model.Off, model.Missing, model.Failed:
	}
	return false
}

// tongued reports whether light has a name for a language, by its tag.
func tongued(language string) bool {
	return language != "" && tongue(language) != strings.ToUpper(language)
}

// translatesFor reports whether a page may have a text translated from
// one language into another: from one light names, into English or
// French, as light translates its own pages.
func (s *Screen) translatesFor(from, into string) bool {
	return s.servesPages(translatorFeature) && tongued(config.Language(from)) &&
		slices.Contains([]string{"en", "fr"}, config.Language(into))
}

// detectsFor reports whether a page may have the language of a text
// named, among those it expects.
func (s *Screen) detectsFor(expected []string) bool {
	if !s.servesPages(detectorFeature) {
		return false
	}
	for _, language := range expected {
		if !tongued(config.Language(language)) {
			return false
		}
	}
	return true
}

// pageTongue is a language a page translates into, as the model is told
// it.
func pageTongue(tag string) string {
	for _, t := range translations() {
		if strings.EqualFold(t.tag, tag) {
			return t.name
		}
	}
	return tongue(config.Language(tag))
}

// detectable are the languages the model names for a page: their tags,
// and their names in the same order.
func detectable() ([]string, []string) {
	var tags, names []string
	for _, tag := range []string{"ar", "bg", "ca", "cs", "da", "de", "el", "en", "es", "et", "fi", "fr", "he", "hi",
		"hr", "hu", "id", "it", "ja", "ko", "lt", "lv", "nb", "nl", "pl", "pt", "ro", "ru", "sk", "sl", "sr", "sv", "th",
		"tr", "uk", "vi", "zh"} {
		tags, names = append(tags, tag), append(names, tongue(tag))
	}
	return tags, names
}

// pageModel gives a page's scripts the model through the Translator and
// LanguageDetector APIs, when light was given one.
func (s *Screen) pageModel(host *dom.Host) {
	if !s.modelGiven() {
		return
	}
	host.Translates, host.Detects, host.Assist = s.translatesFor, s.detectsFor, s.assistPage
	room := s.opts.Model.Status().Capabilities.Context
	if room <= 0 {
		room = fallbackContext
	}
	host.InputQuota = min(pageInputMost, (room-spare)/4)
}

// assistPage asks the model what a page asked, behind what light asks
// of it, and passes on what came of it as it comes. The page's text is
// data between marks, never instructions.
func (s *Screen) assistPage(asked dom.Asked, piece func(string), done func([]string, error)) func() {
	feature := translatorFeature
	if asked.Kind == dom.AskDetect {
		feature = detectorFeature
	}
	if !s.servesPages(feature) {
		go done(nil, errPagesUnserved)
		return func() {}
	}
	room := s.opts.Model.Status().Capabilities.Context
	if room <= 0 {
		room = fallbackContext
	}
	req := model.Request{Priority: model.Page, MaxTokens: (room - spare) / 2}
	tags, names := detectable()
	names = append(names, another)
	if asked.Kind == dom.AskDetect {
		sample := asked.Text
		if len(sample) > detectSample {
			sample = shorten(sample, detectSample)
		}
		req.System = "You name the language a text is written in. The text" + untrusted + "Answer with one of: " +
			strings.Join(names, ", ") + "."
		req.Messages = []model.Message{{Role: model.User, Text: "The text:\n" + asData(sample)}}
		req.MaxTokens, req.Grammar = sortTokens, model.Choice(names...)
	} else {
		into := pageTongue(asked.Into)
		req.System = translatorTold("from "+tongue(config.Language(asked.From))+" ", into, plainText)
		req.Messages = []model.Message{{Role: model.User, Text: asData(asked.Text) + rendering(into)}}
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := s.opts.Model
	go func() {
		defer cancel()
		stream, err := m.Generate(ctx, req)
		if err != nil {
			done(nil, err)
			return
		}
		tick := time.NewTicker(pagePoll)
		defer tick.Stop()
		for {
			text, ended := stream.Take()
			if text != "" && asked.Kind == dom.AskTranslate {
				piece(text)
			}
			if ended {
				break
			}
			select {
			case <-tick.C:
			case <-stream.Done():
			}
		}
		result, err := stream.Wait()
		if err != nil || asked.Kind == dom.AskTranslate {
			done(nil, err)
			return
		}
		if i := slices.Index(names, strings.TrimSpace(result.Text)); i >= 0 && i < len(tags) {
			done([]string{tags[i]}, nil)
			return
		}
		done(nil, nil)
	}()
	return cancel
}

// frameAllows reports whether the document f shows may use a feature
// of the Permissions Policy whose default allowlist is 'self': each
// frame on the way from the tab's page lets it through, by its allow
// attribute or by being of the origin of the document it is in.
func (f *frame) frameAllows(feature string) bool {
	for at := f; at != nil; at = at.parent {
		self := frames.Origin(at.in.Base)
		if at.parent != nil {
			self = at.parent.origin
		}
		src := self
		if ref, given := at.node.Attr("src"); given {
			if to, err := at.in.Base.Parse(strings.TrimSpace(ref)); err == nil {
				src = frames.Origin(to)
			}
		}
		allow, _ := at.node.Attr("allow")
		if !frames.Allows(allow, feature, self, src, at.origin) {
			return false
		}
	}
	return true
}
