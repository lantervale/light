package fetch

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lantervale/light/pkg/cookies"
	"github.com/lantervale/light/pkg/identity"
	"github.com/lantervale/light/pkg/localnet"
)

// Redirect is what a script's request does of a redirect.
type Redirect string

const (
	RedirectFollow Redirect = "follow"
	// RedirectError takes a redirect for a failure.
	RedirectError Redirect = "error"
	// RedirectManual stops at a redirect, of which the script sees
	// nothing (OpaqueRedirect).
	RedirectManual Redirect = "manual"
)

// Call is a request a page's script makes, through fetch,
// XMLHttpRequest or sendBeacon.
type Call struct {
	Method, Address string
	Header          http.Header
	Body            []byte
	// Page is the address of the document whose script asks. Opaque is
	// one whose origin nothing shares, a sandboxed frame's: it asks as
	// "null". Top is the page of the tab for a frame's document, nil
	// for the page itself: a request in the clear is refused when either
	// came over https.
	Page   *url.URL
	Opaque bool
	Top    *url.URL
	// Mode is ModeCORS, Credentials CredentialsSameOrigin and Redirect
	// RedirectFollow when they are not said.
	Mode        Mode
	Credentials Credentials
	Redirect    Redirect
}

// Result is what a script sees of the answer to its Call: of an Opaque
// one and of an OpaqueRedirect, nothing.
type Result struct {
	Answer
	Kind Kind
	// Redirected is whether the answer is of another address than the
	// one asked for.
	Redirected bool
}

const (
	// preflightAge is how long what a preflight allowed holds when the
	// answer does not say, and maxPreflightAge the longest it may.
	preflightAge    = 5 * time.Second
	maxPreflightAge = 10 * time.Minute
)

// leave is what a preflight allowed: its answer's headers, until then.
type leave struct {
	answer http.Header
	until  time.Time
}

// asking is whose leave it is: an origin's to ask an address, with its
// credentials or not.
type asking struct {
	origin, address string
	include         bool
}

// sending is a Call on its way, a redirect after the other.
type sending struct {
	c        *Client
	policy   Policy
	page     *url.URL
	top      *url.URL
	method   string
	header   http.Header
	body     []byte
	redirect Redirect
	first    string
	to       *url.URL
	// crossed is whether the request went to another origin, and
	// tainted whether it went from one to another still: it is then of
	// no origin.
	crossed, tainted bool
	hops             int
	last             *http.Response
}

// Script makes the request of a page's script as a browser does: what
// the same-origin policy lets it send, another origin asked first when
// the request is not one a form could make, the cookies its credentials
// say, each redirect checked on its own, and of the answer what its
// origin shares with the page. An error that is a *BlockedError is a request
// not sent, or an answer kept from the script; any other is no answer
// at all.
func (c *Client) Script(ctx context.Context, call Call) (*Result, error) {
	s, err := c.calling(call)
	if err != nil {
		return nil, err
	}
	before, _ := ctx.Value(beforeKey{}).(int)
	for ; ; s.hops++ {
		result, next, failed := s.hop(ctx)
		if failed != nil || result != nil {
			return result, failed
		}
		if before+s.hops >= c.redirects {
			return nil, ErrTooManyRedirects
		}
		if failed = s.follow(next); failed != nil {
			return nil, failed
		}
	}
}

// calling is a Call ready to be sent, or why it is not: a *BlockedError.
func (c *Client) calling(call Call) (*sending, error) {
	s := &sending{c: c, page: call.Page, top: call.Top, redirect: call.Redirect, first: call.Address,
		header: http.Header{},
		policy: Policy{Origin: OriginOf(call.Page), Mode: call.Mode, Credentials: call.Credentials}}
	if call.Opaque {
		s.policy.Origin = nullOrigin
	}
	if s.policy.Mode == "" {
		s.policy.Mode = ModeCORS
	}
	if s.policy.Credentials == "" {
		s.policy.Credentials = CredentialsSameOrigin
	}
	if s.redirect == "" {
		s.redirect = RedirectFollow
	}
	method, err := Method(call.Method)
	to, parseErr := url.Parse(call.Address)
	switch {
	case err != nil:
		return nil, s.unsent(call.Address, "'"+call.Method+"' HTTP method is unsupported.")
	case parseErr != nil || !web(to) || to.Hostname() == "":
		return nil, s.unsent(call.Address, `URL scheme must be "http" or "https" for CORS request.`)
	case to.User != nil:
		return nil, s.unsent(call.Address, "Request cannot be constructed from a URL that includes credentials.")
	case !s.known():
		return nil, s.unsent(call.Address, "The request's mode, credentials or redirect is not one light knows.")
	case s.policy.Mode == ModeNoCORS && !Safelisted(method):
		return nil, s.unsent(call.Address, "'"+method+"' is unsupported in no-cors mode.")
	}
	for name, values := range call.Header {
		if value := strings.Join(values, ", "); !Forbidden(name, value) {
			s.header.Set(name, value)
		}
	}
	if s.policy.Mode == ModeNoCORS {
		s.header = Plain(s.header)
	}
	s.method, s.to = method, to
	if method != http.MethodGet && method != http.MethodHead {
		s.body = call.Body
	}
	return s, nil
}

