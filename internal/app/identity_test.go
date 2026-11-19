package app //nolint:testpackage // drives the Screen through its unexported parts

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/lantervale/light/pkg/identity"
)

// told is a site that keeps the headers each of its paths was last
// asked with, whole. Its page asks for one thing of each kind.
type told struct {
	*httptest.Server
	mu  sync.Mutex
	got map[string]http.Header
	// script is what the page's script element holds.
	script string
}

const toldPage = `<title>Told</title><link rel="stylesheet" href="/site.css"><body>
	<p id="words">Set in the page's font.</p>
	<img id="map" src="/map.png" alt="The map">
	<a id="near" href="/vaults">near</a> <a id="far" href="FAR/vaults">far</a>
	<a id="kept" href="/file.bin" download="file.bin">kept</a>
	<form action="/search"><input name="q" value="lamp"><input type="submit" id="find" value="Find"></form>
	<form action="/save" method="post"><input name="a" value="1"><input type="submit" id="send" value="Send"></form>
	<script src="/app.js"></script><script>SCRIPT</script></body>`

func telling(t *testing.T) *told {
	t.Helper()
	site := &told{got: map[string]http.Header{}}
	site.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site.mu.Lock()
		site.got[r.Method+" "+r.URL.Path] = r.Header.Clone()
		site.mu.Unlock()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			page := strings.ReplaceAll(toldPage, "FAR", site.named())
			_, _ = io.WriteString(w, strings.ReplaceAll(page, "SCRIPT", site.script))
		case "/site.css":
			w.Header().Set("Content-Type", "text/css")
			_, _ = io.WriteString(w, `@font-face { font-family: Own; src: url(/fonts/own.woff2) }
				body { margin: 0 } #words { font-family: Own } a, form { display: block; height: 24px }`)
		case "/map.png", "/favicon.ico":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(picture(t, 30, 20))
		case "/app.js":
			w.Header().Set("Content-Type", "text/javascript")
		case "/file.bin":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, "kept")
		case "/save":
			http.Redirect(w, r, "/saved", http.StatusSeeOther)
		case "/hop":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, `<meta http-equiv="refresh" content="0; url=/vaults">`)
		case "/old":
			http.Redirect(w, r, site.named()+"/vaults", http.StatusFound)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<title>"+r.URL.Path+"</title><p>"+r.URL.Path+"</p>")
		}
	}))
	t.Cleanup(site.Close)
	return site
}

// named is the site's address by its name: another site than by its
// number.
func (k *told) named() string { return strings.Replace(k.URL, "127.0.0.1", "localhost", 1) }

func (k *told) came(path string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.got[path] != nil
}

func (k *told) forget() {
	k.mu.Lock()
	defer k.mu.Unlock()
	clear(k.got)
}

// said is every header the request for a path came with, a line each,
// sorted.
func (k *told) said(path string) string {
	k.mu.Lock()
	defer k.mu.Unlock()
	var lines []string
	for name, values := range k.got[path] {
		lines = append(lines, name+": "+strings.Join(values, ", "))
	}
	slices.Sort(lines)
	return strings.Join(lines, "\n")
}

// every is the headers any request of light carries, with those given,
// sorted as said sorts them.
func every(more ...string) string {
	lines := append([]string{"Accept-Encoding: gzip, deflate", "Accept-Language: en-US,en;q=0.9",
		"User-Agent: " + identity.Default("")}, more...)
	slices.Sort(lines)
	return strings.Join(lines, "\n")
}

const acceptsPages = "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"

// navigation is the headers of a navigation from a site, by the user
// or not.
func navigation(site, referer string, user bool) string {
	lines := []string{acceptsPages, "Sec-Fetch-Dest: document", "Sec-Fetch-Mode: navigate", "Sec-Fetch-Site: " + site,
		"Upgrade-Insecure-Requests: 1"}
	if referer != "" {
		lines = append(lines, "Referer: "+referer)
	}
	if user {
		lines = append(lines, "Sec-Fetch-User: ?1")
	}
	return every(lines...)
}

// part is the headers of a request for a part of the page at referer:
// kind is what it is for and how it is made, as "image no-cors".
func part(accept, kind, referer string) string {
	dest, mode, _ := strings.Cut(kind, " ")
	return every("Accept: "+accept, "Sec-Fetch-Dest: "+dest, "Sec-Fetch-Mode: "+mode, "Sec-Fetch-Site: same-origin",
		"Referer: "+referer)
}

func (s *shown) heard(site *told, what, path, want string) {
	s.t.Helper()
	s.until(what+" was asked for", func() bool { return site.came(path) })
	if got := site.said(path); got != want {
		s.t.Errorf("%s (%s) said:\n%s\nwant:\n%s", what, path, got, want)
	}
}

