package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its settings

import (
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/script"
	"github.com/lantervale/light/pkg/search"
	"github.com/lantervale/light/pkg/tabs"
)

// outside stands for every site: it answers each request with a small
// page, and keeps what it was asked.
type outside struct {
	sync.Mutex
	asked []*http.Request
	// icon is the picture every site has for its icon, nil for none.
	icon []byte
}

func (o *outside) RoundTrip(r *http.Request) (*http.Response, error) {
	o.Lock()
	o.asked = append(o.asked, r)
	o.Unlock()
	if r.URL.Path == "/favicon.ico" && o.icon != nil {
		return &http.Response{
			Status: "200 OK", StatusCode: http.StatusOK, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Request: r,
			Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader(string(o.icon))),
		}, nil
	}
	return &http.Response{
		Status: "200 OK", StatusCode: http.StatusOK, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Request: r,
		Header: http.Header{"Content-Type": {"text/html"}},
		Body:   io.NopCloser(strings.NewReader(`<title>Found</title><p id="found">what was found</p>`)),
	}, nil
}

// addresses are the addresses asked for, in the order they were, but
// the icon light asks any site it shows a page of.
func (o *outside) addresses() []string {
	o.Lock()
	defer o.Unlock()
	var all []string
	for _, r := range o.asked {
		if r.URL.Path != "/favicon.ico" {
			all = append(all, r.Method+" "+r.URL.String())
		}
	}
	return all
}

// said is every header the request for address came with, a line each,
// sorted.
func (o *outside) said(address string) string {
	o.Lock()
	defer o.Unlock()
	var lines []string
	for _, r := range o.asked {
		if r.URL.String() != address {
			continue
		}
		for name, values := range r.Header {
			lines = append(lines, name+": "+strings.Join(values, ", "))
		}
	}
	slices.Sort(lines)
	return strings.Join(lines, "\n")
}

// searchingWith is a light whose requests all go to an outside that is no
// site of the Internet.
func searchingWith(t *testing.T, opts Options) (*shown, *outside) {
	t.Helper()
	o := &outside{}
	opts.Transport, opts.Downloads = o, t.TempDir()
	return showWith(t, opts), o
}

// types has the keys of text come to what has the focus, one a frame.
func (s *shown) types(text string) {
	for _, r := range text {
		s.in.AddText(r)
		s.frame()
	}
	s.frame()
}

// loaded runs frames until the page at address shows.
func (s *shown) loaded(address string) {
	s.t.Helper()
	s.until(address+" shows", func() bool {
		return s.browser.Tab().Current() == address && s.session().pages[address] != nil
	})
	s.frame()
}

// typedOverTLS is the headers of an address typed, asked over https.
func typedOverTLS() string {
	return strings.Replace(navigation("none", "", true), "gzip, deflate", "gzip, deflate, br", 1)
}

// found is the address DuckDuckGo is asked for query at, in this
// build: its page that needs no script when there is no engine for
// them.
func found(query string) string {
	return search.Named(search.DuckDuckGo).Address(query, script.Available())
}

// ducks is where DuckDuckGo is asked for the words these tests type.
func ducks() string { return found("ember game engine") }

func TestALightThatRunsNoScriptAsksDuckDuckGoForItsPageThatNeedsNone(t *testing.T) {
	s, _ := searchingWith(t, Options{})
	const scripted, plain = "https://duckduckgo.com/?q=tile+maps", "https://html.duckduckgo.com/html/?q=tile+maps"
	want := plain
	if script.Available() {
		want = scripted
	}
	if got := s.Resolve("tile maps"); got != want {
		t.Fatalf("with JavaScript on words are opened as %q, want %q", got, want)
	}
	s.configure(func(c *config.Config) { c.JavaScript = false })
	if got := s.Resolve("tile maps"); got != plain {
		t.Fatalf("with JavaScript off words are opened as %q, want %q", got, plain)
	}
}

func TestWordsTypedInTheAddressBarGoToDuckDuckGoOnEnterAndNothingGoesBefore(t *testing.T) {
	s, o := searchingWith(t, Options{})
	s.click("address")
	s.command(input.KeyA)
	s.types("ember game engine")
	for range 30 {
		s.frame()
	}
	if got := o.addresses(); len(got) != 0 {
		t.Fatalf("typing asked for %v: nothing is to go before Enter", got)
	}
	s.press(input.KeyEnter)
	s.loaded(ducks())
	if got := o.addresses(); len(got) != 1 || got[0] != "GET "+ducks() {
		t.Fatalf("the search asked for %v, want the engine's page alone", got)
	}
	if got, want := o.said(ducks()), typedOverTLS(); got != want {
		t.Fatalf("the engine was told:\n%s\nwant what any address typed is:\n%s", got, want)
	}
	s.enter("https://example.test/?q=ember+game+engine")
	s.loaded("https://example.test/?q=ember+game+engine")
	if got, want := o.said("https://example.test/?q=ember+game+engine"), o.said(ducks()); got != want {
		t.Fatalf("an address typed says:\n%s\nand a search:\n%s", got, want)
	}
}

