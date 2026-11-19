package netlog_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/netlog"
)

// served is a server answering each path with what its function writes.
func served(t *testing.T, by map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, handler := range by {
		mux.HandleFunc(path, handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func text(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, body)
	}
}

// keeping is a log that keeps bodies, and a context recording in it.
func keeping(t *testing.T, each, all int64) (*netlog.Log, context.Context) {
	t.Helper()
	l := netlog.New(100)
	l.KeepBodies(each, all)
	return l, netlog.With(t.Context(), l, "test", netlog.Other)
}

// read asks for address and gives the body its consumer read.
func read(ctx context.Context, t *testing.T, method, address string, body io.Reader) string {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, address, body)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := (&http.Client{Transport: &netlog.Transport{}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

func TestALogKeepsNoBodyUntilTold(t *testing.T) {
	srv := served(t, map[string]http.HandlerFunc{"/": text("hello")})
	l := netlog.New(10)
	read(netlog.With(t.Context(), l, "test", netlog.Other), t, http.MethodPost, srv.URL+"/", strings.NewReader("a=1"))
	r := l.Rows()[0]
	if r.Body != nil || r.Sent != nil || l.Held() != 0 {
		t.Fatalf("a log not told to keeps %q and %q, %d bytes", r.Body, r.Sent, l.Held())
	}
	if r.Size != 5 || r.SentSize != 3 || !r.SentCut || string(r.Preview) != "hello" || r.Truncated {
		t.Fatalf("the row counts %d read and %d sent (cut %v), previews %q", r.Size, r.SentSize, r.SentCut, r.Preview)
	}
}

func TestBodiesAreKeptAsTheConsumerReadsThem(t *testing.T) {
	big := strings.Repeat("0123456789", 100_000)
	srv := served(t, map[string]http.HandlerFunc{"/": text(big), "/form": func(w http.ResponseWriter, r *http.Request) {
		sent, _ := io.ReadAll(r.Body)
		_, _ = w.Write(append([]byte("got "), sent...))
	}})
	l, ctx := keeping(t, netlog.BodyCap, netlog.BodyBudget)
	if got := read(ctx, t, http.MethodGet, srv.URL+"/", nil); got != big {
		t.Fatalf("the consumer read %d bytes of %d", len(got), len(big))
	}
	if got := read(ctx, t, http.MethodPost, srv.URL+"/form", strings.NewReader("name=Ada&pass=s3cret")); got !=
		"got name=Ada&pass=s3cret" {
		t.Fatalf("the consumer read %q", got)
	}
	rows := l.Rows()
	if string(rows[0].Body) != big || rows[0].BodyCut || rows[0].BodyDropped || len(rows[0].Preview) !=
		netlog.PreviewCap || !rows[0].Truncated {
		t.Fatalf("the long body is kept as %d bytes, cut %v, previewed over %d", len(rows[0].Body), rows[0].BodyCut,
			len(rows[0].Preview))
	}
	post := rows[1]
	if string(post.Sent) != "name=Ada&pass=s3cret" || post.SentSize != 20 || post.SentCut ||
		string(post.Body) != "got name=Ada&pass=s3cret" || post.Proto != "HTTP/1.1" {
		t.Fatalf("the POST's row is %+v", post)
	}
	if want := int64(len(big) + len(post.Sent) + len(post.Body)); l.Held() != want {
		t.Fatalf("the log holds %d bytes, want %d", l.Held(), want)
	}
	if got := post.Request.Get("Accept-Encoding"); got != netlog.Encodings("http") {
		t.Fatalf("the row says the request accepted %q, and %s was asked for", got, netlog.Encodings("http"))
	}
}

func TestTheFirstBytesOfASlowBodyReachTheConsumerBeforeTheLastAreSent(t *testing.T) {
	more := make(chan struct{})
	srv := served(t, map[string]http.HandlerFunc{"/": func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "first ")
		_ = http.NewResponseController(w).Flush()
		<-more
		_, _ = io.WriteString(w, "last")
	}})
	l, ctx := keeping(t, netlog.BodyCap, netlog.BodyBudget)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/", nil)
	resp, err := (&http.Client{Transport: &netlog.Transport{}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	start := make([]byte, 6)
	if _, err = io.ReadFull(resp.Body, start); err != nil || string(start) != "first " {
		t.Fatalf("the consumer read %q, %v, while the server holds the rest", start, err)
	}
	if r := l.Rows()[0]; string(r.Body) != "first " || r.Done {
		t.Fatalf("the row keeps %q, done %v, of a body half sent", r.Body, r.Done)
	}
	close(more)
	rest, _ := io.ReadAll(resp.Body)
	if r := l.Rows()[0]; string(rest) != "last" || string(r.Body) != "first last" || !r.Done {
		t.Fatalf("the consumer read %q, the row keeps %q", rest, r.Body)
	}
}

func TestABodyOverTheCapIsCutAndCountedWhole(t *testing.T) {
	srv := served(t, map[string]http.HandlerFunc{"/": func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, strings.Repeat("b", 1000))
	}})
	l, ctx := keeping(t, 64, 1<<20)
	if got := read(ctx, t, http.MethodPost, srv.URL+"/", strings.NewReader(strings.Repeat("s", 500))); len(got) != 1000 {
		t.Fatalf("the consumer read %d bytes", len(got))
	}
	r := l.Rows()[0]
	if len(r.Body) != 64 || !r.BodyCut || r.Size != 1000 || len(r.Sent) != 64 || !r.SentCut || r.SentSize != 500 {
		t.Fatalf("kept %d of %d read (cut %v), %d of %d sent (cut %v)", len(r.Body), r.Size, r.BodyCut, len(r.Sent),
			r.SentSize, r.SentCut)
	}
	if len(r.Preview) != 1000 || r.Truncated {
		t.Fatalf("the preview is %d bytes, cut %v: it has its own cap", len(r.Preview), r.Truncated)
	}
	if l.Held() != 128 {
		t.Fatalf("the log holds %d bytes", l.Held())
	}
}

func TestTheOldestBodiesGoFirstPastTheBudget(t *testing.T) {
	srv := served(t, map[string]http.HandlerFunc{"/": func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat(r.URL.Query().Get("of"), 400))
	}})
	l, ctx := keeping(t, 500, 1000)
	for _, of := range []string{"a", "b", "c"} {
		read(ctx, t, http.MethodGet, srv.URL+"/?of="+of, nil)
	}
	rows := l.Rows()
	if rows[0].Body != nil || !rows[0].BodyDropped || rows[0].Size != 400 || string(rows[0].Preview) !=
		strings.Repeat("a", 400) {
		t.Fatalf("the oldest row keeps %d bytes, dropped %v, previews %d", len(rows[0].Body), rows[0].BodyDropped,
			len(rows[0].Preview))
	}
	if len(rows[1].Body) != 400 || len(rows[2].Body) != 400 || rows[2].BodyDropped || l.Held() != 800 {
		t.Fatalf("the newer rows keep %d and %d bytes, the log %d", len(rows[1].Body), len(rows[2].Body), l.Held())
	}
	l.SetLimit(1)
	if l.Held() != 400 {
		t.Fatalf("with one row left the log holds %d bytes", l.Held())
	}
	l.Clear()
	if l.Held() != 0 {
		t.Fatalf("an emptied log holds %d bytes", l.Held())
	}
}

