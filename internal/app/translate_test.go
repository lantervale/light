package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/model"
	"github.com/lantervale/light/pkg/tabs"
)

// translator is a light whose model is on and ready for translation
// alone, whose requests stay on this machine; the model answers a word
// a Step, and names French as the language of what it is asked about.
func translator(t *testing.T) (*shown, *model.Fake) {
	t.Helper()
	s, fake := modelled(t, Options{Transport: &fenced{}})
	for _, f := range s.features {
		s.assists.Set(f.id, f.id == translateFeature)
	}
	s.configure(func(c *config.Config) { c.Model = true })
	s.frame()
	s.frame()
	fake.Manual = true
	fake.Reply = func(r model.Request) (string, error) {
		if r.Grammar != "" {
			return "French", nil
		}
		return "Hello **world**", nil
	}
	return s, fake
}

// desked opens the translation page.
func (s *shown) desked() {
	s.t.Helper()
	s.Open(translatePage)
	s.frame()
	s.frame()
}

// writes types a text in the translation page's field.
func (s *shown) writes(text string) {
	s.t.Helper()
	s.click("translate-text")
	s.types(text)
}

// pauses runs the frames of the pause after which a text is translated.
func (s *shown) pauses() {
	for range int(translatePause*60) + 2 {
		s.frame()
	}
}

func TestTheTranslationPageTranslatesWhatIsTypedOnceTypingStops(t *testing.T) {
	s, fake := translator(t)
	s.desked()
	if got := s.title(translatePage) + " | " + s.node("translate-engine").Text(); got != "Translate | On this "+
		"device · "+fake.Status().Chosen.Name+" · nothing is sent" {
		t.Fatalf("the page says %q", got)
	}
	if got := strings.Join(s.texts("translate-from", "folder"), ", "); got != "Detect language, English, Français" ||
		s.pressed("translate-from") != "Detect language" {
		t.Fatalf("the page offers to translate from %q, chosen %q", got, s.pressed("translate-from"))
	}
	if got := strings.Join(s.texts("translate-into", "folder"), ", "); got != "English (UK), English (US), Français" ||
		s.pressed("translate-into") != "English (US)" {
		t.Fatalf("the page offers to translate into %q, chosen %q", got, s.pressed("translate-into"))
	}
	if !s.disabled("translate-swap") || s.node("translate-count").Text() != "0 characters" {
		t.Fatalf("with no text the page counts %q", s.node("translate-count").Text())
	}
	s.clean()
	s.writes("Bonjour le monde")
	for range 10 {
		s.frame()
	}
	if len(fake.Asked()) != 0 || s.node("translate-count").Text() != "16 characters" {
		t.Fatalf("while typing the model is asked %d times, the page counts %q", len(fake.Asked()),
			s.node("translate-count").Text())
	}
	s.pauses()
	s.until("the model is asked", func() bool { return len(fake.Asked()) == 2 })
	asked, named := fake.Asked()[0], fake.Asked()[1]
	if !strings.Contains(asked.System, "into American English") || !strings.Contains(asked.System, "never") ||
		strings.Contains(asked.System, "Bonjour") || strings.Contains(asked.System, "from") ||
		asked.Messages[0].Text != asData("Bonjour le monde")+"\n\nThe translation into American "+
			"English:" || asked.Priority != model.Interactive ||
		asked.Grammar != "" {
		t.Fatalf("the model is told %q and asked %q", asked.System, asked.Messages[0].Text)
	}
	if named.Grammar != model.Choice("English", "French", "another") || named.Priority != model.Background ||
		named.MaxTokens > 16 || !strings.Contains(named.System, "never") ||
		named.Messages[0].Text != "The text:\n"+asData("Bonjour le monde") {
		t.Fatalf("for the language the model is told %q and asked %+v", named.System, named)
	}
	fake.Step()
	s.frame()
	if got := s.node("translate-out").Text() + " | " + s.node("translate-stands").Text(); got != "Hello | "+
		"Translating…" || s.doc.ByID("translate-stop") == nil {
		t.Fatalf("as it comes the translation shows as %q", got)
	}
	s.clean()
	s.replied(fake)
	s.until("the language is named", func() bool { return s.pressed("translate-from") == "Detected: French" })
	if got := shape(s.node("translate-out").Children()) + " | " + s.node("translate-stands").Text(); got !=
		"p[Hello world] strong=world | Translated" || s.doc.ByID("translate-stop") != nil ||
		s.disabled("translate-swap") {
		t.Fatalf("translated, the page shows %q", got)
	}
	s.click("translate-copy")
	if got := s.win.Clipboard(); got != "Hello **world**" {
		t.Fatalf("Copy put %q on the clipboard", got)
	}
}