func (s *sending) known() bool {
	switch s.policy.Mode {
	case ModeCORS, ModeSameOrigin, ModeNoCORS:
	default:
		return false
	}
	switch s.policy.Credentials {
	case CredentialsSameOrigin, CredentialsOmit, CredentialsInclude:
	default:
		return false
	}
	switch s.redirect {
	case RedirectFollow, RedirectError, RedirectManual:
		return true
	}
	return false
}

// unsent is a request to address light did not send, and why.
func (s *sending) unsent(address, why string) *BlockedError {
	b := &BlockedError{Address: address, Why: why}
	return s.told(b)
}

// told is b saying who was blocked, and where.
func (s *sending) told(b *BlockedError) *BlockedError {
	b.Origin = s.policy.Origin
	if s.page != nil {
		b.Page = s.page.String()
	}
	if b.Address == "" {
		b.Address = s.to.String()
	}
	if s.first != b.Address {
		b.From = s.first
	}
	return b
}

// kept is err as the script's request was blocked by it, when it was.
func (s *sending) kept(err error) error {
	var blocked *BlockedError
	if errors.As(err, &blocked) {
		return s.told(blocked)
	}
	return err
}

// hop sends the request where it now goes. It ends with what the script
// sees, or with why it sees nothing; neither is a redirect to follow,
// to next.
func (s *sending) hop(ctx context.Context) (*Result, *url.URL, error) {
	p, err := s.allowed()
	if err != nil {
		return nil, nil, err
	}
	if s.crossed && p.Preflighted(s.method, s.header) {
		if err = s.preflight(ctx, p); err != nil {
			return nil, nil, s.kept(err)
		}
	}
	resp, err := s.send(ctx, p, s.method, s.header, s.body)
	if err != nil {
		return nil, nil, err
	}
	kind, err := p.Decide(s.to, s.crossed, resp.Header)
	if err != nil {
		_ = resp.Body.Close()
		return nil, nil, s.kept(err)
	}
	if location := resp.Header.Get("Location"); location != "" && redirects(resp.StatusCode) {
		_ = resp.Body.Close()
		switch s.redirect {
		case RedirectError:
			return nil, nil, s.told(&BlockedError{Sent: true, Why: `The request's redirect mode is "error", and it was ` +
				`redirected.`})
		case RedirectManual:
			return &Result{Kind: OpaqueRedirect}, nil, nil
		case RedirectFollow:
		}
		next, parseErr := s.to.Parse(location)
		if parseErr != nil {
			return nil, nil, errors.New("it redirects to an address that is none")
		}
		s.last = resp
		return nil, next, nil
	}
	result, err := s.read(p, kind, resp)
	return result, nil, err
}

func redirects(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect:
		return true
	}
	return false
}

// allowed is the policy the request goes by where it now goes, or why
// it does not go there: before anything is sent.
func (s *sending) allowed() (Policy, error) {
	p := s.policy
	if Mixed(p.Origin, s.to) || Mixed(OriginOf(s.top), s.to) {
		return p, s.told(&BlockedError{Mixed: true, Why: "Mixed Content: an https page may not ask for an http address."})
	}
	if s.crossed = s.crossed || !p.Same(s.to); s.crossed {
		switch {
		case p.Mode == ModeSameOrigin:
			_, err := p.Decide(s.to, true, nil)
			return p, s.kept(err)
		case p.Mode == ModeNoCORS && s.redirect != RedirectFollow:
			return p, s.unsent("", `A no-cors request to another origin follows its redirects: its redirect mode `+
				`must be "follow".`)
		}
	}
	if s.tainted {
		p.Origin = nullOrigin
	}
	return p, nil
}

