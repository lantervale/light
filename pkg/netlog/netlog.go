// Package netlog records the HTTP requests made through its Transport,
// as a browser's network panel lists them: one row per request, each hop
// of a redirect its own, with what was asked, what was answered, how much
// and how long. A request is recorded in the Log its context carries
// (With), from any goroutine.
package netlog

import (
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lantervale/light/pkg/instant"
)

// PreviewCap is how much of an answer's body a row keeps to be looked at.
const PreviewCap = 256 << 10

// BodyCap and BodyBudget are what KeepBodies is given by who has no
// reason for other sizes: the most kept of one body, and of them all.
const (
	BodyCap    = 4 << 20
	BodyBudget = 32 << 20
)

// Type is what a request fetched, as the panel groups them.
type Type string

const (
	// Any is no type: every row passes a filter by it.
	Any        Type = ""
	Document   Type = "document"
	Stylesheet Type = "stylesheet"
	Script     Type = "script"
	Image      Type = "image"
	Font       Type = "font"
	Media      Type = "media"
	// XHR is what a script asked for through fetch or XMLHttpRequest.
	XHR Type = "xhr"
	// Preflight is the OPTIONS request that asks another origin whether
	// it takes a script's request, before that request is sent.
	Preflight Type = "preflight"
	Other     Type = "other"
)

// TypeOf is the type of an answer of that media type: who asked first,
// what a script asked for being XHR whatever is answered, then the media
// type, and what was wanted when it declares none, as a redirect or a
// failure does.
func TypeOf(media string, wanted Type) Type {
	switch {
	case media == "" || wanted == XHR || wanted == Preflight:
		return wanted
	case media == "text/html" || media == "application/xhtml+xml":
		return Document
	case media == "text/css":
		return Stylesheet
	case scripted(media):
		return Script
	case strings.HasPrefix(media, "image/"):
		return Image
	case strings.HasPrefix(media, "font/") || strings.HasPrefix(media, "application/font") ||
		strings.HasPrefix(media, "application/x-font"):
		return Font
	case strings.HasPrefix(media, "audio/") || strings.HasPrefix(media, "video/"):
		return Media
	}
	return Other
}

// scripted reports whether media is one JavaScript is sent as.
func scripted(media string) bool {
	return strings.HasSuffix(media, "/javascript") || strings.HasSuffix(media, "/x-javascript") ||
		strings.HasSuffix(media, "/ecmascript")
}

// Row is one request and its answer so far.
type Row struct {
	// ID numbers the rows of a Log from 1, in the order they were asked.
	ID              int
	Address, Method string
	// Status is the answer's code and Reason its phrase, 0 and "" until
	// it comes, and for a request that got none.
	Status int
	Reason string
	// Err is why there is no answer, or why its body stopped: "cancelled"
	// for a request stopped on purpose.
	Err   string
	Media string
	// Type is what the answer is, by its media type; what was wanted
	// while there is none, and for an error status, whose body is the
	// server's word about it.
	Type Type
	// Initiator is who asked, as With named it: "document", "stylesheet",
	// or the script and the line of a script's request.
	Initiator string
	// Blocked is whether light refused to send the request, or kept its
	// answer from the script that asked: Err says why.
	Blocked bool
	// Size is the body's bytes read so far, as the client got them: after
	// a gzip encoding was undone. Transferred is those the server sent.
	Size, Transferred int64
	Started           time.Time
	// Took is how long the request has taken, to the end of its body once
	// Done.
	Took time.Duration
	Done bool
	// Remote is the address of the server's end of the connection,
	// Reused whether another request went through it before and Idle
	// whether it waited for one.
	Remote            string
	Reused, Idle      bool
	Request, Response http.Header
	// Preview is the first PreviewCap bytes of the body, Truncated when
	// it went on.
	Preview   []byte
	Truncated bool
	// From is the row whose redirect this request follows, 0 for none.
	From int
	// Proto is the answer's protocol, as "HTTP/1.1", and Unzipped
	// whether the encoding its headers name, gzip or deflate, was undone.
	Proto    string
	Unzipped bool
	// Sent is what the request carried and Body the answer's body as
	// Size counts it, each up to the cap of a log that keeps bodies
	// (KeepBodies) and nil in one that does not. SentSize is all the
	// request carried; SentCut and BodyCut tell one that went on past
	// the cap, and BodyDropped one whose place newer bodies took.
	Sent                          []byte
	SentSize                      int64
	Body                          []byte
	SentCut, BodyCut, BodyDropped bool
	// marks are when the steps of the request were taken, and gone
	// whether the row left its log.
	marks marks
	gone  bool
}

// Failed reports whether the request got no answer, or an error status.
func (r Row) Failed() bool { return r.Err != "" || r.Status >= http.StatusBadRequest }

// Host is the site the row's address is of, "" when it names none.
func (r Row) Host() string {
	u, err := url.Parse(r.Address)
	if err != nil {
		return ""
	}
	return u.Host
}

// Secure reports whether the request went over TLS.
func (r Row) Secure() bool { return strings.HasPrefix(strings.ToLower(r.Address), "https://") }

// Kind is what the answer is in a word, as a network panel's Type
// column: "preflight" for one, else the end of its media type ("html",
// "css", "js", "json", "png"), else the extension of its address, ""
// when neither says.
func (r Row) Kind() string {
	kind := r.Media[strings.LastIndexAny(r.Media, "/.")+1:]
	switch {
	case r.Type == Preflight:
		return string(Preflight)
	case scripted(r.Media):
		return "js"
	case kind == "":
		u, err := url.Parse(r.Address)
		if err != nil {
			return ""
		}
		return strings.ToLower(strings.TrimPrefix(path.Ext(u.Path), "."))
	}
	kind = strings.TrimPrefix(strings.TrimPrefix(kind, "x-"), "font-")
	kind, _, _ = strings.Cut(kind, "+")
	return kind
}

