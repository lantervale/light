package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/localnet"
)

// lan is servers of this machine taken for servers of the Internet, of
// the local network and of this machine, each by its port, and what
// each was asked.
type lan struct {
	t  *testing.T
	mu sync.Mutex
	of map[uint16]localnet.Space
	// asked are the requests each server got, as "METHOD /path".
	asked map[string][]string
	// pages is what each server shows, by its path.
	pages map[string]string
}

func newLAN(t *testing.T) *lan {
	t.Helper()
	return &lan{t: t, of: map[uint16]localnet.Space{}, asked: map[string][]string{}, pages: map[string]string{}}
}

func (l *lan) space(at netip.AddrPort) localnet.Space {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.of[at.Port()]
}

// serve is the address of a new server of a space: it shows its pages,
// redirects where a "to" says, and answers the rest "ok" to whoever
// asks.
func (l *lan) serve(space localnet.Space) string {
	l.t.Helper()
	var at string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		l.asked[at] = append(l.asked[at], r.Method+" "+r.URL.Path)
		page, shown := l.pages[at+r.URL.Path]
		l.mu.Unlock()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		switch to := r.URL.Query().Get("to"); {
		case to != "":
			http.Redirect(w, r, to, http.StatusFound)
		case shown:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, page)
		case strings.HasSuffix(r.URL.Path, ".js"):
			w.Header().Set("Content-Type", "text/javascript")
			_, _ = io.WriteString(w, "window.ran = (window.ran || 0) + 1; export default 1;")
		default:
			_, _ = io.WriteString(w, "ok")
		}
	}))
	l.t.Cleanup(srv.Close)
	at = srv.URL
	l.mu.Lock()
	defer l.mu.Unlock()
	l.of[netip.MustParseAddrPort(srv.Listener.Addr().String()).Port()] = space
	return at
}

func (l *lan) show(address, markup string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pages[address] = markup
}

// got is what a server was asked but its pages and its icon, sorted.
func (l *lan) got(at string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var kept []string
	for _, line := range l.asked[at] {
		if _, page := l.pages[at+strings.TrimPrefix(line, "GET ")]; !page && line != "GET /favicon.ico" {
			kept = append(kept, line)
		}
	}
	slices.Sort(kept)
	return strings.Join(kept, ", ")
}

// blocked waits for the requests for those addresses to be blocked rows
// of the shown tab, each for the reason given.
func (s *shown) blocked(why map[string]string) {
	s.t.Helper()
	s.until("the requests were blocked", func() bool {
		for address := range why {
			if s.reason(address) == "" {
				return false
			}
		}
		return true
	})
	for address, want := range why {
		if got := s.reason(address); got != want {
			s.t.Errorf("the request for %s is blocked for %q, want %q", address, got, want)
		}
	}
}

// reason is why the request for address is a blocked row of the shown
// tab, "" for one that is not.
func (s *shown) reason(address string) string {
	s.t.Helper()
	for _, r := range s.session().log.Rows() {
		if r.Address == address && r.Blocked {
			return r.Err
		}
	}
	return ""
}

const (
	toLocal = "The request client is in the `public` address space and the resource is in more-private address space " +
		"`local`."
	toLoopback = "The request client is in the `public` address space and the resource is in more-private address space " +
		"`loopback`."
)

func TestAPageOfTheInternetLoadsNothingOfTheLocalNetwork(t *testing.T) {
	l := newLAN(t)
	site, other, router, here := l.serve(localnet.Public), l.serve(localnet.Public), l.serve(localnet.Private),
		l.serve(localnet.Loopback)
	parts := func(of ...string) string {
		var markup strings.Builder
		for _, at := range of {
			markup.WriteString(`<img src="` + at + `/pic.png"><link rel="stylesheet" href="` + at + `/look.css">`)
		}
		return markup.String()
	}
	l.show(site+"/", `<title>Public</title>`+parts(other, router, here)+
		`<img src="`+other+`/turned.png?to=`+router+`/reached.png"><img src="//localhost:`+portOf(here)+`/named.png">`)
	l.show(router+"/", `<title>Router</title>`+parts(other, router, here))
	l.show(here+"/", `<title>Here</title>`+parts(other, router, here))
	s := showWith(t, Options{Downloads: t.TempDir(), Space: l.space})

	const all = "GET /look.css, GET /pic.png"
	s.visit(site + "/")
	s.blocked(map[string]string{
		router + "/pic.png": toLocal, router + "/look.css": toLocal, router + "/reached.png": toLocal,
		here + "/pic.png": toLoopback, here + "/look.css": toLoopback,
		"http://localhost:" + portOf(here) + "/named.png": toLoopback,
	})
	if got := l.got(router) + l.got(here); got != "" {
		t.Errorf("a page of the Internet asked the local network for %s", got)
	}
	if got := l.got(other); got != all+", GET /turned.png" {
		t.Errorf("the Internet was asked for %s", got)
	}
	if page := s.session().pages[site+"/"]; page.Space != localnet.Public {
		t.Errorf("the page came from the %s network", page.Space)
	}

	if doc := s.visit(router + "/"); doc == nil || s.session().pages[router+"/"].Space != localnet.Private {
		t.Fatalf("the user's own way to the local network gave %+v", s.session().pages[router+"/"])
	}
	const fromLocal = "The request client is in the `local` address space and the resource is in more-private " +
		"address space `loopback`."
	s.blocked(map[string]string{here + "/pic.png": fromLocal, here + "/look.css": fromLocal})
	s.until("the local network was asked by a page of its own", func() bool { return l.got(router) == all })
	if got := l.got(here); got != "" {
		t.Errorf("a page of the local network asked this machine for %s", got)
	}

	s.visit(here + "/")
	s.until("this machine and the local network were asked by a page of this machine", func() bool {
		return l.got(here) == all && l.got(router) == "GET /look.css, GET /look.css, GET /pic.png, GET /pic.png"
	})
}