func TestTheStartPagesFieldSearchesWithTheSameEngineOnEnterOnly(t *testing.T) {
	s, o := searchingWith(t, Options{})
	field := s.node("search")
	if got, _ := field.Attr("placeholder"); got != "Search with DuckDuckGo or type an address" {
		t.Fatalf("the field says %q", got)
	}
	s.click("search")
	s.types("ember game engine")
	for range 30 {
		s.frame()
	}
	if got := o.addresses(); len(got) != 0 {
		t.Fatalf("typing asked for %v", got)
	}
	s.press(input.KeyEnter)
	s.loaded(ducks())
	if got := o.addresses(); len(got) != 1 || got[0] != "GET "+ducks() {
		t.Fatalf("the search asked for %v", got)
	}
	s.configure(func(c *config.Config) { c.SearchEngine = search.Own })
	s.click("home")
	s.frame()
	if got, _ := s.node("search").Attr("placeholder"); got != "Search light's own pages or type an address" {
		t.Fatalf("with light's own pages the field says %q", got)
	}
	s.click("search")
	s.types("tile maps")
	s.press(input.KeyEnter)
	s.frame()
	s.at(tabs.Search("tile maps"))
	if got := o.addresses(); len(got) != 1 {
		t.Fatalf("a search of light's own pages asked for %v", got)
	}
}

func TestTheSearchPageIsAnAddressWhateverTheEngineAndAnAddressIsNeverSearched(t *testing.T) {
	s, o := searchingWith(t, Options{})
	s.enter("light://search?q=tile+maps")
	s.at(tabs.Search("tile maps"))
	if text := s.node("page").Text(); !strings.Contains(text, "Examples") || s.doc.ByID("none") != nil {
		t.Fatalf("the results are %q", text)
	}
	s.enter("about:blank")
	s.at("about:blank")
	s.enter("light://config")
	s.at(configPage)
	if got := o.addresses(); len(got) != 0 {
		t.Fatalf("light's own addresses asked for %v", got)
	}
	s.enter("localhost:8080")
	s.loaded("http://localhost:8080")
	if got := o.addresses(); len(got) != 1 || got[0] != "GET http://localhost:8080" {
		t.Fatalf("this machine typed asked for %v", got)
	}
}

func TestTheEngineIsChosenInTheSettingsAndKeptBetweenRuns(t *testing.T) {
	dir := t.TempDir()
	s, o := searchingWith(t, Options{Dir: dir})
	s.settings()
	choice := s.node("set-search-engine")
	if got := s.doc.Value(choice); got != search.DuckDuckGo {
		t.Fatalf("the settings start with %q", got)
	}
	var names []string
	for _, option := range choice.Children() {
		names = append(names, option.Text())
	}
	if got := strings.Join(names, ", "); got != "DuckDuckGo, light's own pages" {
		t.Fatalf("the choices are %s", got)
	}
	hint := choice.Parent().Parent().Text()
	if !strings.Contains(hint, "typed in the address bar") || !strings.Contains(hint, "not an address") ||
		!strings.Contains(hint, "sent to") || !strings.Contains(hint, "Enter") {
		t.Fatalf("the choice is explained as %q", hint)
	}
	s.click("set-search-engine")
	s.press(input.KeyDown)
	s.press(input.KeyEnter)
	if s.config.SearchEngine != search.Own {
		t.Fatalf("after choosing the second engine the setting is %q", s.config.SearchEngine)
	}
	s.enter("tile maps")
	s.at(tabs.Search("tile maps"))
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if kept, err := config.Load(filepath.Join(dir, configFile)); err != nil || kept.SearchEngine != search.Own {
		t.Fatalf("the file holds %+v, %v", kept, err)
	}
	again, _ := searchingWith(t, Options{Dir: dir})
	again.settings()
	if got := again.doc.Value(again.node("set-search-engine")); got != search.Own {
		t.Fatalf("the next run shows %q", got)
	}
	again.click("set-reset")
	if got := again.doc.Value(again.node("set-search-engine")); got != search.DuckDuckGo ||
		again.config.SearchEngine != search.DuckDuckGo {
		t.Fatalf("reset left %q shown, %q set", got, again.config.SearchEngine)
	}
	if got := o.addresses(); len(got) != 0 {
		t.Fatalf("choosing an engine asked for %v", got)
	}
}

func TestALightNobodyTypesInSearchesNothing(t *testing.T) {
	for _, opts := range []Options{{}, {Driven: true}} {
		s, o := searchingWith(t, opts)
		for _, address := range []string{tabs.Home, configPage, tabs.Search("ember"), tabs.Page(aboutName)} {
			s.Open(address)
			for range 20 {
				s.frame()
			}
		}
		s.click("new-tab")
		s.click("address")
		s.command(input.KeyA)
		s.types("ember")
		s.press(input.KeyEscape)
		s.click("home")
		for range 20 {
			s.frame()
		}
		if got := o.addresses(); len(got) != 0 {
			t.Fatalf("a light with %+v asked for %v by itself", opts, got)
		}
	}
}

