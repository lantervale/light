package config_test

import (
	"testing"

	"github.com/lantervale/light/pkg/config"
)

func TestTheRuleForThePagesOfALanguageIsKeptByItsLanguage(t *testing.T) {
	c := config.Default()
	if c.TranslateAlways != "" || c.TranslateNever != "" || c.PageRule("fr") != config.AskToTranslate {
		t.Fatalf("by default the pages in French are %v", c.PageRule("fr"))
	}
	c.TranslateAlways, c.TranslateNever = " FR-ca, de,  fr ,x, 12, <b>", "de-AT; it"
	c = c.Valid()
	if c.TranslateAlways != "fr" || c.TranslateNever != "de, it" {
		t.Fatalf("the rules are always %q, never %q", c.TranslateAlways, c.TranslateNever)
	}
	for tag, want := range map[string]config.PageRule{
		"fr": config.AlwaysTranslate, "fr-FR": config.AlwaysTranslate, "FR": config.AlwaysTranslate,
		"de": config.NeverTranslate, "it-IT": config.NeverTranslate, "es": config.AskToTranslate,
		"": config.AskToTranslate, "french": config.AskToTranslate,
	} {
		if got := c.PageRule(tag); got != want {
			t.Errorf("the pages in %q are %v, want %v", tag, got, want)
		}
	}
	c = c.WithPageRule("fr-BE", config.NeverTranslate).WithPageRule("de", config.AskToTranslate).
		WithPageRule("es", config.AlwaysTranslate).WithPageRule("", config.AlwaysTranslate)
	if c.TranslateAlways != "es" || c.TranslateNever != "it, fr" {
		t.Fatalf("changed, the rules are always %q, never %q", c.TranslateAlways, c.TranslateNever)
	}
	data, err := c.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if read, unread := config.Decode(data); unread != nil || read != c {
		t.Fatalf("read again the settings are %+v, %v", read, unread)
	}
}

func TestALanguageIsTheFirstPartOfItsTag(t *testing.T) {
	for _, c := range [][2]string{{"fr", "fr"}, {"en-GB", "en"}, {" PT_br ", "pt"}, {"fil", "fil"}, {"", ""},
		{"e", ""}, {"engl", ""}, {"f1", ""}, {"fr-", "fr"}} {
		if got := config.Language(c[0]); got != c[1] {
			t.Errorf("the language of %q is %q, want %q", c[0], got, c[1])
		}
	}
}
