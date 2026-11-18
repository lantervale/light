package fetch_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/fetch"
)

func site(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, "%s %s %s", r.Method, r.Header.Get("User-Agent"), r.Header.Get("Accept"))
	})
	for code, path := range map[int]string{301: "/moved", 302: "/found", 303: "/other", 307: "/kept",
		308: "/for-good"} {
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", "pages/landing?from="+path[1:])
			w.WriteHeader(code)
		})
	}
	mux.HandleFunc("/pages/landing", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "landed from "+r.URL.Query().Get("from"))
	})
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop",
			http.StatusFound)
	})
	mux.HandleFunc("/hop", func(w http.ResponseWriter, r *http.Request) {
		n := len(r.URL.Query().Get("n"))
		if n >= 3 {
			_, _ = io.WriteString(w, "three hops")
			return
		}
		http.Redirect(w, r, "/hop?n="+strings.Repeat("x", n+1), http.StatusFound)
	})
	mux.HandleFunc("/away", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "ftp://files.test/x")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "<h1>Nothing here</h1>")
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "2000")
		_, _ = w.Write(make([]byte, 2000))
	})
	mux.HandleFunc("/stream", func(w http.ResponseWriter, _ *http.Request) {
		for range 20 {
			_, _ = w.Write(make([]byte, 100))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	})
	mux.HandleFunc("/latin", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=iso-8859-1")
		_, _ = w.Write([]byte{'c', 'l', 0xe9})
	})
	mux.HandleFunc("/meta", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(append([]byte(`<meta charset="windows-1252"><p>`), 0x80))
	})
	mux.HandleFunc("/slow", func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	mux.HandleFunc("/slowly-moved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/slow",
			http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGetAsksAsTheClientIsSetUpAndGivesTheAnswer(t *testing.T) {
	srv := site(t)
	c := fetch.New(nil, fetch.Agent("light/test"))
	a, err := c.Get(t.Context(), srv.URL+"/", "text/html")
	if err != nil {
		t.Fatal(err)
	}
	if string(a.Body) != "GET light/test text/html" || a.Status != 200 || a.Address != srv.URL+"/" || a.Hops != 0 {
		t.Fatalf("the answer is %q, %d, at %s after %d hops", a.Body, a.Status, a.Address, a.Hops)
	}
	if a.Header.Get("Content-Type") != "text/html" {
		t.Errorf("headers %v", a.Header)
	}
}

func TestEveryRedirectCodeIsFollowedFromTheHopsAddress(t *testing.T) {
	srv := site(t)
	c := fetch.New(nil)
	for _, path := range []string{"moved", "found", "other", "kept", "for-good"} {
		a, err := c.Get(t.Context(), srv.URL+"/"+path, "*/*")
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if want := srv.URL + "/pages/landing?from=" + path; a.Address != want || a.Hops != 1 ||
			string(a.Body) != "landed from "+path {
			t.Errorf("%s landed at %s after %d hops with %q", path, a.Address, a.Hops, a.Body)
		}
	}
}

func TestARedirectLoopOrOneTooManyIsAnError(t *testing.T) {
	srv := site(t)
	c := fetch.New(nil)
	if _, err := c.Get(t.Context(), srv.URL+"/loop", "*/*"); !errors.Is(err, fetch.ErrTooManyRedirects) {
		t.Errorf("a loop gives %v", err)
	}
	a, err := c.Get(t.Context(), srv.URL+"/hop", "*/*")
	if err != nil || a.Hops != 3 {
		t.Fatalf("three hops give %v, %v", a, err)
	}
	_, err = c.Get(fetch.After(t.Context(), fetch.MaxRedirects-2), srv.URL+"/hop", "*/*")
	if !errors.Is(err, fetch.ErrTooManyRedirects) {
		t.Errorf("three hops after %d give %v", fetch.MaxRedirects-2, err)
	}
}

func TestARedirectToAnotherSchemeIsRefused(t *testing.T) {
	srv := site(t)
	_, err := fetch.New(nil).Get(t.Context(), srv.URL+"/away", "*/*")
	if err == nil || !strings.Contains(err.Error(), "ftp://files.test/x") || !strings.Contains(err.Error(), "http") {
		t.Fatalf("err %v, want one naming the address and what is opened", err)
	}
}

func TestAnErrorStatusIsAnAnswerWithItsBody(t *testing.T) {
	srv := site(t)
	a, err := fetch.New(nil).Get(t.Context(), srv.URL+"/gone", "*/*")
	if err != nil || a.Status != 404 || string(a.Body) != "<h1>Nothing here</h1>" || a.OK() {
		t.Fatalf("a 404 gives %v, %v", a, err)
	}
}

func TestAnAnswerOverTheLimitIsRefusedSaidOrNot(t *testing.T) {
	srv := site(t)
	c := fetch.New(nil, fetch.Limit(1000))
	for _, path := range []string{"/big", "/stream"} {
		_, err := c.Get(t.Context(), srv.URL+path, "*/*")
		var big *fetch.TooBigError
		if !errors.As(err, &big) || big.Limit != 1000 {
			t.Errorf("%s gives %v", path, err)
		}
	}
	if a, err := fetch.New(nil, fetch.Limit(2000)).Get(t.Context(), srv.URL+"/big", "*/*"); err != nil ||
		len(a.Body) != 2000 {
		t.Errorf("an answer of the limit's size gives %v", err)
	}
}

func TestTextIsDecodedByItsCharset(t *testing.T) {
	srv := site(t)
	c := fetch.New(nil)
	for path, want := range map[string]string{"/latin": "clé", "/meta": `<meta charset="windows-1252"><p>€`} {
		a, err := c.Get(t.Context(), srv.URL+path, "*/*")
		if err != nil {
			t.Fatal(err)
		}
		if got := a.Text(); got != want {
			t.Errorf("%s reads %q, want %q", path, got, want)
		}
	}
}

func TestCodeIsUTF8UnlessItsTypeNamesACharset(t *testing.T) {
	late := strings.Repeat(" ", 2048) + "var r = {À: 'A'}"
	for name, c := range map[string]struct {
		header, body, want string
	}{
		"no charset, the first bytes plain": {"text/javascript", late, late},
		"a charset":                         {"text/javascript; charset=iso-8859-1", "var \xe9 = 1", "var é = 1"},
		"a byte order mark":                 {"text/css", "\xef\xbb\xbfa { content: 'é' }", "a { content: 'é' }"},
		"a charset no one knows":            {"text/css; charset=klingon", "a { content: 'é' }", "a { content: 'é' }"},
	} {
		a := &fetch.Answer{Header: http.Header{"Content-Type": {c.header}}, Body: []byte(c.body)}
		if got := a.Code(); got != c.want {
			t.Errorf("%s reads %q, want %q", name, got, c.want)
		}
	}
	if a := (&fetch.Answer{Header: http.Header{"Content-Type": {"text/html"}}, Body: []byte(late)}); a.Text() == late {
		t.Error("a page with no charset and plain first bytes is no longer sniffed")
	}
}

func TestAStoppedOrSlowRequestEndsAlsoBehindARedirect(t *testing.T) {
	srv := site(t)
	c := fetch.New(nil)
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(50*time.Millisecond, cancel)
	if _, err := c.Get(ctx, srv.URL+"/slowly-moved", "*/*"); !errors.Is(err, context.Canceled) {
		t.Errorf("stopped, it gives %v", err)
	}
	ctx, cancel = context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Get(ctx, srv.URL+"/slow", "*/*"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("too slow, it gives %v", err)
	}
}

func TestARefusedConnectionAndABadAddressAreErrors(t *testing.T) {
	srv := site(t)
	srv.Close()
	c := fetch.New(nil)
	if _, err := c.Get(t.Context(), srv.URL+"/", "*/*"); err == nil || strings.Contains(err.Error(), `Get "`) {
		t.Errorf("a closed server gives %v, want the reason alone", err)
	}
	if _, err := c.Get(t.Context(), "http://bad host/", "*/*"); err == nil {
		t.Error("a bad address gives an answer")
	}
}

func TestARefreshIsReadAsBrowsersReadIt(t *testing.T) {
	base, _ := url.Parse("https://s.test/a/page.html")
	for value, want := range map[string]string{
		"0; url=/next":            "0 https://s.test/next",
		"0;URL='next.html'":       "0 https://s.test/a/next.html",
		`5; url="https://o.test"`: "5 https://o.test",
		"3":                       "3 https://s.test/a/page.html",
		" 0 ; https://o.test/x ":  "0 https://o.test/x",
		"0, url=next":             "0 https://s.test/a/next",
		"soon; url=/x":            "",
		"":                        "",
		"-1; url=/x":              "",
		"0; url=ftp://o.test":     "0 ftp://o.test",
	} {
		got := ""
		if after, to, ok := fetch.Refresh(value, base); ok {
			got = fmt.Sprint(after, " ", to)
		}
		if got != want {
			t.Errorf("%q reads %q, want %q", value, got, want)
		}
	}
}

func TestAnOpenedAnswerIsReadAsItComesAndItsStartSeenWithoutTakingIt(t *testing.T) {
	srv := site(t)
	c := fetch.New(nil, fetch.Limit(100))
	s, err := c.Open(t.Context(), srv.URL+"/moved", "*/*")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Address != srv.URL+"/pages/landing?from=moved" || s.Hops != 1 || !s.OK() || s.Length != 17 {
		t.Fatalf("opened %s after %d hops, status %d, %d bytes", s.Address, s.Hops, s.Status, s.Length)
	}
	if start := s.Peek(6); string(start) != "landed" {
		t.Fatalf("the start is %q", start)
	}
	if all := s.Peek(512); string(all) != "landed from moved" {
		t.Fatalf("a peek past the end gives %q", all)
	}
	body, err := io.ReadAll(s)
	if err != nil || string(body) != "landed from moved" {
		t.Fatalf("read %q, %v", body, err)
	}
}

func TestAnOpenedAnswerPastTheClientsLimitIsStillStreamed(t *testing.T) {
	srv := site(t)
	s, err := fetch.New(nil, fetch.Limit(100)).Open(t.Context(), srv.URL+"/big", "*/*")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	n, err := io.Copy(io.Discard, s)
	if err != nil || n != 2000 || s.Length != 2000 {
		t.Fatalf("streamed %d of %d bytes: %v", n, s.Length, err)
	}
	held, err := fetch.New(nil, fetch.Limit(100)).Open(t.Context(), srv.URL+"/big", "*/*")
	if err != nil {
		t.Fatal(err)
	}
	var big *fetch.TooBigError
	if _, err = held.Answer(); !errors.As(err, &big) {
		t.Fatalf("holding it whole gives %v", err)
	}
}

func TestAClientFollowsAsManyRedirectsAsItIsSetUpTo(t *testing.T) {
	srv := site(t)
	few := fetch.New(nil, fetch.Redirects(2))
	if _, err := few.Get(t.Context(), srv.URL+"/hop", "*/*"); !errors.Is(err, fetch.ErrTooManyRedirects) ||
		few.MaxRedirects() != 2 {
		t.Errorf("three hops with two allowed give %v", err)
	}
	none := fetch.New(nil, fetch.Redirects(0))
	if _, err := none.Get(t.Context(), srv.URL+"/hop", "*/*"); !errors.Is(err, fetch.ErrTooManyRedirects) {
		t.Errorf("a redirect with none allowed gives %v", err)
	}
	if got := fetch.New(nil).MaxRedirects(); got != fetch.MaxRedirects {
		t.Errorf("a client follows %d redirects unless told", got)
	}
}
