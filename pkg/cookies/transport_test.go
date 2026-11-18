package cookies_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/lantervale/light/pkg/cookies"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/netlog"
)

// network is two servers, one over TLS, that answer for every name:
// http://any.name/ is the first, https://example.com/ the second.
type network struct {
	t      *testing.T
	client *fetch.Client
	jar    *cookies.Jar
	log    *netlog.Log

	mu sync.Mutex
	// got is the Cookie header each request came with, by its address.
	got map[string]string
}

func serve(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *network {
	t.Helper()
	n := &network{t: t, jar: cookies.New(), log: netlog.New(100), got: map[string]string{}}
	note := func(scheme string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n.mu.Lock()
			n.got[scheme+"://"+r.Host+r.URL.RequestURI()] = r.Header.Get("Cookie")
			n.mu.Unlock()
			handler(w, r)
		})
	}
	plain, secure := httptest.NewServer(note("http")), httptest.NewTLSServer(note("https"))
	t.Cleanup(plain.Close)
	t.Cleanup(secure.Close)
	transport, ok := secure.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("the server's client goes through a %T", secure.Client().Transport)
	}
	base := transport.Clone()
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		to := plain.Listener.Addr().String()
		if _, port, _ := net.SplitHostPort(address); port == "443" {
			to = secure.Listener.Addr().String()
		}
		return (&net.Dialer{}).DialContext(ctx, network, to)
	}
	t.Cleanup(base.CloseIdleConnections)
	n.client = fetch.New(&cookies.Transport{Jar: n.jar, Base: &netlog.Transport{Base: base}})
	return n
}

func (n *network) get(ctx context.Context, address string) *fetch.Answer {
	n.t.Helper()
	answer, err := n.client.Get(netlog.With(ctx, n.log, "test", netlog.Document), address, "*/*")
	if err != nil {
		n.t.Fatal(err)
	}
	return answer
}

// sent is how many cookies the request to address came with, -1 for a
// request that never came.
func (n *network) sent(address string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	header, came := n.got[address]
	if !came {
		return -1
	}
	return count(header)
}

func set(lines ...string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/set" {
			for _, line := range lines {
				w.Header().Add("Set-Cookie", line)
			}
		}
	}
}

func TestACookieSetByARedirectIsSentToTheNextHop(t *testing.T) {
	n := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch _, err := r.Cookie("pass"); {
		case r.URL.Path == "/enter":
			http.SetCookie(w, &http.Cookie{Name: "pass", Value: "1", Path: "/", HttpOnly: true})
			http.Redirect(w, r, "/check", http.StatusFound)
		case r.URL.Path == "/check" && err != nil:
			http.Redirect(w, r, "/enter", http.StatusFound)
		case r.URL.Path == "/check":
			http.Redirect(w, r, "/inside", http.StatusSeeOther)
		case err != nil:
			w.WriteHeader(http.StatusForbidden)
		}
	})
	answer := n.get(t.Context(), "http://site.test/check")
	if answer.Status != http.StatusOK || answer.URL.Path != "/inside" || answer.Hops != 3 {
		t.Fatalf("the chain ended at %s with %d after %d hops", answer.URL.Path, answer.Status, answer.Hops)
	}
	var listed int
	for _, row := range n.log.Rows() {
		if row.Request.Get("Cookie") != "" {
			listed++
		}
		if row.Address == "http://site.test/enter" && len(row.Response.Values("Set-Cookie")) != 1 {
			t.Error("the hop that set the cookie does not list it among its response headers")
		}
	}
	if listed != 2 {
		t.Errorf("%d rows of the log list the Cookie header they were sent with, want the 2 after it was set", listed)
	}
}

func TestSecureCookiesOverTLS(t *testing.T) {
	n := serve(t, set("locked=1; Secure; Path=/", "open=1; Path=/", "__Host-own=1; Secure; Path=/"))
	n.get(t.Context(), "http://example.com/set")
	if n.jar.Len() != 1 {
		t.Fatalf("an answer over http set %d cookies, want the one that is not Secure", n.jar.Len())
	}
	n.get(t.Context(), "https://example.com/set")
	n.get(t.Context(), "https://example.com/over-tls")
	n.get(t.Context(), "http://example.com/in-clear")
	if got := n.sent("https://example.com/over-tls"); got != 3 {
		t.Errorf("a request over TLS came with %d cookies, want 3", got)
	}
	if got := n.sent("http://example.com/in-clear"); got != 1 {
		t.Errorf("a request in clear came with %d cookies, want 1", got)
	}
}

