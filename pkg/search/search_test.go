package search_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/search"
	"github.com/lantervale/light/pkg/tabs"
)

func TestDuckDuckGoIsTheDefaultAndLightsOwnPagesTheOtherChoice(t *testing.T) {
	engines := search.Engines()
	if len(engines) != 2 || engines[0].ID != search.DuckDuckGo || engines[1].ID != search.Own {
		t.Fatalf("the engines are %+v", engines)
	}
	if search.Default != search.DuckDuckGo || search.Named("").ID != search.DuckDuckGo ||
		search.Named("altavista").ID != search.DuckDuckGo {
		t.Fatalf("the default is %q, an unknown one %+v", search.Default, search.Named("altavista"))
	}
	if !search.Known(search.Own) || search.Known("altavista") || search.Known("") {
		t.Fatal("Known is wrong about an engine")
	}
	engines[0].Results = "https://elsewhere.test/?q={query}"
	if got := search.Named(search.DuckDuckGo).Results; strings.Contains(got, "elsewhere") {
		t.Fatalf("the table was changed from outside: %s", got)
	}
}

func TestAnEnginesAddressHoldsTheWordsAsAFormsValueAndNothingElse(t *testing.T) {
	for query, want := range map[string]string{
		"ember game engine": "https://duckduckgo.com/?q=ember+game+engine",
		"a&b=c #d?":         "https://duckduckgo.com/?q=a%26b%3Dc+%23d%3F",
		"1+1 = 100%":        "https://duckduckgo.com/?q=1%2B1+%3D+100%25",
		"crème brûlée":      "https://duckduckgo.com/?q=cr%C3%A8me+br%C3%BBl%C3%A9e",
		"{query}":           "https://duckduckgo.com/?q=%7Bquery%7D",
	} {
		got := search.Named(search.DuckDuckGo).Address(query, true)
		if got != want {
			t.Errorf("%q is searched at %s, want %s", query, got, want)
		}
		u, err := url.Parse(got)
		if err != nil || len(u.Query()) != 1 || u.Query().Get("q") != query || u.Fragment != "" {
			t.Errorf("%s holds %v (%v), want q alone", got, u.Query(), err)
		}
	}
	for _, scripts := range []bool{true, false} {
		if got := search.Named(search.Own).Address("tile maps", scripts); got != tabs.Search("tile maps") {
			t.Fatalf("light's own pages are searched at %s", got)
		}
	}
}

func TestABrowserThatRunsNoScriptIsSentToThePageThatNeedsNone(t *testing.T) {
	got := search.Named(search.DuckDuckGo).Address("ember game engine", false)
	if got != "https://html.duckduckgo.com/html/?q=ember+game+engine" {
		t.Fatalf("without scripts DuckDuckGo is asked at %s", got)
	}
	u, err := url.Parse(got)
	if err != nil || len(u.Query()) != 1 || u.Query().Get("q") != "ember game engine" {
		t.Fatalf("%s holds %v (%v), want q alone", got, u.Query(), err)
	}
}

func TestEveryEngineIsNamedAndTakesTheWordsOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range search.Engines() {
		if e.ID == "" || e.Name == "" || seen[e.ID] || strings.Count(e.Results, "{query}") != 1 ||
			e.Plain != "" && strings.Count(e.Plain, "{query}") != 1 {
			t.Errorf("the engine %+v is not one a setting can name", e)
		}
		seen[e.ID] = true
		for _, scripts := range []bool{true, false} {
			u, err := url.Parse(e.Address("x", scripts))
			if err != nil || u.Scheme != "https" && u.Scheme != tabs.Scheme {
				t.Errorf("%s is searched at %s (%v)", e.Name, e.Address("x", scripts), err)
			}
		}
	}
}

func TestAnEngineKnowsThePagesOfItsSite(t *testing.T) {
	ducks := search.Named(search.DuckDuckGo)
	for address, want := range map[string]bool{
		"https://duckduckgo.com/?q=x":           true,
		"https://html.duckduckgo.com/html/?q=x": true,
		"https://duckduckgo.com/about":          true,
		"http://duckduckgo.com/":                false,
		"https://duckduckgo.com.evil.test/":     false,
		"https://example.com/?q=duckduckgo.com": false,
		"light://search?q=x":                    false,
		"":                                      false,
	} {
		if got := ducks.Serves(address); got != want {
			t.Errorf("DuckDuckGo serves %q: %v, want %v", address, got, want)
		}
	}
	if !search.Named(search.Own).Serves(tabs.Search("x")) {
		t.Error("light's own search is no page of its engine")
	}
}
