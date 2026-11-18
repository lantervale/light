package har_test

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/har"
	"github.com/lantervale/light/pkg/netlog"
)

//nolint:gochecknoglobals // a flag of the tests
var update = flag.Bool("update", false, "write the golden file again")

func began() time.Time {
	return time.Date(2026, time.November, 18, 15, 4, 5, 678_000_000, time.FixedZone("CET", 3600))
}

const png = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"

// crypt is the log of a small visit: a page that was redirected to, its
// sheet, gzipped, its picture, a form it sent, a request light refused,
// one that failed, a long text cut at the log's cap and one in Latin-1.
func crypt() []netlog.Row {
	at := func(ms int) time.Time { return began().Add(time.Duration(ms) * time.Millisecond) }
	head := func(pairs ...string) http.Header {
		h := http.Header{}
		for i := 0; i < len(pairs); i += 2 {
			h.Add(pairs[i], pairs[i+1])
		}
		return h
	}
	return []netlog.Row{
		{ID: 1, Address: "https://crypt.test/old?from=map&q=lower+vaults", Method: "GET", Status: 302, Reason: "Found",
			Type: netlog.Document, Initiator: "navigation", Started: at(0), Took: 12 * time.Millisecond, Done: true,
			Remote: "192.0.2.7:443", Proto: "HTTP/2.0",
			Request:  head("Accept", "text/html", "User-Agent", "light/1.0 (ember)", "Authorization", "Bearer t0ken"),
			Response: head("Location", "/vaults")},
		{ID: 2, Address: "https://crypt.test/vaults", Method: "GET", Status: 200, Reason: "OK", Media: "text/html",
			Type: netlog.Document, Initiator: "navigation", Started: at(13), Took: 40 * time.Millisecond, Done: true,
			Remote: "[2001:db8::7]:443", Proto: "HTTP/2.0", From: 1, Size: 27, Transferred: 27,
			Request: head("Accept", "text/html", "Cookie", "sid=abc123; theme=dark"),
			Response: head("Content-Type", "text/html; charset=utf-8",
				"Set-Cookie", "sid=def456; Path=/; Domain=crypt.test; Expires=Thu, 19 Nov 2026 15:04:05 GMT; HttpOnly; "+
					"Secure; SameSite=Lax",
				"Set-Cookie", "seen=1"),
			Body: []byte("<title>Vaults</title><p>wet")},
		{ID: 3, Address: "https://crypt.test/site.css", Method: "GET", Status: 200, Reason: "OK", Media: "text/css",
			Type: netlog.Stylesheet, Initiator: "document", Started: at(60), Took: 8 * time.Millisecond, Done: true,
			Remote: "192.0.2.7:443", Proto: "HTTP/2.0", Size: 20, Transferred: 12, Unzipped: true,
			Request:  head("Accept", "text/css", "Accept-Encoding", "gzip"),
			Response: head("Content-Type", "text/css", "Content-Encoding", "gzip"), Body: []byte("body { margin: 0; }\n")},
		{ID: 4, Address: "https://crypt.test/map.png", Method: "GET", Status: 200, Reason: "OK", Media: "image/png",
			Type: netlog.Image, Initiator: "picture", Started: at(61), Took: 9 * time.Millisecond, Done: true,
			Proto: "HTTP/2.0", Size: int64(len(png)), Transferred: int64(len(png)),
			Request: head("Accept", "image/*"), Response: head("Content-Type", "image/png"), Body: []byte(png)},
		{ID: 5, Address: "https://crypt.test/sign-in", Method: "POST", Status: 200, Reason: "OK", Media: "text/html",
			Type: netlog.Document, Initiator: "navigation", Started: at(5000), Took: 30 * time.Millisecond, Done: true,
			Proto: "HTTP/1.1", Size: 2, Transferred: 2, SentSize: 30,
			Request:  head("Content-Type", "application/x-www-form-urlencoded", "Origin", "https://crypt.test"),
			Response: head("Content-Type", "text/html"), Sent: []byte("name=Ada+L&pass=s3cret%21&keep"), Body: []byte("ok")},
		{ID: 6, Address: "https://tracker.test/beacon", Method: "POST", Type: netlog.XHR, Initiator: "app.js:12",
			Blocked: true, Err: "light sends no POST from a script", Done: true, Started: at(5100)},
		{ID: 7, Address: "https://gone.test/", Method: "GET", Type: netlog.Type("preflight"), Initiator: "app.js:14",
			Err: "dial tcp: no such host", Done: true, Started: at(5101), Took: 3 * time.Millisecond,
			Request: head("Accept", "*/*")},
		{ID: 8, Address: "https://crypt.test/long.txt", Method: "GET", Status: 200, Reason: "OK", Media: "text/plain",
			Type: netlog.Other, Initiator: "app.js:20", Started: at(5200), Took: time.Millisecond, Done: true,
			Proto: "HTTP/1.1", Size: 9, Transferred: 9, BodyCut: true,
			Response: head("Content-Type", "text/plain"), Body: []byte("caf\xc3\xa9 \xe2\x82")},
		{ID: 9, Address: "https://crypt.test/latin.txt", Method: "GET", Status: 200, Reason: "OK", Media: "text/plain",
			Type: netlog.Other, Initiator: "app.js:21", Started: at(5300), Took: time.Millisecond, Done: true,
			Proto: "HTTP/1.1", Size: 4, Transferred: 4,
			Response: head("Content-Type", "text/plain; charset=iso-8859-1"), Body: []byte("caf\xe9")},
	}
}