func TestABodyStillComingWhenItsLogIsEmptiedIsNotKept(t *testing.T) {
	more := make(chan struct{})
	srv := served(t, map[string]http.HandlerFunc{"/": func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "first ")
		_ = http.NewResponseController(w).Flush()
		<-more
		_, _ = io.WriteString(w, strings.Repeat("x", 10_000))
	}})
	l, ctx := keeping(t, netlog.BodyCap, netlog.BodyBudget)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/", nil)
	resp, err := (&http.Client{Transport: &netlog.Transport{}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadFull(resp.Body, make([]byte, 6))
	l.Clear()
	close(more)
	_, _ = io.Copy(io.Discard, resp.Body)
	if l.Held() != 0 || len(l.Rows()) != 0 {
		t.Fatalf("an emptied log holds %d bytes in %d rows", l.Held(), len(l.Rows()))
	}
}

func TestAGzippedBodyIsKeptAsItWasRead(t *testing.T) {
	plain := strings.Repeat("the lower vaults stay wet. ", 200)
	srv := served(t, map[string]http.HandlerFunc{"/": func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Encoding", "gzip")
		zip := gzip.NewWriter(w)
		_, _ = io.WriteString(zip, plain)
		_ = zip.Close()
	}})
	l, ctx := keeping(t, netlog.BodyCap, netlog.BodyBudget)
	if got := read(ctx, t, http.MethodGet, srv.URL+"/", nil); got != plain {
		t.Fatal("the consumer did not read the body as it was before gzip")
	}
	r := l.Rows()[0]
	if string(r.Body) != plain || r.Size != int64(len(plain)) || r.Transferred >= r.Size || r.Transferred == 0 {
		t.Fatalf("kept %d bytes of %d read, %d sent", len(r.Body), r.Size, r.Transferred)
	}
}

func TestEachHopOfARedirectedPOSTKeepsWhatItCarried(t *testing.T) {
	srv := served(t, map[string]http.HandlerFunc{
		"/a": func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/b", http.StatusTemporaryRedirect) },
		"/b": func(w http.ResponseWriter, r *http.Request) {
			sent, _ := io.ReadAll(r.Body)
			_, _ = w.Write(sent)
		},
	})
	l, ctx := keeping(t, netlog.BodyCap, netlog.BodyBudget)
	if got := read(ctx, t, http.MethodPost, srv.URL+"/a", bytes.NewReader([]byte("q=1"))); got != "q=1" {
		t.Fatalf("the consumer read %q", got)
	}
	rows := l.Rows()
	if len(rows) != 2 || string(rows[0].Sent) != "q=1" || string(rows[1].Sent) != "q=1" || rows[1].From != rows[0].ID ||
		rows[0].Response.Get("Location") != "/b" {
		t.Fatalf("the hops are %+v", rows)
	}
}