func TestATranslationReadsInTheDirectionOfItsText(t *testing.T) {
	s, fake := translator(t)
	s.desked()
	s.writes("Bonjour")
	s.pauses()
	s.until("the model is asked", func() bool { return len(fake.Asked()) == 2 })
	s.replied(fake)
	if got, _ := s.doc.ComputedValue(s.node("translate-out").Children()[0], "direction"); got != "ltr" {
		t.Fatalf("an English translation reads %s", got)
	}
	fake.Reply = func(r model.Request) (string, error) {
		if r.Grammar != "" {
			return "French", nil
		}
		return "שלום עולם", nil
	}
	s.writes(" le monde")
	s.pauses()
	s.until("the model is asked again", func() bool { return len(fake.Asked()) > 2 })
	s.replied(fake)
	if got, _ := s.doc.ComputedValue(s.node("translate-out").Children()[0], "direction"); got != "rtl" ||
		s.node("translate-out").Text() != "שלום עולם" {
		t.Fatalf("the Hebrew translation %q reads %s", s.node("translate-out").Text(), got)
	}
}

func TestTheTranslationPageFitsItsWindowDarkAndLight(t *testing.T) {
	s, fake := translator(t)
	s.desked()
	s.writes("Bonjour le monde")
	s.pauses()
	s.replied(fake)
	for _, size := range []geom.Vec2{geom.V2(1200, 760), geom.V2(760, 480)} {
		for _, dark := range []bool{false, true} {
			s.win.SetSize(size)
			s.configure(func(c *config.Config) { c.Appearance = appearance(dark) })
			s.frame()
			s.frame()
			if panes, view := s.box("translate-panes"), s.box("view"); panes.X < view.X || panes.X+panes.W >
				view.X+view.W || s.dark() != dark {
				t.Errorf("at %v the panes %v leave the view %v", size, panes, view)
			}
			s.clean()
		}
	}
}

