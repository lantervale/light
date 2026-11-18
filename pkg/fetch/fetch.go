// Package fetch is one way to make an HTTP GET as a browser makes it:
// redirects followed up to a cap and only to http and https, the body
// held up to a limit or read as it comes, its text decoded by its
// charset, the whole stopped by its context.
package fetch

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html/charset"
)

const (
	// MaxRedirects is how many redirects one navigation follows.
	MaxRedirects = 10
	// DefaultLimit is the largest body a Client holds unless told.
	DefaultLimit = 64 << 20
)

// ErrTooManyRedirects is a redirect past MaxRedirects, a loop included.
var ErrTooManyRedirects = errors.New("too many redirects")

// TooBigError is an answer whose body is over the client's limit.
type TooBigError struct{ Limit int64 }

func (e *TooBigError) Error() string {
	return fmt.Sprintf("it is over %d MB, more than is held in memory", e.Limit>>20)
}

// Answer is what a server sent, error statuses included.
type Answer struct {
	// Address is where the answer came from, after its redirects.
	Address string
	URL     *url.URL
	Status  int
	Header  http.Header
	Body    []byte
	// Hops is how many redirects led to it.
	Hops int
}

// OK reports whether the status is a success.
func (a *Answer) OK() bool { return ok(a.Status) }

func ok(status int) bool { return status >= http.StatusOK && status < http.StatusMultipleChoices }

// Text is the body decoded to UTF-8 by the charset its Content-Type
// names, else the one its first bytes show, as a browser sniffs it.
func (a *Answer) Text() string {
	r, err := charset.NewReader(bytes.NewReader(a.Body), a.Header.Get("Content-Type"))
	if err != nil {
		return string(a.Body)
	}
	text, err := io.ReadAll(r)
	if err != nil {
		return string(a.Body)
	}
	return string(text)
}

// Client makes the requests.
type Client struct {
	http      *http.Client
	agent     string
	limit     int64
	redirects int
}

// Option sets a Client up.
type Option func(*Client)

// Agent is the User-Agent requests are sent with.
func Agent(name string) Option { return func(c *Client) { c.agent = name } }

// Limit is the largest body held, DefaultLimit unless set.
func Limit(bytes int64) Option { return func(c *Client) { c.limit = bytes } }

// Redirects is how many redirects one navigation follows, MaxRedirects
// unless set.
func Redirects(most int) Option { return func(c *Client) { c.redirects = most } }

// New is a Client making its requests through transport,
// http.DefaultTransport when nil. It sets no timeout: a request lasts as
// long as its context.
func New(transport http.RoundTripper, opts ...Option) *Client {
	c := &Client{limit: DefaultLimit, redirects: MaxRedirects}
	for _, opt := range opts {
		opt(c)
	}
	c.http = &http.Client{Transport: transport, CheckRedirect: c.checkRedirect}
	return c
}

// MaxRedirects is how many redirects one navigation follows.
func (c *Client) MaxRedirects() int { return c.redirects }

type (
	beforeKey struct{}
	hopsKey   struct{}
)

// hops counts the redirects of one Get, from those before it.
type hops struct{ before, followed int }

// After is ctx for a Get that goes on a navigation already redirected
// hops times, a page's own refresh for one, so the cap counts them all.
func After(ctx context.Context, redirects int) context.Context {
	return context.WithValue(ctx, beforeKey{}, redirects)
}

func (c *Client) checkRedirect(req *http.Request, _ []*http.Request) error {
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return fmt.Errorf("it redirects to %s, and only http and https addresses are opened", req.URL)
	}
	h, _ := req.Context().Value(hopsKey{}).(*hops)
	if h.before+h.followed >= c.redirects {
		return ErrTooManyRedirects
	}
	h.followed++
	return nil
}

// Stream is an answer whose body is read as it comes: Read takes it,
// Close ends it.
type Stream struct {
	// Address is where the answer came from, after its redirects.
	Address string
	URL     *url.URL
	Status  int
	Header  http.Header
	// Length is how many bytes the server says the body has, -1 when it
	// does not say.
	Length int64
	// Hops is how many redirects led to it.
	Hops int

	body  io.Closer
	read  *bufio.Reader
	limit int64
}