func TestTheStepsOfARequestAddUpToWhatItTook(t *testing.T) {
	srv := served(t, map[string]http.HandlerFunc{"/": func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(20 * time.Millisecond)
		_, _ = io.WriteString(w, "late")
	}})
	l, ctx := keeping(t, netlog.BodyCap, netlog.BodyBudget)
	client := &http.Client{Transport: &netlog.Transport{Base: &http.Transport{}}}
	for range 2 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	rows := l.Rows()
	first, again := rows[0].Timing(), rows[1].Timing()
	if first.Connect < 0 || first.DNS != -1 || first.TLS != -1 || first.Blocked < 0 {
		t.Fatalf("a first request to an address by its number took %+v", first)
	}
	if again.Connect != -1 || again.Blocked < 0 {
		t.Fatalf("a request on the connection of another took %+v", again)
	}
	if rows[0].Reused || rows[0].Idle || !rows[1].Reused || !rows[1].Idle {
		t.Fatalf("the first request's connection was used before (%t), the second's was not (%t), or was not idle (%t)",
			rows[0].Reused, !rows[1].Reused, !rows[1].Idle)
	}
	for i, timing := range []netlog.Timing{first, again} {
		if timing.Wait < 20*time.Millisecond || timing.Send < 0 || timing.Receive < 0 {
			t.Fatalf("request %d took %+v, and the server waited 20 ms", i, timing)
		}
		sum := timing.Blocked + max(0, timing.DNS) + max(0, timing.Connect) + timing.Send + timing.Wait + timing.Receive
		if off := sum - rows[i].Took; off < -time.Millisecond || off > time.Millisecond {
			t.Fatalf("request %d: the steps %+v add up to %v, and it took %v", i, timing, sum, rows[i].Took)
		}
	}
}

func TestARowOfATransportThatTellsNoStepStillWaitsAndReceives(t *testing.T) {
	l, ctx := keeping(t, netlog.BodyCap, netlog.BodyBudget)
	client := &http.Client{Transport: &netlog.Transport{Base: canned("hello")}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://crypt.test/", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	r := l.Rows()[0]
	timing := r.Timing()
	if timing.Blocked != -1 || timing.DNS != -1 || timing.Connect != -1 || timing.TLS != -1 || timing.Send != 0 ||
		timing.Wait < 0 || timing.Receive < 0 || timing.Wait+timing.Receive != r.Took {
		t.Fatalf("the steps are %+v of %v", timing, r.Took)
	}
	l.Block(http.MethodPost, "http://crypt.test/", "app.js:3", netlog.XHR, "refused")
	if got := l.Rows()[1].Timing(); got != (netlog.Timing{Blocked: -1, DNS: -1, Connect: -1, TLS: -1}) {
		t.Fatalf("a request never sent took %+v", got)
	}
}

// canned answers every request with body, without a connection.
type canned string

func (c canned) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Proto: "HTTP/1.1", Header: http.Header{},
		Body: io.NopCloser(strings.NewReader(string(c))), Request: req}, nil
}

// TestTheBodiesOfALogStayWithinItsBudget guards the memory a tab's log
// takes: many large answers leave no more of them than the budget.
func TestTheBodiesOfALogStayWithinItsBudget(t *testing.T) {
	const each, all, answers, size = 256 << 10, 1 << 20, 64, 512 << 10
	l, ctx := keeping(t, each, all)
	client := &http.Client{Transport: &netlog.Transport{Base: canned(strings.Repeat("x", size))}}
	heap := func() uint64 {
		runtime.GC()
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapAlloc
	}
	before := heap()
	for i := range answers {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://crypt.test/%d", i), nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if held := l.Held(); held > all {
			t.Fatalf("after %d answers the log holds %d bytes, over its budget of %d", i+1, held, all)
		}
	}
	func() {
		rows := l.Rows()
		kept := 0
		for _, r := range rows {
			kept += len(r.Body)
			if r.Size != size || len(r.Preview) != netlog.PreviewCap {
				t.Fatalf("row %d counts %d bytes and previews %d", r.ID, r.Size, len(r.Preview))
			}
		}
		if kept != all || !rows[0].BodyDropped || rows[len(rows)-1].BodyDropped || !rows[len(rows)-1].BodyCut {
			t.Fatalf("%d bytes are kept of a budget of %d", kept, all)
		}
	}()
	// What stays is the previews (as before bodies were kept) and the
	// budget; the slack is for the rows themselves and the allocator.
	if grown, most := int64(heap())-int64(before), int64(answers*netlog.PreviewCap+all+(4<<20)); grown > most {
		t.Fatalf("%d answers of %d bytes left %d bytes on the heap, over %d", answers, size, grown, most)
	}
	l.Clear()
	if grown := int64(heap()) - int64(before); grown > 2<<20 {
		t.Fatalf("an emptied log leaves %d bytes on the heap", grown)
	}
	runtime.KeepAlive(l)
}
