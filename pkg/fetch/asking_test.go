package fetch_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/identity"
)

const (
	agent     = "Mozilla/5.0 (X11; Linux x86_64) light/0.1.0-alpha"
	languages = "fr-FR,fr;q=0.9,en;q=0.8"
)

// heard is a server that redirects /a to /b on its other name, /b back
// to /c on the first, and keeps the headers each path was asked with.
type heard struct {
	sync.Mutex
	got          map[string]http.Header
	here, beside string
}

func hearing(t *testing.T) *heard {
	t.Helper()
	h := &heard{got: map[string]http.Header{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.Lock()
		h.got[r.Method+" "+r.URL.Path] = r.Header.Clone()
		h.Unlock()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		switch {
		case r.Method == http.MethodOptions:
		case r.URL.Path == "/a":
			http.Redirect(w, r, h.beside+"/b", http.StatusFound)
		case r.URL.Path == "/b":
			http.Redirect(w, r, h.here+"/c", http.StatusFound)
		}
	}))
	t.Cleanup(srv.Close)
	h.here, h.beside = srv.URL, strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
	return h
}

// said is what the request for a path said of itself: the headers
// named, each as "Name: value", those it did not carry left out.
func (h *heard) said(path string, names ...string) string {
	h.Lock()
	defer h.Unlock()
	var out []string
	for _, name := range names {
		if value := h.got[path].Get(name); value != "" {
			out = append(out, name+": "+value)
		}
	}
	return strings.Join(out, "\n")
}

func parsed(t *testing.T, address string) *url.URL {
	t.Helper()
	u, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestARequestIsDescribedAgainAtEachHopOfItsRedirects(t *testing.T) {
	h := hearing(t)
	c := fetch.New(nil, fetch.Agent(agent), fetch.Languages(languages))
	page := parsed(t, h.here+"/page?q=1#top")
	ctx := fetch.Asking(fetch.By(t.Context(), page), identity.Request{Destination: identity.Image})
	if _, err := c.Get(ctx, h.here+"/a", "ignored/for-a-described-request"); err != nil {
		t.Fatal(err)
	}
	names := []string{"Accept", "Accept-Language", "User-Agent", "Referer", "Sec-Fetch-Dest", "Sec-Fetch-Mode",
		"Sec-Fetch-Site", "Sec-Fetch-User", "Upgrade-Insecure-Requests"}
	common := "Accept: image/webp,image/png,image/svg+xml,image/*;q=0.8,*/*;q=0.5\nAccept-Language: " + languages +
		"\nUser-Agent: " + agent + "\n"
	const image = "\nSec-Fetch-Dest: image\nSec-Fetch-Mode: no-cors\nSec-Fetch-Site: "
	for path, want := range map[string]string{
		"GET /a": "Referer: " + h.here + "/page?q=1" + image + "same-origin",
		"GET /b": "Referer: " + h.here + "/" + image + "cross-site",
		"GET /c": "Referer: " + h.here + "/page?q=1" + image + "cross-site",
	} {
		if got := h.said(path, names...); got != common+want {
			t.Errorf("%s said:\n%s\nwant:\n%s", path, got, common+want)
		}
	}
}

func TestANavigationTheUserBeganSaysSoAtEachHopAndNamesNoPage(t *testing.T) {
	h := hearing(t)
	c := fetch.New(nil, fetch.Agent(agent), fetch.Languages(languages))
	ctx := fetch.Asking(t.Context(), identity.Request{Destination: identity.Document, User: true})
	if _, err := c.Get(ctx, h.here+"/a", ""); err != nil {
		t.Fatal(err)
	}
	want := "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8\n" +
		"Sec-Fetch-Dest: document\nSec-Fetch-Mode: navigate\nSec-Fetch-Site: none\nSec-Fetch-User: ?1\n" +
		"Upgrade-Insecure-Requests: 1"
	for _, path := range []string{"GET /a", "GET /b", "GET /c"} {
		if got := h.said(path, "Accept", "Referer", "Sec-Fetch-Dest", "Sec-Fetch-Mode", "Sec-Fetch-Site",
			"Sec-Fetch-User", "Upgrade-Insecure-Requests"); got != want {
			t.Errorf("%s said:\n%s\nwant:\n%s", path, got, want)
		}
	}
}

func TestAReferrerGivenToARequestIsItsOwnAtEachHop(t *testing.T) {
	h := hearing(t)
	c := fetch.New(nil)
	sheet := h.beside + "/css/site.css"
	ctx := fetch.Asking(fetch.By(t.Context(), parsed(t, h.here+"/page")),
		identity.Request{Destination: identity.Font, Referrer: parsed(t, sheet)})
	if _, err := c.Get(ctx, h.here+"/a", ""); err != nil {
		t.Fatal(err)
	}
	ctx = fetch.Referring(fetch.Asking(fetch.By(t.Context(), parsed(t, h.here+"/page")),
		identity.Request{Destination: identity.Script}), sheet)
	if _, err := c.Get(ctx, h.beside+"/b", ""); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"GET /a": "Referer: " + h.beside + "/\nSec-Fetch-Dest: font",
		"GET /b": "Referer: " + sheet + "\nSec-Fetch-Dest: script",
		"GET /c": "Referer: " + h.beside + "/\nSec-Fetch-Dest: script",
	} {
		if got := h.said(path, "Referer", "Sec-Fetch-Dest"); got != want {
			t.Errorf("%s said:\n%s\nwant:\n%s", path, got, want)
		}
	}
}