func portOf(address string) string { return address[strings.LastIndex(address, ":")+1:] }

func TestARefreshTakesAPageOfTheInternetNoFurtherThanItReaches(t *testing.T) {
	l := newLAN(t)
	site, router := l.serve(localnet.Public), l.serve(localnet.Private)
	l.show(site+"/", `<meta http-equiv="refresh" content="0; url=`+router+`/reboot">`)
	l.show(site+"/on", `<meta http-equiv="refresh" content="0; url=`+site+`/there">`)
	l.show(site+"/there", `<title>There</title>`)
	l.show(router+"/reboot", `<title>Rebooting</title>`)
	s := showWith(t, Options{Downloads: t.TempDir(), Space: l.space})
	s.visit(site + "/")
	var refused *localnet.BlockedError
	if page := s.session().pages[site+"/"]; !errors.As(page.Err, &refused) || refused.To != localnet.Private {
		t.Errorf("the page that refreshes to the local network is %+v", page)
	}
	if got := l.asked[router]; len(got) != 0 {
		t.Errorf("the local network was asked for %s", got)
	}
	if got := s.reason(router + "/reboot"); got != toLocal {
		t.Errorf("the refresh is blocked for %q", got)
	}
	s.Open(site + "/on")
	s.until("the refresh within the Internet was followed", func() bool { return s.session().pages[site+"/there"] != nil })
}

func TestTheUserGoesWhereAPageMayNotAndASettingLetsPagesToo(t *testing.T) {
	l := newLAN(t)
	site, router := l.serve(localnet.Public), l.serve(localnet.Private)
	l.show(site+"/", `<title>Public</title><a id="in" href="`+router+`/admin">in</a><img src="`+router+`/pic.png">`)
	l.show(site+"/turn", `<title>Turn</title>`)
	l.show(router+"/admin", `<title>Admin</title>`)
	s := showWith(t, Options{Downloads: t.TempDir(), Space: l.space})
	s.visit(site + "/")
	s.clickAt(s.inPage("in"))
	s.until("the link the user clicked was followed", func() bool { return s.session().pages[router+"/admin"] != nil })
	if page := s.session().pages[router+"/admin"]; page.Err != nil || page.Title != "Admin" {
		t.Errorf("the link gave %+v", page)
	}
	s.visit(site + "/turn?to=" + router + "/admin")
	if got := l.got(router); got != "" {
		t.Errorf("the local network was asked for %s by a page", got)
	}

	local := s.loader.Load().Local("light://test", `<link rel="stylesheet" href="`+router+`/own.css">`, web.Links{})
	if local.Space != localnet.Loopback || l.got(router) != "GET /own.css" {
		t.Errorf("a page of light's, of the %s network, asked the local one for %q", local.Space, l.got(router))
	}

	s.configure(func(c *config.Config) { c.LocalNetwork = true })
	s.session().log.Clear()
	l.show(site+"/again", `<title>Public</title><img src="`+router+`/pic.png">`)
	s.visit(site + "/again")
	s.until("the local network was asked by a page let into it", func() bool {
		return l.got(router) == "GET /own.css, GET /pic.png"
	})
	for _, r := range s.session().log.Rows() {
		if r.Blocked {
			t.Errorf("with pages let into the local network, %s is blocked: %s", r.Address, r.Err)
		}
	}
}