func TestAPositionalAddressIsReadAsTheAddressBarReadsIt(t *testing.T) {
	s, _ := searchingWith(t, Options{})
	for typed, want := range map[string]string{
		"ember game engine": ducks(),
		"example.com":       "https://example.com",
		"localhost:8080":    "http://localhost:8080",
		"light://guide":     "light://guide",
		"":                  tabs.Home,
	} {
		if got := s.Resolve(typed); got != want {
			t.Errorf("%q is opened as %q, want %q", typed, got, want)
		}
	}
	s.configure(func(c *config.Config) { c.SearchEngine = search.Own })
	if got := s.Resolve("ember game engine"); got != tabs.Search("ember game engine") {
		t.Fatalf("with light's own pages words are opened as %q", got)
	}
}

func TestSelectedTextIsSearchedForFromTheMenuInATabOfItsOwn(t *testing.T) {
	s, o := searchingWith(t, Options{})
	page := "https://site.test/"
	s.Open(page)
	s.loaded(page)
	doc := s.session().pages[page].Doc
	words := doc.ByID("found").Children()[0]
	doc.Select(ui.Position{Node: words, Offset: 0}, ui.Position{Node: words, Offset: 8})
	s.rightClickAt(s.inPage("found"))
	if got := labels(s.entries()); got != "[Copy Search DuckDuckGo for “what was” - Inspect element]" {
		t.Fatalf("selected text offers %s", got)
	}
	if got := o.addresses(); len(got) != 1 {
		t.Fatalf("opening the menu asked for %v", got)
	}
	s.choose("Search DuckDuckGo for “what was”")
	results := found("what was")
	s.loaded(results)
	if got, want := s.opened(), page+" ["+results+"]"; got != want {
		t.Fatalf("the tabs are %s, want %s", got, want)
	}
	if got, want := o.said(results), typedOverTLS(); got != want {
		t.Fatalf("the engine was told:\n%s\nwant:\n%s", got, want)
	}
}

func TestALongSelectionIsShortenedInTheMenuAndSearchedWhole(t *testing.T) {
	long := strings.Repeat("vault ", 12)
	if got := searchLabel("DuckDuckGo", long); got != "Search DuckDuckGo for “vault vault vault vault vault…”" {
		t.Fatalf("the entry reads %q", got)
	}
	if got := searchLabel("light's own pages", " two\n words "); got != "Search light's own pages for “two words”" {
		t.Fatalf("the entry reads %q", got)
	}
}

// depicts is the picture the element id shows; "lock" is what stands
// before the address: the engine's mark while words are typed.
func (s *shown) depicts(id string) string {
	s.t.Helper()
	if id == "lock" && s.field.HasClass("searching") {
		id = "sought"
	}
	src, _ := s.node(id).Attr("src")
	return src
}

func TestTheEnginesMarkShowsWhereLightSearchesAndNothingIsFetchedForIt(t *testing.T) {
	s, o := searchingWith(t, Options{})
	o.icon = picture(t, 16, 16)
	if got := s.depicts("search-mark"); got != "search.svg" {
		t.Fatalf("the start page's field is marked %q before the engine was visited", got)
	}
	s.click("address")
	s.command(input.KeyA)
	s.types("ember")
	if got := s.depicts("lock"); got != "search.svg" {
		t.Fatalf("words typed are marked %q", got)
	}
	s.types(".com")
	if got := s.depicts("lock"); got != "open.svg" {
		t.Fatalf("an address typed is marked %q", got)
	}
	s.types(" games")
	if got := s.depicts("lock"); got != "search.svg" {
		t.Fatalf("words typed after an address are marked %q", got)
	}
	s.settings()
	if got := s.depicts("engine-mark"); got != "search.svg" {
		t.Fatalf("the settings mark the engine %q", got)
	}
	if got := o.addresses(); len(got) != 0 || len(o.asked) != 0 {
		t.Fatalf("marking the engine asked for %v", o.asked)
	}
	s.Open(ducks())
	s.loaded(ducks())
	s.until("the engine's icon is kept", func() bool { return s.mark() != "search.svg" })
	icon := faviconOf(ducks())
	s.settings()
	if got := s.depicts("engine-mark"); got != icon {
		t.Fatalf("once visited, the settings mark the engine %q, want %q", got, icon)
	}
	s.click("address")
	s.command(input.KeyA)
	s.types("ember")
	if got := s.depicts("lock"); got != icon {
		t.Fatalf("once visited, words typed are marked %q, want %q", got, icon)
	}
	s.press(input.KeyEscape)
	s.configure(func(c *config.Config) { c.SearchEngine = search.Own })
	if got := s.depicts("engine-mark"); got != logo {
		t.Fatalf("light's own pages are marked %q in the settings", got)
	}
	s.Open(tabs.Home)
	s.frame()
	if got := s.depicts("search-mark"); got != logo {
		t.Fatalf("light's own pages are marked %q on the start page", got)
	}
	s.configure(func(c *config.Config) { c.SearchEngine = search.DuckDuckGo })
	if got := s.depicts("search-mark"); got != icon {
		t.Fatalf("back to DuckDuckGo the start page is marked %q, want %q", got, icon)
	}
}