func TestARequestThatIsNotDescribedNamesLightAndTheLanguagesAlone(t *testing.T) {
	h := hearing(t)
	c := fetch.New(nil, fetch.Agent(agent), fetch.Languages(languages))
	described := fetch.Asking(t.Context(), identity.Request{Destination: identity.Document, User: true})
	if _, err := c.Get(fetch.By(described, parsed(t, h.here+"/")), h.here+"/c", "text/plain"); err != nil {
		t.Fatal(err)
	}
	want := "Accept: text/plain\nAccept-Language: " + languages + "\nUser-Agent: " + agent
	if got := h.said("GET /c", "Accept", "Accept-Language", "User-Agent", "Referer", "Sec-Fetch-Dest", "Sec-Fetch-Mode",
		"Sec-Fetch-Site", "Sec-Fetch-User", "Upgrade-Insecure-Requests"); got != want {
		t.Errorf("it said:\n%s\nwant:\n%s", got, want)
	}
}

func TestAScriptsRequestSaysWhatItIsAndKeepsLightsName(t *testing.T) {
	h := hearing(t)
	c := fetch.New(nil, fetch.Agent(agent), fetch.Languages(languages))
	page := parsed(t, h.here+"/page")
	names := []string{"Accept", "Accept-Language", "User-Agent", "Origin", "Referer", "Sec-Fetch-Dest",
		"Sec-Fetch-Mode", "Sec-Fetch-Site", "Sec-Fetch-User", "X-Asked"}
	call := func(address string, mode fetch.Mode, header http.Header) {
		t.Helper()
		ctx := fetch.Asking(fetch.By(t.Context(), page), identity.Request{Destination: identity.Empty})
		if _, err := c.Script(ctx, fetch.Call{Method: http.MethodGet, Address: address, Header: header, Page: page,
			Mode: mode}); err != nil {
			t.Fatal(err)
		}
	}
	call(h.here+"/c", "", http.Header{"Accept": {"application/json"}, "Accept-Language": {"de"},
		"User-Agent": {"a script's"}, "Sec-Fetch-Site": {"none"}})
	want := "Accept: application/json\nAccept-Language: de\nUser-Agent: " + agent + "\nReferer: " + h.here +
		"/page\nSec-Fetch-Dest: empty\nSec-Fetch-Mode: cors\nSec-Fetch-Site: same-origin"
	if got := h.said("GET /c", names...); got != want {
		t.Errorf("a fetch of the page's origin said:\n%s\nwant:\n%s", got, want)
	}
	call(h.here+"/c", fetch.ModeSameOrigin, nil)
	want = "Accept: */*\nAccept-Language: " + languages + "\nUser-Agent: " + agent + "\nReferer: " + h.here +
		"/page\nSec-Fetch-Dest: empty\nSec-Fetch-Mode: same-origin\nSec-Fetch-Site: same-origin"
	if got := h.said("GET /c", names...); got != want {
		t.Errorf("a same-origin fetch said:\n%s\nwant:\n%s", got, want)
	}
	call(h.beside+"/b", "", http.Header{"X-Asked": {"1"}})
	cross := "Accept: */*\nAccept-Language: " + languages + "\nUser-Agent: " + agent + "\nOrigin: " + h.here +
		"\nReferer: " + h.here + "/\nSec-Fetch-Dest: empty\nSec-Fetch-Mode: cors\nSec-Fetch-Site: cross-site"
	if got := h.said("OPTIONS /b", names...); got != cross {
		t.Errorf("the preflight said:\n%s\nwant:\n%s", got, cross)
	}
	if got := h.said("GET /b", names...); got != cross+"\nX-Asked: 1" {
		t.Errorf("a fetch of another origin said:\n%s\nwant:\n%s", got, cross+"\nX-Asked: 1")
	}
	back := h.said("GET /c", "Origin", "Referer", "Sec-Fetch-Site")
	if !strings.HasSuffix(back, "Referer: "+h.here+"/page\nSec-Fetch-Site: cross-site") {
		t.Errorf("redirected back to the page's origin, it said:\n%s", back)
	}
}