// send makes one request where the script's now goes, with what the
// browser says of it: who asks, where from, and the cookies its
// credentials allow.
func (s *sending) send(ctx context.Context, p Policy, method string, header http.Header, body []byte,
) (*http.Response, error) {
	preflight := method == http.MethodOptions && header.Get("Access-Control-Request-Method") != ""
	if preflight || !p.Credentialed(s.to, s.crossed) {
		ctx = cookies.Without(ctx)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.to.String(), bytes.NewReader(body))
	if err != nil {
		return nil, plain(err)
	}
	req.Header = header.Clone()
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "*/*")
	}
	if s.c.agent != "" && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", s.c.agent)
	}
	if (s.crossed && p.Mode == ModeCORS) || (method != http.MethodGet && method != http.MethodHead) {
		req.Header.Set("Origin", p.Origin)
	}
	if from := identity.Referrer(s.page, s.to); from != "" {
		req.Header.Set("Referer", from)
	}
	s.c.present(req, p.Mode)
	req.Response = s.last
	resp, err := s.c.direct.Do(req)
	var refused *localnet.BlockedError
	switch {
	case errors.As(err, &refused):
		return nil, s.told(&BlockedError{Sent: true, CORS: true, Why: refused.Blocked()})
	case err != nil:
		return nil, plain(err)
	}
	return resp, nil
}

// preflight asks the origin the request now goes to whether it takes
// it, unless it said so lately.
func (s *sending) preflight(ctx context.Context, p Policy) error {
	who := asking{p.Origin, s.to.String(), p.Credentials == CredentialsInclude}
	if s.c.granted(who, p, s.method, s.header) {
		return nil
	}
	resp, err := s.send(ctx, p, http.MethodOptions, p.Preflight(s.method, s.header), nil)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if err = p.Allows(resp.StatusCode, resp.Header, s.method, s.header); err != nil {
		return err
	}
	age := preflightAge
	if seconds, unsaid := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Access-Control-Max-Age"))); unsaid == nil {
		age = min(time.Duration(max(seconds, 0))*time.Second, maxPreflightAge)
	}
	s.c.grant(who, resp.Header, age)
	return nil
}

// granted reports whether a preflight not yet stale allowed who a
// request of that method and those headers.
func (c *Client) granted(who asking, p Policy, method string, header http.Header) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	given, found := c.leaves[who]
	if !found {
		return false
	}
	if !time.Now().Before(given.until) {
		delete(c.leaves, who)
		return false
	}
	return p.Allows(http.StatusOK, given.answer, method, header) == nil
}

func (c *Client) grant(who asking, answer http.Header, age time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if age <= 0 {
		delete(c.leaves, who)
		return
	}
	if len(c.leaves) >= maxLeaves {
		clear(c.leaves)
	}
	c.leaves[who] = leave{answer: answer.Clone(), until: time.Now().Add(age)}
}

// maxLeaves is how many preflights' answers are kept.
const maxLeaves = 256

// follow has the request go on to next, where it was redirected, as a
// browser sends it on: a POST as a GET after a 301 or a 302, anything
// but a HEAD as a GET after a 303, and to another origin without what
// authorised it.
func (s *sending) follow(next *url.URL) error {
	switch {
	case !web(next):
		return errors.New("it redirects to " + next.String() + ", and only http and https addresses are fetched")
	case next.User != nil && s.policy.Mode == ModeCORS && (s.crossed || !s.policy.Same(next)):
		return s.told(&BlockedError{Address: next.String(), CORS: true, Why: "Redirect location '" + next.Redacted() +
			"' contains a username and password, which is disallowed for cross-origin requests."})
	}
	if next.Fragment == "" {
		next.Fragment, next.RawFragment = s.to.Fragment, s.to.RawFragment
	}
	status := s.last.StatusCode
	post := s.method == http.MethodPost && (status == http.StatusMovedPermanently || status == http.StatusFound)
	if other := status == http.StatusSeeOther && s.method != http.MethodGet && s.method != http.MethodHead; post || other {
		s.method, s.body = http.MethodGet, nil
		for _, name := range []string{"Content-Encoding", "Content-Language", "Content-Location", "Content-Type"} {
			s.header.Del(name)
		}
	}
	if !sameOrigin(s.to, next) {
		s.header.Del("Authorization")
		s.tainted = s.tainted || !s.policy.Same(s.to)
	}
	s.to = next
	return nil
}

// read is the answer as the script sees it, its body read whole.
func (s *sending) read(p Policy, kind Kind, resp *http.Response) (*Result, error) {
	redirected := s.last != nil
	if kind == Opaque {
		_ = resp.Body.Close()
		return &Result{Kind: Opaque}, nil
	}
	stream := &Stream{
		Address: s.to.String(), URL: s.to, Status: resp.StatusCode, Header: resp.Header,
		Length: resp.ContentLength, Hops: s.hops,
		body: resp.Body, read: bufio.NewReaderSize(resp.Body, peeked), limit: s.c.limit,
	}
	answer, err := stream.Answer()
	if err != nil {
		return nil, err
	}
	answer.Header = p.Visible(kind, answer.Header)
	return &Result{Answer: *answer, Kind: kind, Redirected: redirected}, nil
}
