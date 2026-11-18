package netlog_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/netlog"
)

func site(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<p>hello</p>")
	})
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/older", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/older", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/",
			http.StatusFound)
	})
	mux.HandleFunc("/big.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = io.WriteString(w, strings.Repeat("a{}", netlog.PreviewCap))
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func get(ctx context.Context, t *testing.T, address string) {
	t.Helper()
	client := &http.Client{Transport: &netlog.Transport{}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/html")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func TestARequestIsARowWithWhatWasAskedAndAnswered(t *testing.T) {
	srv := site(t)
	l := netlog.New(10)
	get(netlog.With(t.Context(), l, "navigation", netlog.Other), t, srv.URL+"/")
	rows := l.Rows()
	if len(rows) != 1 {
		t.Fatalf("%d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.Address != srv.URL+"/" || r.Method != http.MethodGet || r.Status != 200 || r.Reason != "OK" || !r.Done ||
		r.Err != "" {
		t.Errorf("the row is %+v", r)
	}
	if r.Type != netlog.Document || r.Media != "text/html" || r.Initiator != "navigation" || r.Size != 12 {
		t.Errorf("type %s, media %s, asked by %s, %d bytes", r.Type, r.Media, r.Initiator, r.Size)
	}
	if string(r.Preview) != "<p>hello</p>" || r.Truncated {
		t.Errorf("the preview is %q, cut %v", r.Preview, r.Truncated)
	}
	if r.Request.Get("Accept") != "text/html" || r.Response.Get("Content-Type") == "" || r.Remote == "" {
		t.Errorf("headers %v / %v, remote %q", r.Request, r.Response, r.Remote)
	}
	if r.Took <= 0 || r.Started.IsZero() {
		t.Errorf("started %v, took %v", r.Started, r.Took)
	}
}

func TestEachRedirectHopIsARowOfOneChain(t *testing.T) {
	srv := site(t)
	l := netlog.New(10)
	get(netlog.With(t.Context(), l, "navigation", netlog.Document), t, srv.URL+"/old")
	rows := l.Rows()
	var got []string
	for _, r := range rows {
		got = append(got, fmt.Sprint(strings.TrimPrefix(r.Address, srv.URL), " ", r.Status, " ", r.Type))
	}
	if want := "[/old 301 document /older 302 document / 200 document]"; fmt.Sprint(got) != want {
		t.Fatalf("rows %v, want %s", got, want)
	}
	for _, r := range rows {
		chain := netlog.Chain(rows, r.ID)
		if len(chain) != 3 || chain[0].ID != rows[0].ID || chain[2].ID != rows[2].ID {
			t.Errorf("the chain of row %d is %d rows", r.ID, len(chain))
		}
	}
}

func TestARequestIsReTaggedInTheSameLog(t *testing.T) {
	srv := site(t)
	l := netlog.New(10)
	ctx := netlog.With(t.Context(), l, "navigation", netlog.Document)
	get(netlog.As(ctx, "document", netlog.Stylesheet), t, srv.URL+"/gone")
	get(netlog.As(t.Context(), "document", netlog.Stylesheet), t, srv.URL+"/")
	rows := l.Rows()
	if len(rows) != 1 || rows[0].Initiator != "document" {
		t.Fatalf("rows %+v", rows)
	}
}

func TestAFailureAndAnErrorStatusAreRows(t *testing.T) {
	srv := site(t)
	l := netlog.New(10)
	ctx := netlog.With(t.Context(), l, "document", netlog.Stylesheet)
	get(ctx, t, srv.URL+"/gone")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	get(cancelled, t, srv.URL+"/")
	srv.Close()
	get(ctx, t, srv.URL+"/")
	rows := l.Rows()
	if len(rows) != 3 {
		t.Fatalf("%d rows, want 3", len(rows))
	}
	if rows[0].Status != 404 || !rows[0].Failed() || rows[0].Type != netlog.Stylesheet {
		t.Errorf("a 404 is %+v", rows[0])
	}
	if rows[1].Err != "cancelled" || !rows[1].Done || rows[1].Type != netlog.Stylesheet {
		t.Errorf("a cancelled request is %+v", rows[1])
	}
	if rows[2].Err == "" || rows[2].Err == "cancelled" || !rows[2].Failed() {
		t.Errorf("a refused connection is %+v", rows[2])
	}
}

func TestALongBodyIsCountedWholeAndPreviewedUpToTheCap(t *testing.T) {
	srv := site(t)
	l := netlog.New(10)
	get(netlog.With(t.Context(), l, "document", netlog.Other), t, srv.URL+"/big.css")
	r := l.Rows()[0]
	if r.Size != 3*netlog.PreviewCap || len(r.Preview) != netlog.PreviewCap || !r.Truncated ||
		r.Type != netlog.Stylesheet {
		t.Fatalf("%d bytes, %d previewed, cut %v, type %s", r.Size, len(r.Preview), r.Truncated, r.Type)
	}
}

func TestTheOldestRowsGoPastTheCapAndAreCounted(t *testing.T) {
	srv := site(t)
	l := netlog.New(2)
	ctx := netlog.With(t.Context(), l, "document", netlog.Other)
	before := l.Version()
	for range 5 {
		get(ctx, t, srv.URL+"/")
	}
	rows := l.Rows()
	if len(rows) != 2 || l.Dropped() != 3 || rows[0].ID != 4 || rows[1].ID != 5 {
		t.Fatalf("%d rows, %d dropped, ids %v", len(rows), l.Dropped(), rows)
	}
	if l.Version() == before {
		t.Error("the version stayed while rows came")
	}
	if _, ok := l.Row(5); !ok {
		t.Error("row 5 is not found by its id")
	}
	if _, ok := l.Row(1); ok {
		t.Error("a dropped row is found")
	}
	l.SetLimit(1)
	if rows = l.Rows(); len(rows) != 1 || rows[0].ID != 5 || l.Dropped() != 4 || l.Limit() != 1 {
		t.Errorf("a smaller cap keeps %v, %d dropped", rows, l.Dropped())
	}
	l.Clear()
	if len(l.Rows()) != 0 || l.Dropped() != 0 {
		t.Error("cleared, the log still holds rows")
	}
}

func TestARequestWithNoLogIsMadeAndNotRecorded(t *testing.T) {
	srv := site(t)
	resp, err := (&http.Client{Transport: &netlog.Transport{}}).Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if body, _ := io.ReadAll(resp.Body); string(body) != "<p>hello</p>" {
		t.Fatalf("the body is %q", body)
	}
}

func TestRowsAreRecordedFromManyGoroutines(t *testing.T) {
	srv := site(t)
	l := netlog.New(100)
	ctx := netlog.With(t.Context(), l, "document", netlog.Other)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { get(ctx, t, srv.URL+"/"); _ = l.Rows() })
	}
	wg.Wait()
	if n := len(l.Rows()); n != 20 {
		t.Fatalf("%d rows, want 20", n)
	}
}

func TestATypeComesFromTheContentTypeElseFromWhatWasWanted(t *testing.T) {
	for media, want := range map[string]netlog.Type{
		"text/html": netlog.Document, "application/xhtml+xml": netlog.Document, "text/css": netlog.Stylesheet,
		"image/png": netlog.Image, "image/svg+xml": netlog.Image, "font/woff2": netlog.Font,
		"application/font-woff": netlog.Font, "application/json": netlog.Other, "": netlog.Image,
		"text/javascript": netlog.Script, "application/javascript": netlog.Script, "application/x-javascript": netlog.Script,
		"application/ecmascript": netlog.Script, "video/mp4": netlog.Media, "audio/ogg": netlog.Media,
	} {
		if got := netlog.TypeOf(media, netlog.Image); got != want {
			t.Errorf("%q is %s, want %s", media, got, want)
		}
	}
}

func TestRowsAreFilteredByTextAndType(t *testing.T) {
	rows := []netlog.Row{
		{ID: 1, Address: "https://s.test/", Type: netlog.Document},
		{ID: 2, Address: "https://s.test/css/Site.css", Type: netlog.Stylesheet},
		{ID: 3, Address: "https://s.test/map.png", Type: netlog.Image},
		{ID: 4, Address: "https://cdn.test/site.woff2", Type: netlog.Font},
		{ID: 5, Address: "https://s.test/feed.pdf", Type: netlog.Other},
		{ID: 6, Address: "https://s.test/api/items", Type: netlog.XHR},
	}
	ids := func(rows []netlog.Row) string {
		var out []int
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return fmt.Sprint(out)
	}
	for name, c := range map[string]struct {
		text string
		kind netlog.Type
		want string
	}{
		"nothing":         {"", netlog.Any, "[1 2 3 4 5 6]"},
		"a word":          {"SITE", netlog.Any, "[2 4]"},
		"a type":          {"", netlog.Image, "[3]"},
		"both":            {"s.test", netlog.Stylesheet, "[2]"},
		"fonts":           {"", netlog.Font, "[4]"},
		"the other types": {"", netlog.Other, "[5]"},
		"scripts' own":    {"", netlog.XHR, "[6]"},
	} {
		if got := ids(netlog.Filter(rows, c.text, c.kind)); got != c.want {
			t.Errorf("%s keeps %s, want %s", name, got, c.want)
		}
	}
}

func TestTotalsCountRequestsBytesAndTheTimeToTheLastAnswer(t *testing.T) {
	start := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	rows := []netlog.Row{
		{Size: 100, Started: start, Took: 300 * time.Millisecond, Done: true},
		{Size: 50, Started: start.Add(200 * time.Millisecond), Took: 400 * time.Millisecond, Done: true},
		{Size: 7, Started: start.Add(250 * time.Millisecond)},
	}
	got := netlog.Totals(rows)
	if got.Requests != 3 || got.Bytes != 157 || got.Took != 600*time.Millisecond {
		t.Fatalf("totals %+v", got)
	}
	if (netlog.Totals(nil) != netlog.Total{}) {
		t.Error("no rows total something")
	}
}

func TestARowIsNamedByTheLastPartOfItsAddress(t *testing.T) {
	for address, want := range map[string]string{
		"https://s.test/css/site.css?v=3": "site.css?v=3", "https://s.test/": "s.test", "https://s.test": "s.test",
		"https://s.test/a/b/": "b/", "::bad": "::bad",
	} {
		if got := (netlog.Row{Address: address}).Name(); got != want {
			t.Errorf("%s is named %q, want %q", address, got, want)
		}
	}
}