func options() har.Options {
	return har.Options{Version: "1.2.3", Title: func(address string) string {
		return map[string]string{"https://crypt.test/old?from=map&q=lower+vaults": "Vaults"}[address]
	}}
}

func encoded(t *testing.T, a *har.Archive) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := har.Encode(&out, a); err != nil {
		t.Fatal(err)
	}
	if err := har.Validate(out.Bytes()); err != nil {
		t.Fatalf("the archive is not HAR 1.2:\n%v", err)
	}
	return out.Bytes()
}

func TestTheArchiveOfASmallLogIsTheGoldenFile(t *testing.T) {
	got := encoded(t, har.From(crypt(), options()))
	const golden = "testdata/crypt.har"
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("the archive is not %s (go test ./pkg/har -update writes it again):\n%s", golden, got)
	}
}

// same fails the test when got is not want.
func same[T any](t *testing.T, what string, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s is %+v, want %+v", what, got, want)
	}
}

func TestAnArchiveNamesItsFormatItsCreatorAndItsPages(t *testing.T) {
	a := har.From(crypt(), options())
	light := har.Creator{Name: "light", Version: "1.2.3"}
	same(t, "the version", a.Log.Version, "1.2")
	same(t, "the creator", a.Log.Creator, light)
	same(t, "the browser", *a.Log.Browser, light)
	unknown := har.PageTimings{OnContentLoad: -1, OnLoad: -1}
	same(t, "the pages", a.Log.Pages, []har.Page{
		{StartedDateTime: "2026-11-18T14:04:05.678Z", ID: "page_1", Title: "Vaults", PageTimings: unknown},
		{StartedDateTime: "2026-11-18T14:04:10.678Z", ID: "page_5", Title: "https://crypt.test/sign-in",
			PageTimings: unknown},
	})
	var refs []string
	iso := regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$`)
	for _, e := range a.Log.Entries {
		refs = append(refs, e.Pageref)
		if !iso.MatchString(e.StartedDateTime) {
			t.Errorf("an entry began at %q", e.StartedDateTime)
		}
	}
	same(t, "the entries' pages", strings.Join(refs, " "),
		"page_1 page_1 page_1 page_1 page_5 page_5 page_5 page_5 page_5")
}

func TestAnEntrySaysWhatWasAskedAndWhatAnswered(t *testing.T) {
	a := har.From(crypt(), options())
	old, vaults := a.Log.Entries[0], a.Log.Entries[1]
	same(t, "the request", old.Request, har.Request{
		Method: http.MethodGet, URL: "https://crypt.test/old?from=map&q=lower+vaults", HTTPVersion: "HTTP/2.0",
		Cookies: []har.Cookie{},
		Headers: []har.Pair{{Name: "Host", Value: "crypt.test"}, {Name: "Accept", Value: "text/html"},
			{Name: "Authorization", Value: "Bearer t0ken"}, {Name: "User-Agent", Value: "light/1.0 (ember)"}},
		QueryString: []har.Pair{{Name: "from", Value: "map"}, {Name: "q", Value: "lower vaults"}},
		HeadersSize: -1,
	})
	same(t, "the redirect's answer", old.Response, har.Response{
		Status: http.StatusFound, StatusText: "Found", HTTPVersion: "HTTP/2.0", Cookies: []har.Cookie{},
		Headers: []har.Pair{{Name: "Location", Value: "/vaults"}}, Content: har.Content{MimeType: "x-unknown"},
		RedirectURL: "/vaults", HeadersSize: -1,
	})
	same(t, "the servers", []string{old.ServerIPAddress, old.Connection, vaults.ServerIPAddress},
		[]string{"192.0.2.7", "443", "2001:db8::7"})
	same(t, "the cookies sent", vaults.Request.Cookies,
		[]har.Cookie{{Name: "sid", Value: "abc123"}, {Name: "theme", Value: "dark"}})
	same(t, "the cookies set", vaults.Response.Cookies, []har.Cookie{
		{Name: "sid", Value: "def456", Path: "/", Domain: "crypt.test", Expires: "2026-11-19T15:04:05.000Z",
			HTTPOnly: true, Secure: true, SameSite: "Lax"},
		{Name: "seen", Value: "1"},
	})
}

func TestAnEntrySaysWhatAFormCarried(t *testing.T) {
	form := har.From(crypt(), options()).Log.Entries[4].Request
	same(t, "what the form carried", form.PostData, &har.PostData{
		MimeType: "application/x-www-form-urlencoded", Text: "name=Ada+L&pass=s3cret%21&keep",
		Params: []har.Pair{{Name: "name", Value: "Ada L"}, {Name: "pass", Value: "s3cret!"}, {Name: "keep"}},
	})
	same(t, "its size", form.BodySize, 30)
	same(t, "its length among the headers", slices.Contains(form.Headers, har.Pair{Name: "Content-Length", Value: "30"}),
		true)
}

func TestABodyIsTextByItsCharsetAndBase64ForTheRest(t *testing.T) {
	entries := har.From(crypt(), options()).Log.Entries
	same(t, "the page's content", entries[1].Response.Content,
		har.Content{Size: 27, MimeType: "text/html; charset=utf-8", Text: "<title>Vaults</title><p>wet"})
	same(t, "the gzipped sheet's content", entries[2].Response.Content,
		har.Content{Size: 20, Compression: 8, MimeType: "text/css", Text: "body { margin: 0; }\n"})
	same(t, "the bytes sent of the sheet", entries[2].Response.BodySize, 12)
	same(t, "the picture's content", entries[3].Response.Content, har.Content{Size: int64(len(png)),
		MimeType: "image/png", Text: base64.StdEncoding.EncodeToString([]byte(png)), Encoding: "base64"})
	same(t, "the cut text's content", entries[7].Response.Content, har.Content{Size: 9, MimeType: "text/plain",
		Text: "café ", Truncated: true, Comment: "Cut: the first 8 of the 9 bytes of this body, all light kept."})
	same(t, "the Latin-1 text's content", entries[8].Response.Content,
		har.Content{Size: 4, MimeType: "text/plain; charset=iso-8859-1", Text: "café"})
}

func TestABodyTheLogDidNotKeepIsSaidSoAndItsPreviewStands(t *testing.T) {
	rows := []netlog.Row{
		{ID: 1, Address: "http://crypt.test/a", Method: "GET", Status: 200, Size: 5000, Transferred: 5000, Done: true,
			Started: began(), Response: http.Header{"Content-Type": {"text/plain"}}, BodyDropped: true,
			Preview: []byte("the first words"), Truncated: true},
		{ID: 2, Address: "http://crypt.test/b", Method: "POST", Status: 200, Size: 7, Transferred: 7, SentSize: 9,
			SentCut: true, Done: true, Started: began(), Request: http.Header{"Content-Type": {"application/json"}},
			Response: http.Header{"Content-Type": {"application/octet-stream"}}},
		{ID: 3, Address: "http://crypt.test/c", Method: "GET", Status: 200, Size: 4, Transferred: 4, Done: true,
			Started: began(), Response: http.Header{"Content-Type": {"text/plain"}, "Content-Encoding": {"br"}},
			Body: []byte("\x0b\x01\x80h")},
	}
	a := har.From(rows, har.Options{})
	encoded(t, a)
	same(t, "the dropped body's content", a.Log.Entries[0].Response.Content, har.Content{Size: 5000,
		MimeType: "text/plain", Text: "the first words", Truncated: true,
		Comment: "Cut: the first 15 of the 5000 bytes of this body, all light kept."})
	same(t, "the content of a body not kept", a.Log.Entries[1].Response.Content, har.Content{Size: 7,
		MimeType: "application/octet-stream", Truncated: true, Comment: "light kept none of the 7 bytes of this body."})
	same(t, "what a request carried and was not kept", a.Log.Entries[1].Request.PostData, &har.PostData{
		MimeType: "application/json", Truncated: true, Comment: "light kept none of the 9 bytes of this body."})
	same(t, "its size", a.Log.Entries[1].Request.BodySize, 9)
	same(t, "the content of a body still encoded", a.Log.Entries[2].Response.Content, har.Content{Size: 4,
		MimeType: "text/plain", Text: "CwGAaA==", Encoding: "base64",
		Comment: "The body is as the server sent it, encoded as br: light does not undo that encoding."})
}

func TestARequestNeverSentAndOneThatFailedAreEntriesOfStatus0(t *testing.T) {
	entries := har.From(crypt(), options()).Log.Entries
	blocked, failed := entries[5], entries[6]
	none := har.Response{Cookies: []har.Cookie{}, Headers: []har.Pair{}, Content: har.Content{MimeType: "x-unknown"},
		HeadersSize: -1, BodySize: -1}
	same(t, "the blocked request", blocked, har.Entry{
		Pageref: "page_5", StartedDateTime: "2026-11-18T14:04:10.778Z",
		Request: har.Request{Method: http.MethodPost, URL: "https://tracker.test/beacon", Cookies: []har.Cookie{},
			Headers: []har.Pair{}, QueryString: []har.Pair{}, HeadersSize: -1},
		Response: none, Timings: har.Timings{Blocked: -1, DNS: -1, Connect: -1, SSL: -1},
		Kind: "xhr", ResourceType: "xhr", Initiator: &har.Initiator{Type: "script", Name: "app.js:12"},
		Blocked: "light sends no POST from a script",
	})
	same(t, "the failed request's answer", failed.Response, none)
	same(t, "its steps", failed.Timings, har.Timings{Blocked: -1, DNS: -1, Connect: -1, Wait: 3, SSL: -1})
	same(t, "its time", failed.Time, 3)
	same(t, "its kind, one har knows nothing of", failed.Kind, "preflight")
	same(t, "who asked for it", *failed.Initiator, har.Initiator{Type: "other", Name: "app.js:14"})
	same(t, "why it failed", []string{failed.Error, failed.Blocked}, []string{"dial tcp: no such host", ""})
}

func urls(a *har.Archive) string {
	var out []string
	for _, e := range a.Log.Entries {
		out = append(out, e.Pageref+" "+e.Request.URL)
	}
	return strings.Join(out, ", ")
}

func TestOnlyTheRowsKeptAreEntriesAndTheirPagesStand(t *testing.T) {
	o := options()
	o.Keep = func(r netlog.Row) bool { return r.Type == netlog.XHR || r.ID == 3 }
	a := har.From(crypt(), o)
	encoded(t, a)
	same(t, "the entries kept", urls(a), "page_1 https://crypt.test/site.css, page_5 https://tracker.test/beacon")
	same(t, "their pages", len(a.Log.Pages), 2)
	o.Keep = func(r netlog.Row) bool { return r.ID == 6 }
	a = har.From(crypt(), o)
	same(t, "the pages of one request", []string{a.Log.Pages[0].ID}, []string{"page_5"})
	a = har.From(crypt()[2:4], har.Options{})
	encoded(t, a)
	same(t, "the entries of no navigation", urls(a), " https://crypt.test/site.css,  https://crypt.test/map.png")
	same(t, "their pages", len(a.Log.Pages), 0)
	encoded(t, har.From(nil, har.Options{}))
}

func TestSanitizeRedactsCredentialsAndLeavesAddressesAndBodies(t *testing.T) {
	a := har.From(crypt(), options())
	har.Sanitize(a)
	out := string(encoded(t, a))
	for _, secret := range []string{"abc123", "def456", "Bearer t0ken", `"value": "dark"`} {
		if strings.Contains(out, secret) {
			t.Errorf("the sanitized archive still holds %s", secret)
		}
	}
	for _, kept := range []string{"pass=s3cret%21", `"value": "s3cret!"`, "from=map", `"name": "sid"`, "light/1.0 (ember)",
		`"name": "Set-Cookie"`, `"name": "Authorization"`} {
		if !strings.Contains(out, kept) {
			t.Errorf("the sanitized archive lost %s", kept)
		}
	}
	same(t, "the values redacted: 4 headers', 4 cookies', and the comment naming it", strings.Count(out, har.Redacted), 9)
	same(t, "the comment's start", strings.HasPrefix(a.Log.Comment, "Sanitized"), true)
	full := string(encoded(t, har.From(crypt(), options())))
	same(t, "the secrets of an archive not sanitized",
		[]bool{strings.Contains(full, "abc123"), strings.Contains(full, "Bearer t0ken"),
			strings.Contains(full, har.Redacted)},
		[]bool{true, true, false})
}

// object is v as the JSON object it is.
func object(v any) map[string]any {
	o, _ := v.(map[string]any)
	return o
}

func TestValidateSaysWhatIsNotTheFormat(t *testing.T) {
	good := encoded(t, har.From(crypt(), options()))
	broken := func(change func(log, first map[string]any)) error {
		var file map[string]any
		if err := json.Unmarshal(good, &file); err != nil {
			t.Fatal(err)
		}
		log := object(file["log"])
		entries, _ := log["entries"].([]any)
		change(log, object(entries[0]))
		data, _ := json.Marshal(file)
		return har.Validate(data)
	}
	for want, change := range map[string]func(log, first map[string]any){
		"log.creator is missing":                func(log, _ map[string]any) { delete(log, "creator") },
		`log.version is "1.1"`:                  func(log, _ map[string]any) { log["version"] = "1.1" },
		"log.entries[0].request.url is missing": func(_, first map[string]any) { delete(object(first["request"]), "url") },
		"log.entries[0].response.status is 302, want a": func(_, first map[string]any) {
			object(first["response"])["status"] = "302"
		},
		"log.entries[0].startedDateTime is yesterday": func(_, first map[string]any) {
			first["startedDateTime"] = "yesterday"
		},
		"log.entries[0].kind is not a field":   func(_, first map[string]any) { first["kind"] = "document" },
		"log.entries[0].pageref names no page": func(_, first map[string]any) { first["pageref"] = "page_0" },
		"log.entries[0].time is 99":            func(_, first map[string]any) { first["time"] = 99.0 },
		"log.entries[0].timings.wait is -1":    func(_, first map[string]any) { object(first["timings"])["wait"] = -1.0 },
		"log.entries[0].request.headers is {}, want an": func(_, first map[string]any) {
			object(first["request"])["headers"] = "{}"
		},
		"log.entries[0].response.content.encoding is": func(_, first map[string]any) {
			object(object(first["response"])["content"])["encoding"] = "hex"
		},
		"log.entries[0].request.bodySize is 1.5, want a": func(_, first map[string]any) {
			object(first["request"])["bodySize"] = 1.5
		},
	} {
		if err := broken(change); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("the fault is %v, want %s", err, want)
		}
	}
	if err := har.Validate([]byte("{")); err == nil {
		t.Error("what is not JSON is valid")
	}
}

// TestTheArchiveOfRealRequests goes through the transport: a gzipped
// answer is its text with both sizes, each hop of a redirect an entry,
// and the steps add up.
func TestTheArchiveOfRealRequests(t *testing.T) {
	plain := strings.Repeat("the lower vaults stay wet. ", 100)
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/wet?a=1", http.StatusFound)
	})
	mux.HandleFunc("/wet", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Encoding", "gzip")
		zip := gzip.NewWriter(w)
		_, _ = io.WriteString(zip, plain)
		_ = zip.Close()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	l := netlog.New(10)
	l.KeepBodies(netlog.BodyCap, netlog.BodyBudget)
	req, _ := http.NewRequestWithContext(netlog.With(t.Context(), l, har.Navigation, netlog.Document), http.MethodGet,
		srv.URL+"/old", nil)
	resp, err := (&http.Client{Transport: &netlog.Transport{Base: &http.Transport{}}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	a := har.From(l.Rows(), har.Options{})
	encoded(t, a)
	if len(a.Log.Entries) != 2 || len(a.Log.Pages) != 1 {
		t.Fatalf("%d entries of %d pages", len(a.Log.Entries), len(a.Log.Pages))
	}
	hop, wet := a.Log.Entries[0], a.Log.Entries[1]
	if hop.Response.Status != 302 || hop.Response.RedirectURL != "/wet?a=1" || wet.Request.URL != srv.URL+"/wet?a=1" {
		t.Fatalf("the hops are %+v and %+v", hop.Response, wet.Request)
	}
	c := wet.Response.Content
	if c.Text != plain || c.Encoding != "" || c.Size != int64(len(plain)) || wet.Response.BodySize >= c.Size ||
		wet.Response.BodySize <= 0 || c.Compression != c.Size-wet.Response.BodySize {
		t.Fatalf("the gzipped answer is %d bytes of text, size %d, %d sent", len(c.Text), c.Size, wet.Response.BodySize)
	}
	if hop.Timings.Connect < 0 || hop.Timings.DNS != -1 || wet.Timings.Connect != -1 || wet.Timings.Wait <= 0 ||
		hop.ServerIPAddress != "127.0.0.1" || wet.Response.HTTPVersion != "HTTP/1.1" {
		t.Fatalf("the steps are %+v then %+v, of %s", hop.Timings, wet.Timings, hop.ServerIPAddress)
	}
}
