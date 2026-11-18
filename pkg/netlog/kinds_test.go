package netlog_test

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/netlog"
)

func TestWhatAScriptAskedForIsXHRWhateverIsAnswered(t *testing.T) {
	for _, media := range []string{"", "application/json", "text/html", "image/png", "text/javascript"} {
		if got := netlog.TypeOf(media, netlog.XHR); got != netlog.XHR {
			t.Errorf("a script's request answered %q is %s", media, got)
		}
	}
	srv := site(t)
	l := netlog.New(10)
	get(netlog.With(t.Context(), l, "app.js:12", netlog.XHR), t, srv.URL+"/")
	get(netlog.With(t.Context(), l, "app.js:13", netlog.XHR), t, srv.URL+"/gone")
	rows := l.Rows()
	if rows[0].Type != netlog.XHR || rows[1].Type != netlog.XHR || rows[0].Initiator != "app.js:12" {
		t.Fatalf("the rows are %s by %s and %s", rows[0].Type, rows[0].Initiator, rows[1].Type)
	}
	if rows[0].Kind() != "html" {
		t.Errorf("the first is of kind %q", rows[0].Kind())
	}
}

func TestARowSaysWhatItIsInAWord(t *testing.T) {
	for _, c := range []struct{ media, address, want string }{
		{"text/html", "https://s.test/", "html"}, {"text/css", "https://s.test/a", "css"},
		{"application/javascript", "https://s.test/a", "js"}, {"application/json", "https://s.test/a", "json"},
		{"image/svg+xml", "https://s.test/a", "svg"}, {"image/jpeg", "https://s.test/a", "jpeg"},
		{"font/woff2", "https://s.test/a", "woff2"}, {"text/plain", "https://s.test/a", "plain"},
		{"application/x-font-ttf", "https://s.test/a", "ttf"}, {"image/vnd.microsoft.icon", "https://s.test/a", "icon"},
		{"", "https://s.test/app.JS?v=2", "js"}, {"", "https://s.test/", ""},
		{"application/octet-stream", "https://s.test/a.bin", "octet-stream"},
	} {
		if got := (netlog.Row{Media: c.media, Address: c.address}).Kind(); got != c.want {
			t.Errorf("%q at %s is %q, want %q", c.media, c.address, got, c.want)
		}
	}
}

func TestARowNamesItsSiteAndWhetherItIsSecure(t *testing.T) {
	for address, want := range map[string]string{
		"https://s.test/a/b": "s.test|true", "http://s.test:8080/": "s.test:8080|false", "::bad": "|false",
	} {
		r := netlog.Row{Address: address}
		got := r.Host() + "|"
		if r.Secure() {
			got += "true"
		} else {
			got += "false"
		}
		if got != want {
			t.Errorf("%s is of %s, want %s", address, got, want)
		}
	}
}

func TestARequestRefusedIsABlockedRow(t *testing.T) {
	l := netlog.New(10)
	before := l.Version()
	l.Block("POST", "https://s.test/collect", "app.js:3", netlog.XHR, "light sends no POST")
	rows := l.Rows()
	if len(rows) != 1 || l.Version() == before {
		t.Fatalf("%d rows", len(rows))
	}
	r := rows[0]
	if !r.Blocked || !r.Failed() || !r.Done || r.Method != http.MethodPost || r.Type != netlog.XHR ||
		r.Initiator != "app.js:3" || r.Err != "light sends no POST" || r.ID != 1 || r.Started.IsZero() {
		t.Fatalf("the row is %+v", r)
	}
}

