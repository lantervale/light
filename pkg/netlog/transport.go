package netlog

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptrace"
	"time"
)

type key struct{}

// asked is what a context carries for the Transport.
type asked struct {
	log       *Log
	initiator string
	wanted    Type
}

// With is ctx for requests recorded in l as asked by initiator, of the
// type wanted until their answer declares its own.
func With(ctx context.Context, l *Log, initiator string, wanted Type) context.Context {
	return context.WithValue(ctx, key{}, asked{l, initiator, wanted})
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
	start := time.Now()
	row := a.log.add(Row{
		Address: req.URL.String(), Method: req.Method, Type: a.wanted, Initiator: a.initiator,
		Started: start, Request: req.Header.Clone(),
	}, req.Response)
	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		remote := info.Conn.RemoteAddr().String()
		a.log.change(row, func(r *Row) { r.Remote = remote })
	}}
	resp, err := t.base().RoundTrip(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
	if err != nil {
		a.log.change(row, func(r *Row) { r.Err, r.Done, r.Took = reason(err), true, time.Since(start) })
		return nil, err
	}
	media, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	a.log.change(row, func(r *Row) {
		r.Status, r.Reason = resp.StatusCode, http.StatusText(resp.StatusCode)
		r.Media = media
		if resp.StatusCode < http.StatusBadRequest {
			r.Type = TypeOf(media, a.wanted)
		}
		r.Response, r.Took = resp.Header.Clone(), time.Since(start)
	})
	if resp.Header.Get("Location") != "" {
		a.log.redirected(row, resp)
	}
	resp.Body = &body{ReadCloser: resp.Body, log: a.log, row: row, start: start}
	return resp, nil
}

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
	log   *Log
	row   *Row
	start time.Time
	ended bool
}

func (b *body) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.log.change(b.row, func(r *Row) {
		r.Size += int64(n)
		keep := min(n, PreviewCap-len(r.Preview))
		r.Preview = append(r.Preview, p[:keep]...)
		r.Truncated = r.Truncated || keep < n
		r.Took = time.Since(b.start)
	})
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
		r.Done, r.Took = true, time.Since(b.start)
		if err != nil && !errors.Is(err, io.EOF) {
			r.Err = reason(err)
		}
	})
}