func TestATextOrALanguageThatChangesStopsTheTranslationAndBeginsItAgain(t *testing.T) {
	s, fake := translator(t)
	s.desked()
	s.writes("Bonjour")
	s.pauses()
	s.until("the model is asked", func() bool { return len(fake.Asked()) == 2 })
	s.replied(fake)
	s.until("the language is named", func() bool { return s.pressed("translate-from") == "Detected: French" })
	s.click("translate-into-fr")
	s.until("the model is asked again", func() bool { return len(fake.Asked()) == 3 })
	s.frame()
	if told := fake.Asked()[2].System; !strings.Contains(told, "into French") || s.config.TranslateInto != "fr" ||
		s.pressed("translate-into") != "Français" || s.node("translate-stands").Text() != "Translating…" {
		t.Fatalf("French chosen, the model is told %q, the setting is %q", told, s.config.TranslateInto)
	}
	s.writes(" le monde")
	s.until("the earlier translation is stopped", func() bool { return fake.Status().State == model.Ready })
	if s.doc.ByID("translate-stop") != nil || len(fake.Asked()) != 3 {
		t.Fatalf("typing again left the translation running, the model asked %d times", len(fake.Asked()))
	}
	s.pauses()
	s.until("the model is asked for the new text", func() bool { return len(fake.Asked()) == 4 })
	if asked := fake.Asked()[3]; asked.Messages[0].Text != asData("Bonjour le monde")+
		"\n\nThe translation into French:" {
		t.Fatalf("for the new text the model is asked %q", asked.Messages[0].Text)
	}
	s.frame()
	s.click("translate-stop")
	s.until("the model stops", func() bool { return fake.Status().State == model.Ready })
	s.frame()
	if got := s.node("translate-stands").Text(); got != "Stopped" || s.doc.ByID("translate-stop") != nil {
		t.Fatalf("stopped, the page says %q", got)
	}
	s.click("translate-from-en")
	s.until("the model is asked from English", func() bool { return len(fake.Asked()) == 5 })
	s.replied(fake)
	if told := fake.Asked()[4].System; !strings.Contains(told, "from English into French") ||
		s.pressed("translate-from") != "English" || len(fake.Asked()) != 5 {
		t.Fatalf("English named, the model is told %q, asked %d times, %q chosen", told, len(fake.Asked()),
			s.pressed("translate-from"))
	}
	s.click("translate-swap")
	s.until("the model is asked the other way", func() bool { return len(fake.Asked()) == 6 })
	s.replied(fake)
	if asked := fake.Asked()[5]; !strings.Contains(asked.System, "from French into American English") ||
		asked.Messages[0].Text != asData("Hello **world**")+"\n\nThe translation into American "+
			"English:" || s.config.TranslateInto != "en-US" ||
		s.doc.Value(s.node("translate-text")) != "Hello **world**" || s.pressed("translate-from") != "Français" {
		t.Fatalf("swapped, the model is told %q and asked %q", asked.System, asked.Messages[0].Text)
	}
	s.click("translate-clear")
	if got := s.doc.Value(s.node("translate-text")) + s.node("translate-out").Text() +
		s.node("translate-stands").Text(); got != "" || s.node("translate-count").Text() != "0 characters" {
		t.Fatalf("cleared, the page shows %q", got)
	}
	s.win.SetClipboard("Au revoir")
	s.click("translate-paste")
	s.until("what was pasted is translated", func() bool { return len(fake.Asked()) == 7 })
	if asked := fake.Asked()[6]; !strings.HasPrefix(asked.Messages[0].Text, asData("Au revoir")+
		"\n\nThe translation into ") ||
		s.node("translate-count").Text() != "9 characters" {
		t.Fatalf("pasted, the model is asked %q", asked.Messages[0].Text)
	}
	s.Open(historyPage)
	s.until("leaving the page stops the translation", func() bool { return fake.Status().State == model.Ready })
	s.clean()
}

func TestALongTextIsTranslatedFromItsStartAndThePageSaysSo(t *testing.T) {
	s, fake := translator(t)
	fake.SetCapabilities(model.Capabilities{Context: 400, Chat: true})
	s.desked()
	long := strings.Repeat("Le conseil a voté mardi soir. ", 80)
	s.win.SetClipboard(long)
	s.click("translate-paste")
	s.until("the model is asked", func() bool { return len(fake.Asked()) >= 1 })
	s.replied(fake)
	given := fake.Asked()[0].Messages[0].Text
	if len(given) >= len(long) || !strings.Contains(given, "Le conseil a voté") ||
		s.node("translate-stands").Text() != "Translated the start of your text, which is long" {
		t.Fatalf("of a long text the model is given %d bytes of %d and the page says %q", len(given), len(long),
			s.node("translate-stands").Text())
	}
	s.clean()
}

