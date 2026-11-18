package har

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"maps"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html/charset"

	"github.com/lantervale/light/pkg/netlog"
)

// Navigation is who asks for a page in light's logs.
const Navigation = "navigation"

// Options are what From is told beside the rows.
type Options struct {
	// Creator and Version name the program, "light" when Creator is "".
	Creator, Version string
	// Title is the title of the page at address, the address when nil
	// or "".
	Title func(address string) string
	// Navigation is the initiator of the requests that begin a page,
	// Navigation when "".
	Navigation string
	// Keep chooses the rows written, all of them when nil. The pages
	// are still those of every row.
	Keep func(r netlog.Row) bool
}

// From is the archive of rows, a log's in the order they were asked: a
// page for each navigation that has an entry, and an entry for each row
// kept, whatever its type, those never sent and those that failed
// included, with a status of 0.
func From(rows []netlog.Row, o Options) *Archive {
	if o.Creator == "" {
		o.Creator = "light"
	}
	if o.Navigation == "" {
		o.Navigation = Navigation
	}
	who := Creator{Name: o.Creator, Version: o.Version}
	a := &Archive{Log: Log{Version: Version, Creator: who, Browser: &who, Pages: []Page{}, Entries: []Entry{}}}
	var page *Page
	listed := false
	for _, r := range rows {
		if r.Initiator == o.Navigation && r.From == 0 {
			title := ""
			if o.Title != nil {
				title = o.Title(r.Address)
			}
			if title == "" {
				title = r.Address
			}
			page = &Page{StartedDateTime: date(r.Started), ID: "page_" + strconv.Itoa(r.ID), Title: title,
				PageTimings: PageTimings{OnContentLoad: -1, OnLoad: -1}}
			listed = false
		}
		if o.Keep != nil && !o.Keep(r) {
			continue
		}
		e := entry(r)
		if page != nil {
			e.Pageref = page.ID
			if !listed {
				a.Log.Pages, listed = append(a.Log.Pages, *page), true
			}
		}
		a.Log.Entries = append(a.Log.Entries, e)
	}
	return a
}

func date(at time.Time) string { return at.UTC().Format("2006-01-02T15:04:05.000Z") }

// ms is d in milliseconds, -1 for a step not taken.
func ms(d time.Duration) float64 {
	if d < 0 {
		return -1
	}
	return float64(d.Microseconds()) / 1000
}

func entry(r netlog.Row) Entry {
	t := r.Timing()
	e := Entry{
		StartedDateTime: date(r.Started),
		Request:         request(r),
		Response:        response(r),
		Timings: Timings{Blocked: ms(t.Blocked), DNS: ms(t.DNS), Connect: ms(t.Connect), Send: ms(t.Send),
			Wait: ms(t.Wait), Receive: ms(t.Receive), SSL: ms(t.TLS)},
		Kind: string(r.Type), ResourceType: string(r.Type),
	}
	e.Time = ms(max(0, t.Blocked) + max(0, t.DNS) + max(0, t.Connect) + t.Send + t.Wait + t.Receive)
	if host, port, err := net.SplitHostPort(r.Remote); err == nil {
		e.ServerIPAddress, e.Connection = host, port
	}
	if r.Initiator != "" {
		e.Initiator = &Initiator{Type: "other", Name: r.Initiator}
		if r.Type == netlog.XHR {
			e.Initiator.Type = "script"
		}
	}
	switch {
	case r.Blocked:
		e.Blocked = r.Err
	case r.Err != "":
		e.Error = r.Err
	}
	return e
}

func request(r netlog.Row) Request {
	q := Request{Method: r.Method, URL: r.Address, HTTPVersion: r.Proto, Cookies: []Cookie{}, Headers: []Pair{},
		QueryString: []Pair{}, HeadersSize: -1, BodySize: r.SentSize}
	header := r.Request.Clone()
	u, err := url.Parse(r.Address)
	if err == nil {
		q.QueryString = fields(u.RawQuery)
		if header.Get("Host") == "" && u.Host != "" && !r.Blocked {
			q.Headers = append(q.Headers, Pair{"Host", u.Host})
		}
	}
	if r.SentSize > 0 && header.Get("Content-Length") == "" && header.Get("Transfer-Encoding") == "" {
		header.Set("Content-Length", strconv.FormatInt(r.SentSize, 10))
	}
	q.Headers = append(q.Headers, pairs(header)...)
	if cookies, bad := http.ParseCookie(strings.Join(header.Values("Cookie"), "; ")); bad == nil {
		for _, c := range cookies {
			q.Cookies = append(q.Cookies, Cookie{Name: c.Name, Value: c.Value})
		}
	}
	if r.SentSize > 0 {
		q.PostData = posted(r)
	}
	return q
}

// posted is what r carried, as much of it as its log kept.
func posted(r netlog.Row) *PostData {
	p := &PostData{MimeType: r.Request.Get("Content-Type")}
	media, _, _ := mime.ParseMediaType(p.MimeType)
	sent := r.Sent
	if r.SentCut {
		p.Truncated, p.Comment = true, cut(len(sent), r.SentSize)
		sent = uncut(sent)
	}
	if !utf8.Valid(sent) {
		p.Text, p.Encoding = base64.StdEncoding.EncodeToString(r.Sent), inBase64
		return p
	}
	p.Text = string(sent)
	if media == "application/x-www-form-urlencoded" {
		p.Params = fields(p.Text)
	}
	return p
}