func TestThirdPartyRequests(t *testing.T) {
	n := serve(t, set("id=1; Path=/", "wide=1; Path=/; SameSite=None; Secure"))
	page := at(t, "https://www.example.com/")
	n.get(cookies.Within(t.Context(), page), "http://tracker.test/set")
	if n.jar.Len() != 0 {
		t.Fatal("a third party set a cookie")
	}
	n.get(t.Context(), "http://tracker.test/set")
	n.get(t.Context(), "https://example.com/set")
	n.get(cookies.Within(t.Context(), page), "http://tracker.test/pixel")
	n.get(cookies.Within(t.Context(), page), "https://static.example.com/sheet.css")
	n.get(cookies.Within(t.Context(), page), "https://example.com/data.json")
	if got := n.sent("http://tracker.test/pixel"); got != 0 {
		t.Errorf("a request to a third party came with %d cookies", got)
	}
	if got := n.sent("https://static.example.com/sheet.css"); got != 0 {
		t.Errorf("a host-only cookie went to another host of the site: %d cookies", got)
	}
	if got := n.sent("https://example.com/data.json"); got != 2 {
		t.Errorf("a request of the page to its own site came with %d cookies, want 2", got)
	}

	n.jar.SetThirdParty(true)
	n.get(cookies.Within(t.Context(), at(t, "https://other.test/")), "https://example.com/embedded")
	if got := n.sent("https://example.com/embedded"); got != 1 {
		t.Errorf("with third parties allowed the request came with %d cookies, want the SameSite=None one", got)
	}
}

func TestARedirectToAThirdPartyCarriesNoCookie(t *testing.T) {
	n := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			w.Header().Add("Set-Cookie", "id=1; Path=/")
		case "/out":
			http.Redirect(w, r, "http://tracker.test/in", http.StatusFound)
		}
	})
	n.get(t.Context(), "http://tracker.test/set")
	n.get(t.Context(), "http://site.test/set")
	n.get(cookies.Within(t.Context(), at(t, "http://site.test/")), "http://site.test/out")
	if got := n.sent("http://site.test/out"); got != 1 {
		t.Errorf("the page's own request came with %d cookies, want 1", got)
	}
	if got := n.sent("http://tracker.test/in"); got != 0 {
		t.Errorf("the hop to a third party came with %d cookies", got)
	}
}

func TestStrictCookiesStayHomeOnALinkFromElsewhere(t *testing.T) {
	n := serve(t, set("strict=1; Path=/; SameSite=Strict", "lax=1; Path=/"))
	n.get(t.Context(), "http://site.test/set")
	n.get(cookies.From(t.Context(), at(t, "http://other.test/")), "http://site.test/linked")
	n.get(cookies.From(t.Context(), at(t, "http://site.test/")), "http://site.test/inside")
	if got := n.sent("http://site.test/linked"); got != 1 {
		t.Errorf("a link from another site came with %d cookies, want the Lax one", got)
	}
	if got := n.sent("http://site.test/inside"); got != 2 {
		t.Errorf("a link within the site came with %d cookies, want 2", got)
	}
}

func TestRequestsShareTheJarFromManyGoroutines(t *testing.T) {
	n := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "last="+r.URL.Path[1:]+"; Path=/; Max-Age=60")
	})
	var wg sync.WaitGroup
	for _, path := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		wg.Go(func() {
			for range 10 {
				n.get(t.Context(), "http://site.test/"+path)
				n.jar.Script(at(t, "http://site.test/"))
				n.jar.Changed()
			}
		})
	}
	wg.Wait()
	if n.jar.Len() != 1 {
		t.Errorf("the jar holds %d cookies, want 1", n.jar.Len())
	}
}

func TestWithNoJarTheTransportIsItsBase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "a", Value: "1"})
		if r.Header.Get("Cookie") != "" {
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer server.Close()
	client := fetch.New(&cookies.Transport{})
	for range 2 {
		answer, err := client.Get(t.Context(), server.URL, "*/*")
		if err != nil || answer.Status != http.StatusOK {
			t.Fatalf("without a jar a request failed or came with a cookie: %v", err)
		}
	}
}

