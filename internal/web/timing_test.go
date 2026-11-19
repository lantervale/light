package web //nolint:testpackage // the marks a load sets are read through its unexported steps

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/fetch"
)

func TestALoadMarksItsStepsInOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/site.css" {
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte(`p { color: red; }`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<title>Marks</title><link rel="stylesheet" href="/site.css"><p>one</p>`))
	}))
	t.Cleanup(srv.Close)
	timing := &Timing{}
	loader := &Loader{Client: fetch.New(nil)}
	page := loader.Load(Timed(context.Background(), timing), srv.URL+"/", Links{})
	if page.Err != nil {
		t.Fatal(page.Err)
	}
	if page.Timing != timing {
		t.Fatal("the page does not keep the timing its load was given")
	}
	order := []Mark{Asked, Connected, Sent, Answered, Received, Parsed, Gathered, Built}
	for i, mark := range order {
		at := timing.At(mark)
		if at.IsZero() {
			t.Fatalf("%s is not marked", mark)
		}
		if i > 0 && at.Before(timing.At(order[i-1])) {
			t.Errorf("%s is marked before %s", mark, order[i-1])
		}
	}
	if !timing.At(Drawn).IsZero() {
		t.Error("a load marks what only the window knows")
	}
	if got := timing.Reached(); got != Built {
		t.Errorf("the load reached %s, want %s", got, Built)
	}
	var none *Timing
	none.Mark(Asked)
	if !none.At(Asked).IsZero() || Timed(context.Background(), nil) == nil {
		t.Error("no timing is not marked")
	}
}

func TestTheSheetsAndScriptsOfAPageAreAskedForTogether(t *testing.T) {
	const sheets, scripts = 20, 4
	var waiting atomic.Int32
	together := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			var page strings.Builder
			for i := range sheets {
				fmt.Fprintf(&page, `<link rel="stylesheet" href="/%d.css">`, i)
			}
			for i := range scripts {
				fmt.Fprintf(&page, `<script src="/%d.js"></script>`, i)
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, page.String()+`<p id="p">text</p>`)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".ico") {
			http.NotFound(w, r)
			return
		}
		if waiting.Add(1) == sheets+scripts {
			once.Do(func() { close(together) })
		}
		select {
		case <-together:
		case <-time.After(5 * time.Second):
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
		fmt.Fprintf(w, "#p { --from: %q }", r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	loader := testLoader()
	loader.Scripts = true
	nothing := func(string) {}
	page := loader.Load(t.Context(), srv.URL+"/", Links{Open: nothing, Over: nothing})
	if page.Err != nil {
		t.Fatal(page.Err)
	}
	select {
	case <-together:
	default:
		t.Fatalf("the server never held the %d sheets and scripts of the page at once: %d at most", sheets+scripts,
			waiting.Load())
	}
	if len(page.Scripts) != scripts {
		t.Fatalf("the page has %d scripts", len(page.Scripts))
	}
	for i, s := range page.Scripts {
		if want := fmt.Sprintf("/%d.js", i); !strings.Contains(s.Source, want) {
			t.Errorf("script %d of the page is %q: not in the page's order", i, s.Source)
		}
	}
}