// Name is the last part of the row's address, with its query, or its host
// for the root of a site.
func (r Row) Name() string {
	u, err := url.Parse(r.Address)
	if err != nil || u.Host == "" {
		return r.Address
	}
	trimmed := strings.TrimSuffix(u.Path, "/")
	if trimmed == "" {
		return u.Host
	}
	name := trimmed[strings.LastIndex(trimmed, "/")+1:] + u.Path[len(trimmed):]
	if u.RawQuery != "" {
		name += "?" + u.RawQuery
	}
	return name
}

// Log is the rows of one tab, oldest first, never more than its cap.
type Log struct {
	// mu guards every field below: requests end on their own goroutines
	// while the panel reads.
	mu      sync.Mutex
	rows    []*Row
	last    int
	limit   int
	dropped int
	version uint64
	// hops are the rows of redirects not yet followed, by their answer.
	hops map[*http.Response]int
	// each is the most kept of one body, all of every row's together,
	// and held what is.
	each, all, held int64
}

// New is an empty log keeping the last limit rows.
func New(limit int) *Log { return &Log{limit: limit, hops: map[*http.Response]int{}} }

// Rows is a copy of the rows as they stand.
func (l *Log) Rows() []Row {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Row, len(l.rows))
	for i, r := range l.rows {
		out[i] = *r
	}
	return out
}

// Row is the row numbered id, false once it is dropped or cleared.
func (l *Log) Row(id int) (Row, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.rows {
		if r.ID == id {
			return *r, true
		}
	}
	return Row{}, false
}

// Dropped is how many rows went past the cap since the log was cleared.
func (l *Log) Dropped() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.dropped
}

// Version changes whenever a row does, so a reader knows to read again.
func (l *Log) Version() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.version
}

// Clear empties the log. Requests on their way stay out of it.
func (l *Log) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.rows {
		r.gone = true
	}
	l.rows, l.dropped, l.held = nil, 0, 0
	clear(l.hops)
	l.version++
}

// add starts a row for a request and gives it to change.
func (l *Log) add(r Row, follows *http.Response) *Row {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.last++
	r.ID = l.last
	if follows != nil {
		r.From = l.hops[follows]
		delete(l.hops, follows)
	}
	row := &r
	l.rows = append(l.rows, row)
	l.trim()
	l.version++
	return row
}

// Refuse marks the row numbered id as one whose answer light kept from
// the script that asked, and why: the request was sent.
func (l *Log) Refuse(id int, why string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.rows {
		if r.ID == id {
			r.Blocked, r.Err = true, why
			l.version++
		}
	}
}

// Block lists a request light refused to send, and why.
func (l *Log) Block(method, address, initiator string, wanted Type, why string) {
	l.add(Row{
		Address: address, Method: method, Type: wanted, Initiator: initiator, Blocked: true, Err: why, Done: true,
		Started: instant.Now(),
	}, nil)
}

// trim drops the oldest rows past the cap, and counts them.
func (l *Log) trim() {
	if over := len(l.rows) - l.limit; over > 0 {
		for _, r := range l.rows[:over] {
			l.held -= int64(len(r.Body) + len(r.Sent))
			r.gone = true
		}
		l.rows = slices.Delete(l.rows, 0, over)
		l.dropped += over
	}
}

// Limit is how many rows are kept.
func (l *Log) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}

// SetLimit changes how many rows are kept, dropping the oldest past it.
func (l *Log) SetLimit(limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = limit
	l.trim()
	l.version++
}

// change changes a row under the log's lock.
func (l *Log) change(row *Row, how func(r *Row)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	how(row)
	l.version++
}

// redirected notes that resp, row's answer, sends the request elsewhere.
func (l *Log) redirected(row *Row, resp *http.Response) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hops[resp] = row.ID
}

// Chain is the hops of the redirect the row numbered id is part of, first
// to last: that row alone when nothing redirected to it or from it.
func Chain(rows []Row, id int) []Row {
	byID := map[int]Row{}
	next := map[int]int{}
	for _, r := range rows {
		byID[r.ID] = r
		if r.From != 0 {
			next[r.From] = r.ID
		}
	}
	first := id
	for byID[first].From != 0 {
		if _, ok := byID[byID[first].From]; !ok {
			break
		}
		first = byID[first].From
	}
	var chain []Row
	for at, ok := first, true; ok; at, ok = next[at], next[at] != 0 {
		if r, found := byID[at]; found {
			chain = append(chain, r)
		}
	}
	return chain
}

// Filter is the rows whose address holds text, whatever its case, and
// whose type is kind: a preflight goes with the requests of scripts.
func Filter(rows []Row, text string, kind Type) []Row {
	text = strings.ToLower(text)
	var out []Row
	for _, r := range rows {
		of := kind == Any || r.Type == kind || (kind == XHR && r.Type == Preflight)
		if of && strings.Contains(strings.ToLower(r.Address), text) {
			out = append(out, r)
		}
	}
	return out
}

// Total sums rows up.
type Total struct {
	Requests int
	// Bytes is what was read, Transferred what the servers sent.
	Bytes, Transferred int64
	// Took is from the first request to the end of the last answer.
	Took time.Duration
}

func Totals(rows []Row) Total {
	var t Total
	var first, last time.Time
	for _, r := range rows {
		t.Requests++
		t.Bytes += r.Size
		t.Transferred += r.Transferred
		if first.IsZero() || r.Started.Before(first) {
			first = r.Started
		}
		if end := r.Started.Add(r.Took); r.Done && end.After(last) {
			last = end
		}
	}
	if last.After(first) {
		t.Took = last.Sub(first)
	}
	return t
}
