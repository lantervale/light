package fetch

import (
	"errors"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// Mode is how far from its page a script's request may go.
type Mode string

const (
	// ModeCORS reads another origin's answer only when that origin shares
	// it: what fetch and XMLHttpRequest do unless told.
	ModeCORS Mode = "cors"
	// ModeSameOrigin asks the page's own origin only.
	ModeSameOrigin Mode = "same-origin"
	// ModeNoCORS sends another origin a simple request, and reads nothing
	// of its answer.
	ModeNoCORS Mode = "no-cors"
)

// Credentials is when a script's request carries cookies and keeps
// those its answer sets.
type Credentials string

const (
	CredentialsSameOrigin Credentials = "same-origin"
	CredentialsOmit       Credentials = "omit"
	CredentialsInclude    Credentials = "include"
)

// Kind is what a script sees of an answer, a Response's type.
type Kind string

const (
	// Basic is an answer of the page's own origin: all of it but its
	// cookies.
	Basic Kind = "basic"
	// Shared is another origin's answer it shares: its body, and the
	// headers it exposes.
	Shared Kind = "cors"
	// Opaque is another origin's answer to a no-cors request: nothing.
	Opaque Kind = "opaque"
	// OpaqueRedirect is a redirect a request that follows none by hand
	// was answered.
	OpaqueRedirect Kind = "opaqueredirect"
)

// BlockedError is a request of a script that was not sent, or whose
// answer was kept from the script: Error is why.
type BlockedError struct {
	// Address is what was asked for when it was blocked, and From what
	// redirected there, "" for none.
	Address, From string
	// Origin is the origin that asked and Page its address.
	Origin, Page string
	Why          string
	// Sent is whether the server got the request all the same.
	Sent bool
	// CORS is an answer its origin does not share, Mixed a request in
	// the clear of a page that is not.
	CORS, Mixed bool
}

func (b *BlockedError) Error() string { return b.Why }

// Console is the line a browser's console says of it, for what asked:
// "fetch" or "XMLHttpRequest".
func (b *BlockedError) Console(api string) string {
	switch {
	case b.Mixed:
		return "Mixed Content: The page at '" + b.Page + "' was loaded over HTTPS, but requested an insecure resource '" +
			b.Address + "'. This request has been blocked; the content must be served over HTTPS."
	case b.CORS:
		at := "'" + b.Address + "'"
		if b.From != "" {
			at += " (redirected from '" + b.From + "')"
		}
		return "Access to " + api + " at " + at + " from origin '" + b.Origin + "' has been blocked by CORS policy: " +
			b.Why
	case api == "fetch":
		return "Fetch API cannot load " + b.Address + ". " + b.Why
	}
	return api + " cannot load " + b.Address + ". " + b.Why
}

func unshared(why string) *BlockedError { return &BlockedError{Why: why, CORS: true, Sent: true} }

// nullOrigin is the origin of a page that has none to name.
const nullOrigin = "null"

const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// web reports whether u is an address of the web: an http or an https
// one.
func web(u *url.URL) bool { return u.Scheme == schemeHTTP || u.Scheme == schemeHTTPS }

// OriginOf is the origin of an address as an Origin header names it:
// its scheme, its host and its port when it is not the scheme's own,
// "null" for an address that is not of the web.
func OriginOf(u *url.URL) string {
	if u == nil || !web(u) || u.Hostname() == "" {
		return nullOrigin
	}
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port == "" || (u.Scheme == schemeHTTP && port == "80") || (u.Scheme == schemeHTTPS && port == "443") {
		return u.Scheme + "://" + host
	}
	return u.Scheme + "://" + host + ":" + port
}

// Policy is who asks and how: the origin of the page (OriginOf), and
// the mode and the credentials of its request.
type Policy struct {
	Origin      string
	Mode        Mode
	Credentials Credentials
}

// Same reports whether to is of the page's own origin. A page of no
// origin has none that is its own.
func (p Policy) Same(to *url.URL) bool { return p.Origin != nullOrigin && OriginOf(to) == p.Origin }

// Credentialed reports whether a request to to carries cookies and
// keeps those its answer sets; crossed is whether it, or what
// redirected to it, went to another origin.
func (p Policy) Credentialed(to *url.URL, crossed bool) bool {
	switch p.Credentials {
	case CredentialsInclude:
		return true
	case CredentialsSameOrigin:
		return !crossed && p.Same(to)
	case CredentialsOmit:
	}
	return false
}

// Decide is what a script of the page sees of the answer a request to
// to got, by the headers of that answer: an error is a *BlockedError,
// the answer kept from the script. crossed is whether a redirect that
// led to it went to another origin.
func (p Policy) Decide(to *url.URL, crossed bool, answer http.Header) (Kind, error) {
	if !crossed && p.Same(to) {
		return Basic, nil
	}
	switch p.Mode {
	case ModeNoCORS:
		return Opaque, nil
	case ModeCORS:
		if err := p.Read(answer); err != nil {
			return "", err
		}
		return Shared, nil
	case ModeSameOrigin:
	}
	return "", &BlockedError{Why: `Request mode is "same-origin" but the URL's origin is not same as the request ` +
		`initiator origin.`}
}

// Read is the CORS check: whether another origin's answer says the page
// may read it. A page of no origin reads only what is shared with all.
func (p Policy) Read(answer http.Header) error {
	const name = "Access-Control-Allow-Origin"
	allowed, include := answer.Values(name), p.Credentials == CredentialsInclude
	switch {
	case len(allowed) == 0:
		return unshared("No '" + name + "' header is present on the requested resource.")
	case len(allowed) > 1 || strings.Contains(allowed[0], ","):
		return unshared("The '" + name + "' header contains multiple values '" + strings.Join(allowed, ", ") +
			"', but only one is allowed.")
	case allowed[0] == "*" && !include:
		return nil
	case allowed[0] == "*":
		return unshared("The value of the '" + name + "' header in the response must not be the wildcard '*' when " +
			"the request's credentials mode is 'include'.")
	case allowed[0] != p.Origin || p.Origin == nullOrigin:
		return unshared("The '" + name + "' header has a value '" + allowed[0] +
			"' that is not equal to the supplied origin.")
	case !include:
		return nil
	}
	if with := answer.Get("Access-Control-Allow-Credentials"); with != "true" {
		return unshared("The value of the 'Access-Control-Allow-Credentials' header in the response is '" + with +
			"' which must be 'true' when the request's credentials mode is 'include'.")
	}
	return nil
}

// Preflighted reports whether a request to another origin is asked for
// first: one whose method or headers are not those a form could send.
func (p Policy) Preflighted(method string, header http.Header) bool {
	return p.Mode == ModeCORS && (!Safelisted(method) || len(Unsafe(header)) > 0)
}

// Preflight is the headers of the OPTIONS request that asks another
// origin whether it takes a request of that method and those headers.
func (p Policy) Preflight(method string, header http.Header) http.Header {
	asks := http.Header{"Accept": {"*/*"}, "Access-Control-Request-Method": {method}, "Origin": {p.Origin}}
	if names := Unsafe(header); len(names) > 0 {
		asks.Set("Access-Control-Request-Headers", strings.Join(names, ","))
	}
	return asks
}

// Allows reports whether the answer to a preflight, its status and its
// headers, lets the request of that method and those headers be sent:
// an error is a *BlockedError.
func (p Policy) Allows(status int, answer http.Header, method string, header http.Header) error {
	const prefix = "Response to preflight request doesn't pass access control check: "
	if status >= http.StatusMultipleChoices && status < http.StatusBadRequest {
		return unshared(prefix + "Redirect is not allowed for a preflight request.")
	}
	if err := p.Read(answer); err != nil {
		var blocked *BlockedError
		if errors.As(err, &blocked) {
			return unshared(prefix + blocked.Why)
		}
		return err
	}
	if !ok(status) {
		return unshared(prefix + "It does not have HTTP ok status.")
	}
	include := p.Credentials == CredentialsInclude
	methods, names := list(answer, "Access-Control-Allow-Methods"), list(answer, "Access-Control-Allow-Headers")
	every := func(of []string) bool { return !include && slices.Contains(of, "*") }
	if !Safelisted(method) && !slices.Contains(methods, method) && !every(methods) {
		return unshared("Method " + method + " is not allowed by Access-Control-Allow-Methods in preflight response.")
	}
	for i := range names {
		names[i] = strings.ToLower(names[i])
	}
	for _, name := range Unsafe(header) {
		if !slices.Contains(names, name) && (name == "authorization" || !every(names)) {
			return unshared("Request header field " + name +
				" is not allowed by Access-Control-Allow-Headers in preflight response.")
		}
	}
	return nil
}

// list is the values of a header that lists them, each on its own.
func list(header http.Header, name string) []string {
	var out []string
	for _, line := range header.Values(name) {
		for _, value := range strings.Split(line, ",") {
			if value = strings.TrimSpace(value); value != "" {
				out = append(out, value)
			}
		}
	}
	return out
}

// Visible is what a script reads of an answer's headers: all of its
// own origin's but the cookies, of another's those every answer may
// show and those it exposes, of an opaque one none.
func (p Policy) Visible(kind Kind, answer http.Header) http.Header {
	out := http.Header{}
	if kind != Basic && kind != Shared {
		return out
	}
	exposed := list(answer, "Access-Control-Expose-Headers")
	all := kind == Basic || (p.Credentials != CredentialsInclude && slices.Contains(exposed, "*"))
	for i := range exposed {
		exposed[i] = strings.ToLower(exposed[i])
	}
	for name, values := range answer {
		switch lower := strings.ToLower(name); {
		case lower == "set-cookie" || lower == "set-cookie2":
		case all || slices.Contains(shown(), lower) || slices.Contains(exposed, lower):
			out[name] = slices.Clone(values)
		}
	}
	return out
}

// shown are the headers of another origin's answer a script reads
// without their being exposed.
func shown() []string {
	return []string{"cache-control", "content-language", "content-length", "content-type", "expires", "last-modified",
		"pragma"}
}

// Mixed reports whether a request to to is one in the clear asked by a
// page that is not: an https page's to an http address, but this
// machine's.
func Mixed(origin string, to *url.URL) bool {
	if !strings.HasPrefix(origin, schemeHTTPS+"://") || to.Scheme != schemeHTTP {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(to.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}

// ErrMethod is a method no script's request is sent with.
var ErrMethod = errors.New("the method is not one a script may send")

// Method is a method as a script's request is sent with it: the common
// ones in capitals whatever their case, the others as written. It is
// ErrMethod for CONNECT, TRACE and TRACK, and for what is no method.
func Method(name string) (string, error) {
	if name == "" || strings.ContainsFunc(name, func(r rune) bool { return !token(r) }) {
		return "", ErrMethod
	}
	switch upper := strings.ToUpper(name); upper {
	case "CONNECT", "TRACE", "TRACK":
		return "", ErrMethod
	case http.MethodDelete, http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPost, http.MethodPut:
		return upper, nil
	}
	return name, nil
}

func token(r rune) bool {
	return r > 0x20 && r < 0x7f && !strings.ContainsRune(`"(),/:;<=>?@[\]{}`, r)
}

// Forbidden reports whether a header is the browser's to set, never a
// script's: one a script names is dropped.
func Forbidden(name, value string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "accept-charset", "accept-encoding", "access-control-request-headers", "access-control-request-method",
		"connection", "content-length", "cookie", "cookie2", "date", "dnt", "expect", "host", "keep-alive", "origin",
		"referer", "set-cookie", "te", "trailer", "transfer-encoding", "upgrade", "via":
		return true
	case "x-http-method", "x-http-method-override", "x-method-override":
		for _, method := range strings.Split(value, ",") {
			if _, err := Method(strings.TrimSpace(method)); err != nil && strings.TrimSpace(method) != "" {
				return true
			}
		}
		return false
	}
	return name == "" || strings.HasPrefix(lower, "proxy-") || strings.HasPrefix(lower, "sec-") ||
		strings.ContainsFunc(name, func(r rune) bool { return !token(r) })
}

// Safelisted reports whether method is one another origin is sent
// without being asked first.
func Safelisted(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodPost
}

// Unsafe are the headers of a request another origin must allow before
// it is sent, in small letters and sorted: those that are not, name and
// value, what a form could send.
func Unsafe(header http.Header) []string {
	var names []string
	for name, values := range header {
		if lower := strings.ToLower(name); !simple(lower, strings.Join(values, ", ")) {
			names = append(names, lower)
		}
	}
	slices.Sort(names)
	return names
}

// maxSimpleValue is the longest value of a header that needs no leave.
const maxSimpleValue = 128

// simple reports whether a header, its name in small letters, is one a
// request to another origin carries without leave.
func simple(name, value string) bool {
	if len(value) > maxSimpleValue {
		return false
	}
	switch name {
	case "accept":
		return !strings.ContainsFunc(value, unsafeByte)
	case "accept-language", "content-language":
		return !strings.ContainsFunc(value, func(r rune) bool {
			return (r < '0' || r > '9') && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && !strings.ContainsRune(" *,-.;=", r)
		})
	case "content-type":
		if strings.ContainsFunc(value, unsafeByte) {
			return false
		}
		media, _, err := mime.ParseMediaType(value)
		return err == nil && (media == "application/x-www-form-urlencoded" || media == "multipart/form-data" ||
			media == "text/plain")
	case "range":
		from, to, cut := strings.Cut(strings.TrimPrefix(value, "bytes="), "-")
		return strings.HasPrefix(value, "bytes=") && cut && from != "" && digits(from) && digits(to)
	}
	return false
}

func unsafeByte(r rune) bool {
	return (r < 0x20 && r != '\t') || r == 0x7f || strings.ContainsRune(`"():<>?@[\]{}`, r)
}

func digits(text string) bool {
	return !strings.ContainsFunc(text, func(r rune) bool { return r < '0' || r > '9' })
}

// Plain is the headers of a no-cors request: those of header a form
// could send, the rest left out.
func Plain(header http.Header) http.Header {
	out := http.Header{}
	for name, values := range header {
		if lower := strings.ToLower(name); lower != "range" && simple(lower, strings.Join(values, ", ")) {
			out[name] = slices.Clone(values)
		}
	}
	return out
}