func TestAFormPostedFromAnotherSiteCarriesNoLaxCookieUntilItIsRedirected(t *testing.T) {
	n := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			w.Header().Add("Set-Cookie", "lax=1; Path=/")
			w.Header().Add("Set-Cookie", "none=1; Path=/; SameSite=None; Secure")
		case "/save":
			w.Header().Add("Set-Cookie", "saved=1; Path=/; Secure")
			http.Redirect(w, r, "/after", http.StatusSeeOther)
		}
	})
	n.get(t.Context(), "https://example.com/set")
	post := func(from string) {
		t.Helper()
		ctx := netlog.With(cookies.From(t.Context(), at(t, from)), n.log, "test", netlog.Document)
		stream, err := n.client.Post(ctx, "https://example.com/save", "*/*", fetch.Form{Type: "text/plain"})
		if err != nil {
			t.Fatal(err)
		}
		_ = stream.Close()
	}
	post("https://other.test/form")
	if save, after := n.sent("https://example.com/save"), n.sent("https://example.com/after"); save != 1 || after != 3 {
		t.Errorf("from another site the form came with %d cookies and its redirect with %d, want 1 and 3", save, after)
	}
	post("https://example.com/form")
	if save := n.sent("https://example.com/save"); save != 3 {
		t.Errorf("from its own site the form came with %d cookies, want 3", save)
	}
}

func TestARequestWithoutCookiesCarriesNoneAndKeepsNone(t *testing.T) {
	n := serve(t, set("answered=1"))
	n.get(t.Context(), "http://site.test/set")
	if n.jar.Len() != 1 {
		t.Fatalf("the jar holds %d cookies", n.jar.Len())
	}
	n.jar.Clear()
	n.jar.Store(cookies.Request{URL: at(t, "http://site.test/")}, []string{"session=1"})
	n.get(cookies.Without(cookies.Within(t.Context(), at(t, "http://site.test/"))), "http://site.test/set")
	if got := n.sent("http://site.test/set"); got != 0 {
		t.Errorf("a request without cookies came with %d", got)
	}
	if n.jar.Len() != 1 {
		t.Errorf("the jar holds %d cookies after an answer that sets one to a request without cookies", n.jar.Len())
	}
	n.get(cookies.Within(t.Context(), at(t, "http://site.test/")), "http://site.test/set")
	if got := n.sent("http://site.test/set"); got != 1 || n.jar.Len() != 2 {
		t.Errorf("a request with cookies came with %d, and the jar holds %d", got, n.jar.Len())
	}
}

func TestAScriptsPostToAnotherSiteCarriesNoLaxAndNoStrictCookie(t *testing.T) {
	n := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Add("Set-Cookie", "lax=2")
		w.Header().Add("Set-Cookie", "back=1; SameSite=None; Secure")
	})
	const api = "https://example.com/api"
	post := func(page string, credentials fetch.Credentials) {
		t.Helper()
		ctx := cookies.Within(netlog.With(t.Context(), n.log, "test", netlog.XHR), at(t, page))
		if _, err := n.client.Script(ctx, fetch.Call{Method: http.MethodPost, Address: api, Page: at(t, page),
			Credentials: credentials, Body: []byte("b")}); err != nil {
			t.Fatal(err)
		}
	}
	held := func() int { return len(n.jar.For(at(t, api))) }
	n.jar.Store(cookies.Request{URL: at(t, api)}, []string{"lax=1", "strict=1; SameSite=Strict",
		"none=1; SameSite=None; Secure"})

	post("http://site.test/", fetch.CredentialsInclude)
	if got := n.sent(api); got != 0 || held() != 3 {
		t.Errorf("with third parties blocked the POST came with %d cookies, and the jar holds %d", got, held())
	}
	n.jar.SetThirdParty(true)
	post("http://site.test/", fetch.CredentialsInclude)
	if got := n.sent(api); got != 1 || held() != 4 {
		t.Errorf("from another site the POST came with %d cookies, want the one that is SameSite=None; the jar holds %d, "+
			"want the one that is SameSite=None more", got, held())
	}
	post("http://site.test/", fetch.CredentialsSameOrigin)
	if got := n.sent(api); got != 0 || held() != 4 {
		t.Errorf("without credentials the POST came with %d cookies, and the jar holds %d", got, held())
	}
	post("https://www.example.com/", fetch.CredentialsInclude)
	if got := n.sent(api); got != 4 {
		t.Errorf("from its own site the POST came with %d cookies, want 4", got)
	}
}