// OK reports whether the status is a success.
func (s *Stream) OK() bool { return ok(s.Status) }

// Peek is up to the first n bytes not yet read, left to be read.
func (s *Stream) Peek(n int) []byte {
	start, _ := s.read.Peek(n)
	return start
}

func (s *Stream) Read(p []byte) (int, error) { return s.read.Read(p) }

// Close ends the answer, read whole or not.
func (s *Stream) Close() error { return s.body.Close() }

// Answer reads what is left of the stream whole, and closes it. A body
// over the client's limit is a TooBigError.
func (s *Stream) Answer() (*Answer, error) {
	defer s.Close()
	if s.Length > s.limit {
		return nil, &TooBigError{s.limit}
	}
	body, err := io.ReadAll(io.LimitReader(s, s.limit+1))
	if err != nil {
		return nil, plain(err)
	}
	if int64(len(body)) > s.limit {
		return nil, &TooBigError{s.limit}
	}
	return &Answer{Address: s.Address, URL: s.URL, Status: s.Status, Header: s.Header, Body: body, Hops: s.Hops}, nil
}

// peeked is how much of a stream Peek can show.
const peeked = 4 << 10

// Open asks for address, saying what it accepts, and gives the answer
// with its body still to read, whatever its size. An error status is a
// Stream; an error is no answer at all: a server not reached, a redirect
// refused, ctx ended.
func (c *Client) Open(ctx context.Context, address, accept string) (*Stream, error) {
	before, _ := ctx.Value(beforeKey{}).(int)
	count := &hops{before: before}
	req, err := http.NewRequestWithContext(context.WithValue(ctx, hopsKey{}, count), http.MethodGet, address, nil)
	if err != nil {
		return nil, plain(err)
	}
	if c.agent != "" {
		req.Header.Set("User-Agent", c.agent)
	}
	req.Header.Set("Accept", accept)
	resp, err := c.http.Do(req) //nolint:bodyclose // the Stream returned holds the body, and its Close closes it
	if err != nil {
		return nil, plain(err)
	}
	return &Stream{
		Address: resp.Request.URL.String(), URL: resp.Request.URL, Status: resp.StatusCode, Header: resp.Header,
		Length: resp.ContentLength, Hops: count.followed,
		body: resp.Body, read: bufio.NewReaderSize(resp.Body, peeked), limit: c.limit,
	}, nil
}

// Get asks for address, saying what it accepts, and reads the answer
// whole. An error status is an Answer; an error is no answer at all: a
// server not reached, a redirect refused, a body over the limit, ctx
// ended.
func (c *Client) Get(ctx context.Context, address, accept string) (*Answer, error) {
	s, err := c.Open(ctx, address, accept)
	if err != nil {
		return nil, err
	}
	return s.Answer()
}

// plain is err without the method and address http.Client wraps it in,
// which whoever shows it has already.
func plain(err error) error {
	var wrapped *url.Error
	if errors.As(err, &wrapped) {
		return wrapped.Err
	}
	return err
}

// Refresh reads a Refresh header or a meta refresh's content, "3" or
// "0; url=next.html": the seconds to wait and the address to go to then,
// resolved from base, base itself when none is named. It is false for
// what names no time.
func Refresh(value string, base *url.URL) (int, string, bool) {
	wait, target, _ := strings.Cut(value, ";")
	if !strings.Contains(value, ";") {
		wait, target, _ = strings.Cut(value, ",")
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(wait))
	if err != nil || seconds < 0 {
		return 0, "", false
	}
	target = strings.TrimSpace(target)
	if len(target) >= 4 && strings.EqualFold(target[:3], "url") {
		if rest := strings.TrimSpace(target[3:]); strings.HasPrefix(rest, "=") {
			target = strings.TrimSpace(rest[1:])
		}
	}
	target = strings.Trim(target, `"'`)
	u, err := base.Parse(target)
	if err != nil {
		return 0, "", false
	}
	return seconds, u.String(), true
}