// cut says how much of a body an archive holds.
func cut(kept int, of int64) string {
	if kept == 0 {
		return fmt.Sprintf("light kept none of the %d bytes of this body.", of)
	}
	return fmt.Sprintf("Cut: the first %d of the %d bytes of this body, all light kept.", kept, of)
}

// uncut is text without the bytes of a character its end was cut in.
func uncut(text []byte) []byte {
	for range utf8.UTFMax - 1 {
		if r, size := utf8.DecodeLastRune(text); r != utf8.RuneError || size != 1 {
			break
		}
		text = text[:len(text)-1]
	}
	return text
}

func response(r netlog.Row) Response {
	a := Response{Status: r.Status, StatusText: r.Reason, HTTPVersion: r.Proto, Cookies: []Cookie{},
		Headers: pairs(r.Response), RedirectURL: r.Response.Get("Location"), HeadersSize: -1, BodySize: -1,
		Content: content(r)}
	if r.Status != 0 {
		a.BodySize = r.Transferred
	}
	for _, line := range r.Response.Values("Set-Cookie") {
		c, err := http.ParseSetCookie(line)
		if err != nil {
			continue
		}
		set := Cookie{Name: c.Name, Value: c.Value, Path: c.Path, Domain: c.Domain, HTTPOnly: c.HttpOnly,
			Secure: c.Secure, SameSite: sameSite(c.SameSite)}
		if !c.Expires.IsZero() {
			set.Expires = date(c.Expires)
		}
		a.Cookies = append(a.Cookies, set)
	}
	return a
}

func sameSite(s http.SameSite) string {
	switch s {
	case http.SameSiteLaxMode:
		return "Lax"
	case http.SameSiteStrictMode:
		return "Strict"
	case http.SameSiteNoneMode:
		return "None"
	case http.SameSiteDefaultMode:
	}
	return ""
}

// content is r's body as its log kept it, its preview when no more is:
// text decoded by its charset for what is text, base64 for the rest and
// for a body still encoded.
func content(r netlog.Row) Content {
	c := Content{Size: r.Size, MimeType: r.Response.Get("Content-Type"), Compression: max(0, r.Size-r.Transferred)}
	if c.MimeType == "" {
		c.MimeType = "x-unknown"
	}
	if r.Size == 0 {
		return c
	}
	body := r.Body
	if body == nil {
		body = r.Preview
	}
	if int64(len(body)) < r.Size {
		c.Truncated, c.Comment = true, cut(len(body), r.Size)
	}
	if len(body) == 0 {
		return c
	}
	encoded := r.Response.Get("Content-Encoding")
	if r.Unzipped {
		encoded = ""
	}
	if text, ok := decoded(body, c.MimeType, c.Truncated); ok && encoded == "" {
		c.Text = text
		return c
	}
	c.Text, c.Encoding = base64.StdEncoding.EncodeToString(body), inBase64
	if encoded != "" {
		c.Comment = strings.TrimSpace(c.Comment + " The body is as the server sent it, encoded as " + encoded +
			": light does not undo that encoding.")
	}
	return c
}

// decoded is body as UTF-8 text when its type is one of text, or names
// none and it reads as text: by the charset the type names, else the one
// its first bytes show.
func decoded(body []byte, header string, cut bool) (string, bool) {
	media, _, _ := mime.ParseMediaType(header)
	if media != "" && !textual(media) {
		return "", false
	}
	if cut {
		body = uncut(body)
	}
	if utf8.Valid(body) && !strings.Contains(strings.ToLower(header), "charset") {
		return string(body), true
	}
	if media == "" {
		return "", false
	}
	read, err := charset.NewReader(bytes.NewReader(body), header)
	if err != nil {
		return "", false
	}
	text, err := io.ReadAll(read)
	if err != nil || !utf8.Valid(text) {
		return "", false
	}
	return string(text), true
}

func textual(media string) bool {
	switch {
	case strings.HasPrefix(media, "text/"), strings.HasSuffix(media, "+json"), strings.HasSuffix(media, "+xml"),
		strings.HasSuffix(media, "/json"), strings.HasSuffix(media, "/xml"), strings.HasSuffix(media, "javascript"),
		strings.HasSuffix(media, "ecmascript"), media == "application/x-www-form-urlencoded":
		return true
	}
	return false
}

// pairs are the headers by name, each value its own.
func pairs(header http.Header) []Pair {
	out := []Pair{}
	for _, name := range slices.Sorted(maps.Keys(header)) {
		for _, value := range header[name] {
			out = append(out, Pair{name, value})
		}
	}
	return out
}

// fields are the fields of a query or of a form as they are written, in
// their order.
func fields(encoded string) []Pair {
	out := []Pair{}
	for field := range strings.SplitSeq(encoded, "&") {
		if field == "" {
			continue
		}
		name, value, _ := strings.Cut(field, "=")
		out = append(out, Pair{unescaped(name), unescaped(value)})
	}
	return out
}

func unescaped(s string) string {
	if plain, err := url.QueryUnescape(s); err == nil {
		return plain
	}
	return s
}