func TestWhatLightTellsASiteForEachKindOfRequest(t *testing.T) {
	site := telling(t)
	s := show(t)
	page := site.URL + "/"
	s.enter(page)
	s.heard(site, "an address typed", "GET /", navigation("none", "", true))
	s.heard(site, "a sheet", "GET /site.css", part("text/css,*/*;q=0.1", "style no-cors", page))
	const pictures = "image/webp,image/png,image/svg+xml,image/*;q=0.8,*/*;q=0.5"
	s.heard(site, "a picture", "GET /map.png", part(pictures, "image no-cors", page))
	s.heard(site, "the site's icon", "GET /favicon.ico", part(pictures, "image no-cors", page))
	s.heard(site, "a font", "GET /fonts/own.woff2", part("*/*", "font no-cors", site.URL+"/site.css"))
	s.until("the page shows", func() bool { return s.session().pages[page] != nil })
	s.frame()

	s.clickAt(s.inPage("near"))
	s.heard(site, "a link clicked", "GET /vaults", navigation("same-origin", page, true))

	s.visit(page)
	site.forget()
	s.clickAt(s.inPage("far"))
	s.heard(site, "a link to another site", "GET /vaults", navigation("cross-site", page, true))

	s.visit(page)
	s.clickAt(s.inPage("find"))
	s.heard(site, "a form sent with a GET", "GET /search", navigation("same-origin", page, true))

	s.visit(page)
	s.clickAt(s.inPage("send"))
	s.heard(site, "a form sent with a POST", "POST /save", every(acceptsPages, "Content-Length: 3",
		"Content-Type: application/x-www-form-urlencoded", "Origin: "+site.URL, "Referer: "+page,
		"Sec-Fetch-Dest: document", "Sec-Fetch-Mode: navigate", "Sec-Fetch-Site: same-origin", "Sec-Fetch-User: ?1",
		"Upgrade-Insecure-Requests: 1"))
	s.heard(site, "where the POST redirects", "GET /saved", navigation("same-origin", page, true))

	s.visit(page)
	s.clickAt(s.inPage("kept"))
	s.heard(site, "a download", "GET /file.bin", part("*/*", "empty no-cors", page))
	s.until("the download ended", func() bool { return s.downloaded("file.bin") == "kept" })

	site.forget()
	s.visit(site.URL + "/hop")
	s.heard(site, "a page's refresh", "GET /vaults", navigation("same-origin", site.URL+"/hop", false))

	site.forget()
	s.visit(site.URL + "/old")
	s.heard(site, "a typed address redirected to another site", "GET /vaults", navigation("none", "", true))

	s.until("it landed", func() bool { return s.session().pages[site.named()+"/vaults"] != nil })
	site.forget()
	s.click("reload")
	s.heard(site, "a reload", "GET /vaults", navigation("none", "", true))
}

func TestASiteInTheClearIsToldNothingOfTheFetch(t *testing.T) {
	var mu sync.Mutex
	got := map[string]http.Header{}
	n := names(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got[r.Host+r.URL.Path] = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<title>Clear</title><a id="on" href="http://img.site.test/next">on</a>`)
	})
	s := showWith(t, Options{Transport: n.transport, Downloads: t.TempDir()})
	s.visit("http://www.site.test/")
	s.clickAt(s.inPage("on"))
	s.until("the link was followed", func() bool { return n.came("img.site.test/next") })
	mu.Lock()
	defer mu.Unlock()
	for at, referer := range map[string]string{"www.site.test/": "", "img.site.test/next": "http://www.site.test/"} {
		h := got[at]
		for name := range h {
			if strings.HasPrefix(name, "Sec-") {
				t.Errorf("%s was told %s: %s", at, name, h.Get(name))
			}
		}
		if h.Get("Referer") != referer || h.Get("Upgrade-Insecure-Requests") != "1" || h.Get("Accept") == "" {
			t.Errorf("%s was told %v", at, h)
		}
	}
}

func TestTheSettingsSayWhatLightTellsASite(t *testing.T) {
	site := telling(t)
	dir := t.TempDir()
	former := `{"version": 1, "user_agent": "light/1.0 (ember)"}`
	if err := os.WriteFile(filepath.Join(dir, configFile), []byte(former), 0o600); err != nil {
		t.Fatal(err)
	}
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir(), Version: "v0.4.2", Languages: []string{"fr-FR", "en-US"}})
	own := identity.Default("0.4.2")
	s.settings()
	agent, languages := s.node("set-user-agent"), s.node("set-accept-language")
	if placeholder, _ := agent.Attr("placeholder"); s.doc.Value(agent) != "" || placeholder != own ||
		!strings.HasSuffix(own, " light/0.4.2") {
		t.Fatalf("the User-Agent an earlier light wrote shows as %q over %q", s.doc.Value(agent), placeholder)
	}
	if placeholder, _ := languages.Attr("placeholder"); s.doc.Value(languages) != "" ||
		placeholder != "fr-FR, fr, en-US, en" {
		t.Fatalf("the languages show as %q over %q", s.doc.Value(languages), placeholder)
	}
	said := func(what, wantAgent, wantLanguages string) {
		t.Helper()
		path := "/" + strings.ReplaceAll(what, " ", "-")
		s.visit(site.URL + path)
		h := site.said("GET " + path)
		if !strings.Contains(h, "\nUser-Agent: "+wantAgent) || !strings.Contains(h, "Accept-Language: "+wantLanguages+"\n") {
			t.Errorf("%s, the site was told:\n%s", what, h)
		}
		s.Open(configPage)
		s.frame()
	}
	said("with nothing set", own, "fr-FR,fr;q=0.9,en-US;q=0.8,en;q=0.7")
	s.set("set-user-agent", "Probe/2.0")
	s.set("set-accept-language", "de_ch, en;q=0.5")
	if got := s.doc.Value(s.node("set-accept-language")); got != "de-CH, en" {
		t.Errorf("the languages typed are kept as %q", got)
	}
	said("once set", "Probe/2.0", "de-CH,de;q=0.9,en;q=0.8")
	s.set("set-user-agent", "")
	s.set("set-accept-language", "")
	said("emptied again", own, "fr-FR,fr;q=0.9,en-US;q=0.8,en;q=0.7")
	s.set("set-user-agent", "Probe/2.0")
	s.click("set-reset")
	if s.config.UserAgent != "" || s.doc.Value(s.node("set-user-agent")) != "" {
		t.Errorf("reset, the User-Agent is %q", s.config.UserAgent)
	}
}