func TestTheTranslationPageOnlyOffersAModelWithoutOne(t *testing.T) {
	bare := showWith(t, Options{Transport: &fenced{}, Downloads: t.TempDir()})
	bare.desked()
	if got := bare.node("needs-model").Text(); !strings.Contains(got, "this light has none") ||
		bare.doc.ByID("needs-model-on") != nil || bare.doc.ByID("translate-text") != nil {
		t.Fatalf("a light without a model says %q", got)
	}
	bare.nothingHelped("no model, " + translatePage)
	bare.clean()
	s, fake := translator(t)
	s.configure(func(c *config.Config) { c.Model = false })
	s.desked()
	if got := s.node("needs-model").Text(); !strings.Contains(got, "needs a model on this device, and it is off") ||
		s.doc.ByID("translate-text") != nil || s.node("needs-model-on").Text() != turnOn {
		t.Fatalf("with the model off the page says %q", got)
	}
	s.nothingHelped("the model off, " + translatePage)
	s.clean()
	s.click("needs-model-on")
	if !s.cardShown() {
		t.Fatal("what turns a model on leaves its card closed")
	}
	s.configure(func(c *config.Config) { c.Model = true })
	s.frame()
	s.frame()
	if s.doc.ByID("needs-model") != nil || s.doc.ByID("translate-text") == nil {
		t.Fatal("with the model on the page still asks for one")
	}
	s.writes("Bonjour")
	s.pauses()
	s.until("the model is asked", func() bool { return len(fake.Asked()) >= 1 })
	s.assists.Set(translateFeature, false)
	s.until("the feature off stops the translation", func() bool { return fake.Status().State == model.Ready })
	if got := s.node("needs-model").Text(); !strings.Contains(got, "switched off") ||
		s.doc.ByID("translate-text") != nil || s.node("needs-model-on").Text() != modelName+"…" {
		t.Fatalf("with the feature off the page says %q", got)
	}
	s.clean()
}

func TestTheTranslationPageIsNoVisit(t *testing.T) {
	s, _ := translator(t)
	s.desked()
	s.configure(func(c *config.Config) { c.Model = false })
	s.desked()
	if got, _ := lastVisit(s.Profile.visits, translatePage); got.Address != "" {
		t.Fatal("the translation page is noted in the history")
	}
	s.Open(tabs.Home)
	s.frame()
	s.frame()
	if s.doc.ByID("continue") != nil {
		t.Fatalf("with the model off the start page offers to continue: %q", s.node("continue").Text())
	}
	s.nothingHelped("the start page")
}

func TestTheTranslationPageIsListedOnlyWhileTheModelTranslates(t *testing.T) {
	s, _ := translator(t)
	listed := func() string {
		var out []string
		s.Open(s.config.StartPage)
		s.frame()
		s.frame()
		if strings.Contains(s.node("page").Text(), translateTitle) {
			out = append(out, "the start page")
		}
		s.click("menu")
		if s.doc.ByID("menu-translate") != nil && s.seen(s.node("menu-translate")) {
			out = append(out, "the menu")
		}
		s.press(input.KeyEscape)
		s.Open(own + aboutName)
		s.frame()
		if strings.Contains(s.node("own-pages").Text(), translatePage) {
			out = append(out, "the About page")
		}
		s.Open(tabs.Search("translated"))
		s.frame()
		if strings.Contains(s.node("page").Text(), translatePage) {
			out = append(out, "a search")
		}
		return strings.Join(out, ", ")
	}
	if got := listed(); got != "the menu, the About page, a search" {
		t.Fatalf("with the model translating the page is listed on %q", got)
	}
	s.Open(s.config.StartPage)
	s.frame()
	s.assists.Set(translateFeature, false)
	s.redrawModel()
	s.frame()
	s.frame()
	if got := listed(); got != "" {
		t.Fatalf("with the feature off the page is listed on %q", got)
	}
	s.configure(func(c *config.Config) { c.Model = false })
	s.assists.Set(translateFeature, true)
	if got := listed(); got != "" {
		t.Fatalf("with the model off the page is listed on %q", got)
	}
	s.clean()
	s.configure(func(c *config.Config) { c.Model = true })
	s.frame()
	s.frame()
	s.pick("menu-translate")
	s.frame()
	if got := s.browser.Tab().Current(); got != translatePage || s.doc.ByID("translate-text") == nil {
		t.Fatalf("the menu's row opened %q", got)
	}
}
