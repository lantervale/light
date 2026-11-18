package fetch_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/lantervale/light/pkg/fetch"
)

// took is what a server was asked, each request a line: its method, its
// path, its type, its origin, its referrer and its body.
type took struct {
	sync.Mutex
	lines []string
}

func (k *took) note(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	k.Lock()
	defer k.Unlock()
	k.lines = append(k.lines, fmt.Sprintf("%s %s type=%q origin=%q referer=%q body=%q", r.Method, r.URL.Path,
		r.Header.Get("Content-Type"), r.Header.Get("Origin"), r.Header.Get("Referer"), body))
}

// saving is a site that takes a form at /save and at /save/<status>,
// which sends it on to /landing with that status, and at /away/<status>,
// which sends it on to another site's /landing.
func saving(t *testing.T) (*httptest.Server, *took) {
	t.Helper()
	asked := &took{}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.note(r)
		_, _ = io.WriteString(w, "elsewhere")
	}))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.note(r)
		var status int
		switch {
		case scan(r.URL.Path, "/save/%d", &status):
			w.Header().Set("Location", "/landing")
			w.WriteHeader(status)
		case scan(r.URL.Path, "/away/%d", &status):
			w.Header().Set("Location", other.URL+"/landing")
			w.WriteHeader(status)
		default:
			_, _ = io.WriteString(w, "saved")
		}
	}))
	t.Cleanup(srv.Close)
	return srv, asked
}

func scan(path, format string, status *int) bool {
	n, err := fmt.Sscanf(path, format, status)
	return err == nil && n == 1
}

const urlencoded = "application/x-www-form-urlencoded"

func TestAPostCarriesItsFormItsOriginAndItsReferrer(t *testing.T) {
	srv, asked := saving(t)
	ctx := fetch.Referring(context.Background(), srv.URL+"/page")
	form := fetch.Form{Type: urlencoded, Body: []byte("a=1&b=two+words"), Origin: srv.URL}
	answer, err := fetch.New(nil, fetch.Agent("light-test")).Post(ctx, srv.URL+"/save", "text/html", form)
	if err != nil {
		t.Fatal(err)
	}
	got, err := answer.Answer()
	if err != nil || string(got.Body) != "saved" || got.Status != http.StatusOK {
		t.Fatalf("the answer is %v, %v", got, err)
	}
	want := fmt.Sprintf("POST /save type=%q origin=%q referer=%q body=%q", urlencoded, srv.URL, srv.URL+"/page",
		"a=1&b=two+words")
	if len(asked.lines) != 1 || asked.lines[0] != want {
		t.Fatalf("the server was asked\n%s\nwant\n%s", strings.Join(asked.lines, "\n"), want)
	}
}

func TestARedirectedPostIsAGetOrAPostByItsStatus(t *testing.T) {
	for _, c := range []struct {
		path string
		// kept is whether the form goes on to where the redirect leads.
		kept bool
		// origin is the Origin a kept form goes on with, the site's own when "".
		origin string
	}{
		{"/save/301", false, ""}, {"/save/302", false, ""}, {"/save/303", false, ""},
		{"/save/307", true, ""}, {"/save/308", true, ""},
		{"/away/303", false, ""}, {"/away/307", true, "null"}, {"/away/308", true, "null"},
	} {
		t.Run(c.path, func(t *testing.T) {
			srv, asked := saving(t)
			ctx := fetch.Referring(context.Background(), srv.URL+"/page")
			form := fetch.Form{Type: urlencoded, Body: []byte("a=1"), Origin: srv.URL}
			stream, err := fetch.New(nil).Post(ctx, srv.URL+c.path, "text/html", form)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			if stream.Hops != 1 || !strings.HasSuffix(stream.Address, "/landing") {
				t.Fatalf("it ended at %s after %d redirects", stream.Address, stream.Hops)
			}
			first := fmt.Sprintf("POST %s type=%q origin=%q referer=%q body=%q", c.path, urlencoded, srv.URL,
				srv.URL+"/page", "a=1")
			referrer := srv.URL + "/page"
			if strings.HasPrefix(c.path, "/away") {
				referrer = srv.URL + "/"
			}
			then := fmt.Sprintf("GET /landing type=%q origin=%q referer=%q body=%q", "", "", referrer, "")
			if c.kept {
				origin := c.origin
				if origin == "" {
					origin = srv.URL
				}
				then = fmt.Sprintf("POST /landing type=%q origin=%q referer=%q body=%q", urlencoded, origin, referrer,
					"a=1")
			}
			if want := []string{first, then}; strings.Join(asked.lines, "\n") != strings.Join(want, "\n") {
				t.Fatalf("the servers were asked\n%s\nwant\n%s", strings.Join(asked.lines, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}
