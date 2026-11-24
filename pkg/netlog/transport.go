package netlog

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"

	"github.com/lantervale/light/pkg/instant"
)

type key struct{}

// asked is what a context carries for the Transport.
type asked struct {
	log       *Log
	initiator string
	wanted    Type
	trail     *Trail
}

// Trail is the rows of the requests made with one context, in the order
// they were asked.
type Trail struct {
	mu  sync.Mutex
	ids []int
}

// Last is the row of the last request made, 0 for none.
func (t *Trail) Last() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.ids) == 0 {
		return 0
	}
	return t.ids[len(t.ids)-1]
}

func (t *Trail) add(id int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ids = append(t.ids, id)
}

// Trailed is ctx with a Trail of the requests recorded with it.
func Trailed(ctx context.Context) (context.Context, *Trail) {
	a, _ := ctx.Value(key{}).(asked)
	a.trail = &Trail{}
	return context.WithValue(ctx, key{}, a), a.trail
}

// With is ctx for requests recorded in l as asked by initiator, of the
// type wanted until their answer declares its own.
func With(ctx context.Context, l *Log, initiator string, wanted Type) context.Context {
	a, _ := ctx.Value(key{}).(asked)
	return context.WithValue(ctx, key{}, asked{l, initiator, wanted, a.trail})
}

// As is ctx for requests recorded in the Log ctx already carries, if any,
// as asked by another initiator: a page's sheets, asked by its document.
func As(ctx context.Context, initiator string, wanted Type) context.Context {
	a, _ := ctx.Value(key{}).(asked)
	return With(ctx, a.log, initiator, wanted)
}

// cancelled is the Err of a request stopped on purpose.
const cancelled = "cancelled"

// Transport makes requests through Base, http.DefaultTransport when nil,
// and records each whose context carries a Log.
type Transport struct {
	Base http.RoundTripper
}

func (t *Transport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	a, ok := req.Context().Value(key{}).(asked)
	if !ok || a.log == nil {
		return t.base().RoundTrip(req)
	}
	if req.Method == http.MethodOptions && req.Header.Get("Access-Control-Request-Method") != "" {
		a.wanted = Preflight
	}
	start := instant.Now()
	row := a.log.add(Row{
		Address: req.URL.String(), Method: req.Method, Type: a.wanted, Initiator: a.initiator,
		Started: start, Request: req.Header.Clone(),
	}, req.Response)
	a.trail.add(row.ID)
	sent := req.WithContext(httptrace.WithClientTrace(req.Context(), a.log.traced(row)))
	a.log.carrying(sent, row)
	// As http.Transport does for gzip, the encodings light undoes are
	// asked for and undone here when the request asks for no encoding
	// of its own, so that the bytes sent are counted beside those read.
	unzip := req.Header.Get("Accept-Encoding") == "" && req.Header.Get("Range") == "" && req.Method != http.MethodHead
	if unzip {
		sent.Header = req.Header.Clone()
		sent.Header.Set("Accept-Encoding", Encodings(req.URL.Scheme))
		a.log.change(row, func(r *Row) { r.Request = sent.Header.Clone() })
	}
	resp, err := t.base().RoundTrip(sent)
	if err != nil {
		var refused interface{ Blocked() string }
		a.log.change(row, func(r *Row) {
			r.Err, r.Done, r.Took = reason(err), true, instant.Since(start)
			if errors.As(err, &refused) {
				r.Blocked, r.Err = true, refused.Blocked()
			}
		})
		return nil, err
	}
	media, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	a.log.change(row, func(r *Row) {
		r.Status, r.Reason, r.Proto = resp.StatusCode, http.StatusText(resp.StatusCode), resp.Proto
		r.marks.answered = instant.Now()
		r.Media = media
		if resp.StatusCode < http.StatusBadRequest {
			r.Type = TypeOf(media, a.wanted)
		}
		r.Response, r.Took = resp.Header.Clone(), instant.Since(start)
	})
	if resp.Header.Get("Location") != "" {
		a.log.redirected(row, resp)
	}
	wire := &counted{ReadCloser: resp.Body}
	resp.Body = wire
	if open := decoder(resp.Header.Get("Content-Encoding")); unzip && open != nil {
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.ContentLength, resp.Uncompressed = -1, true
		resp.Body = &unzipped{zipped: wire, open: open}
		a.log.change(row, func(r *Row) { r.Unzipped = true })
	}
	resp.Body = &body{ReadCloser: resp.Body, wire: wire, log: a.log, row: row, start: start}
	return resp, nil
}

// counted counts the bytes read through it: an answer's as the server
// sent them.
type counted struct {
	io.ReadCloser
	n int64
}

func (c *counted) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	c.n += int64(n)
	return n, err
}

// Encodings is the Accept-Encoding of a request of scheme that names
// none: the encodings the Transport undoes, Brotli only over TLS, as
// browsers ask, since a proxy in the clear may spoil it.
func Encodings(scheme string) string {
	if scheme == "https" {
		return "gzip, deflate, br"
	}
	return "gzip, deflate"
}

// decoder is what undoes an answer's Content-Encoding, nil for one that
// is none of those.
func decoder(encoding string) func(io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "gzip", "x-gzip":
		return func(r io.Reader) (io.Reader, error) { return gzip.NewReader(r) }
	case "deflate":
		return inflated
	case "br":
		return func(r io.Reader) (io.Reader, error) { return brotli.NewReader(r), nil }
	}
	return nil
}

// inflated undoes a deflate encoding: the zlib stream the standard
// means, or the bare deflate stream some servers send for it, told
// apart by the two bytes a zlib stream begins with.
func inflated(r io.Reader) (io.Reader, error) {
	start := bufio.NewReader(r)
	if head, err := start.Peek(2); err == nil && head[0]&0x0f == 8 && (uint16(head[0])<<8|uint16(head[1]))%31 == 0 {
		return zlib.NewReader(start)
	}
	return flate.NewReader(start), nil
}

// unzipped is an encoded body as it was before, read from its first
// Read.
type unzipped struct {
	zipped io.ReadCloser
	open   func(io.Reader) (io.Reader, error)
	plain  io.Reader
	err    error
}

func (u *unzipped) Read(p []byte) (int, error) {
	if u.plain == nil && u.err == nil {
		u.plain, u.err = u.open(u.zipped)
	}
	if u.err != nil {
		return 0, u.err
	}
	return u.plain.Read(p)
}

func (u *unzipped) Close() error { return u.zipped.Close() }

func reason(err error) string {
	if errors.Is(err, context.Canceled) {
		return cancelled
	}
	return err.Error()
}

// body counts an answer's bytes into its row as they are read, keeps the
// first of them, and ends the row with it.
type body struct {
	io.ReadCloser
	// wire counts the bytes as the server sent them.
	wire  *counted
	log   *Log
	row   *Row
	start time.Time
	ended bool
}

func (b *body) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.log.read(b.row, p[:n], b.wire.n, instant.Since(b.start))
	if err != nil {
		b.end(err)
	}
	return n, err
}

func (b *body) Close() error {
	b.end(nil)
	return b.ReadCloser.Close()
}

func (b *body) end(err error) {
	if b.ended {
		return
	}
	b.ended = true
	b.log.change(b.row, func(r *Row) {
		r.Done, r.Took = true, instant.Since(b.start)
		if err != nil && !errors.Is(err, io.EOF) {
			r.Err = reason(err)
		}
	})
}