func TestAGzippedAnswerIsCountedAsSentAndAsRead(t *testing.T) {
	text := strings.Repeat("a { color: red }\n", 500)
	var encoding string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoding = r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Type", "text/css")
		if r.URL.Path == "/plain.css" {
			_, _ = io.WriteString(w, text)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		zip := gzip.NewWriter(w)
		_, _ = io.WriteString(zip, text)
		_ = zip.Close()
	}))
	t.Cleanup(srv.Close)
	l := netlog.New(10)
	for _, path := range []string{"/site.css", "/plain.css"} {
		req, err := http.NewRequestWithContext(netlog.With(t.Context(), l, "document", netlog.Stylesheet),
			http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := (&http.Client{Transport: &netlog.Transport{}}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || string(got) != text || resp.Header.Get("Content-Encoding") != "" {
			t.Fatalf("%s is read as %d bytes (%v), encoded %q", path, len(got), err, resp.Header.Get("Content-Encoding"))
		}
	}
	if encoding != netlog.Encodings {
		t.Errorf("the server was asked for %q", encoding)
	}
	rows := l.Rows()
	if zipped := rows[0]; zipped.Size != int64(len(text)) || zipped.Transferred <= 0 ||
		zipped.Transferred >= zipped.Size/4 || string(zipped.Preview) != text {
		t.Errorf("the gzipped answer: %d bytes read, %d sent", zipped.Size, zipped.Transferred)
	}
	if plain := rows[1]; plain.Size != int64(len(text)) || plain.Transferred != plain.Size {
		t.Errorf("the plain answer: %d bytes read, %d sent", plain.Size, plain.Transferred)
	}
	if total := netlog.Totals(rows); total.Bytes != 2*int64(len(text)) ||
		total.Transferred != rows[0].Transferred+rows[1].Transferred {
		t.Errorf("totals %+v", total)
	}
}

func TestAnEncodingTheRequestAskedForIsLeftToIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", r.Header.Get("Accept-Encoding"))
		_, _ = io.WriteString(w, "raw")
	}))
	t.Cleanup(srv.Close)
	l := netlog.New(10)
	req, _ := http.NewRequestWithContext(netlog.With(t.Context(), l, "document", netlog.Other),
		http.MethodGet, srv.URL, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := (&http.Client{Transport: &netlog.Transport{}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(got) != "raw" || resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("the body is %q, encoded %q", got, resp.Header.Get("Content-Encoding"))
	}
}

func TestAPreflightIsARowOfItsOwnKind(t *testing.T) {
	srv := site(t)
	l := netlog.New(10)
	ctx, trail := netlog.Trailed(netlog.With(t.Context(), l, "app.js:3", netlog.XHR))
	if trail.Last() != 0 {
		t.Errorf("a trail with no request ends at row %d", trail.Last())
	}
	for _, method := range []string{http.MethodOptions, http.MethodPut, http.MethodOptions} {
		req, err := http.NewRequestWithContext(ctx, method, srv.URL+"/", http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		if method == http.MethodOptions && trail.Last() == 0 {
			req.Header.Set("Access-Control-Request-Method", http.MethodPut)
		}
		resp, err := (&http.Client{Transport: &netlog.Transport{}}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	rows := l.Rows()
	if rows[0].Type != netlog.Preflight || rows[0].Kind() != "preflight" || rows[0].Initiator != "app.js:3" ||
		rows[1].Type != netlog.XHR || rows[2].Type != netlog.XHR || rows[2].Kind() == "preflight" {
		t.Errorf("the rows are %s (%s), %s and %s", rows[0].Type, rows[0].Kind(), rows[1].Type, rows[2].Type)
	}
	if got := netlog.TypeOf("text/html", netlog.Preflight); got != netlog.Preflight {
		t.Errorf("a preflight answered a page is %s", got)
	}
	if kept := netlog.Filter(rows, "", netlog.XHR); len(kept) != 3 {
		t.Errorf("XHR keeps %d rows of a preflight and two requests", len(kept))
	}
	if kept := netlog.Filter(rows, "", netlog.Preflight); len(kept) != 1 {
		t.Errorf("preflight keeps %d rows", len(kept))
	}
	if kept := netlog.Filter(rows, "", netlog.Document); len(kept) != 0 {
		t.Errorf("document keeps %d rows", len(kept))
	}
	if trail.Last() != rows[2].ID {
		t.Errorf("the trail ends at row %d, want %d", trail.Last(), rows[2].ID)
	}
	if _, other := netlog.Trailed(netlog.As(ctx, "document", netlog.Other)); other.Last() != 0 {
		t.Errorf("a new trail ends at row %d", other.Last())
	}
}

func TestAnAnswerKeptFromAScriptIsABlockedRow(t *testing.T) {
	srv := site(t)
	l := netlog.New(10)
	ctx, trail := netlog.Trailed(netlog.With(t.Context(), l, "app.js:3", netlog.XHR))
	get(ctx, t, srv.URL+"/")
	get(netlog.As(ctx, "app.js:4", netlog.XHR), t, srv.URL+"/gone")
	before := l.Version()
	l.Refuse(trail.Last(), "No 'Access-Control-Allow-Origin' header is present on the requested resource.")
	l.Refuse(99, "no such row")
	rows := l.Rows()
	if rows[0].Blocked || rows[0].Err != "" || l.Version() != before+1 {
		t.Errorf("the first row is %+v", rows[0])
	}
	if kept := rows[1]; !kept.Blocked || !kept.Failed() || kept.Status != http.StatusNotFound || kept.Request == nil ||
		!strings.HasPrefix(kept.Err, "No 'Access-Control-Allow-Origin'") || kept.Initiator != "app.js:4" {
		t.Errorf("the row of the answer that was kept is %+v", kept)
	}
}
