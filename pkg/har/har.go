// Package har writes the requests of a netlog.Log as an HTTP Archive, the
// HAR 1.2 file browsers' network panels save and import: a page for each
// navigation and an entry for each request, with what was sent, what was
// answered, the bodies the log kept and how long each step took. What
// HAR has no field for is in fields that begin with "_". It is plain Go.
package har

import (
	"encoding/json"
	"io"
	"strings"
)

// Version is the version of the format written.
const Version = "1.2"

// Redacted is what Sanitize leaves in place of a secret.
const Redacted = "[redacted]"

// inBase64 is the encoding of a body that is no text.
const inBase64 = "base64"

// Archive is a HAR file.
type Archive struct {
	Log Log `json:"log"`
}

type Log struct {
	Version string   `json:"version"`
	Creator Creator  `json:"creator"`
	Browser *Creator `json:"browser,omitempty"`
	Pages   []Page   `json:"pages"`
	Entries []Entry  `json:"entries"`
	Comment string   `json:"comment,omitempty"`
}

// Creator names the program that wrote the file, or the browser that
// made its requests.
type Creator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Page is one navigation: the entries that name its ID were asked for
// from it on.
type Page struct {
	StartedDateTime string      `json:"startedDateTime"`
	ID              string      `json:"id"`
	Title           string      `json:"title"`
	PageTimings     PageTimings `json:"pageTimings"`
}

// PageTimings are the milliseconds a page took to load, -1 for unknown.
type PageTimings struct {
	OnContentLoad float64 `json:"onContentLoad"`
	OnLoad        float64 `json:"onLoad"`
	Comment       string  `json:"comment,omitempty"`
}

// Entry is one request and its answer.
type Entry struct {
	Pageref         string   `json:"pageref,omitempty"`
	StartedDateTime string   `json:"startedDateTime"`
	Time            float64  `json:"time"`
	Request         Request  `json:"request"`
	Response        Response `json:"response"`
	Cache           struct{} `json:"cache"`
	Timings         Timings  `json:"timings"`
	ServerIPAddress string   `json:"serverIPAddress,omitempty"`
	Connection      string   `json:"connection,omitempty"`
	// Kind is the request's netlog.Type, ResourceType the same under the
	// name Chrome reads, Initiator who asked, Blocked why light refused
	// to send it and Error why it got no answer.
	Kind         string     `json:"_kind,omitempty"`
	ResourceType string     `json:"_resourceType,omitempty"`
	Initiator    *Initiator `json:"_initiator,omitempty"`
	Blocked      string     `json:"_blocked,omitempty"`
	Error        string     `json:"_error,omitempty"`
}

// Initiator is who asked for an entry: Type as Chrome names the kinds
// ("script", "other"), Name as light's panel says it.
type Initiator struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type Request struct {
	Method      string    `json:"method"`
	URL         string    `json:"url"`
	HTTPVersion string    `json:"httpVersion"`
	Cookies     []Cookie  `json:"cookies"`
	Headers     []Pair    `json:"headers"`
	QueryString []Pair    `json:"queryString"`
	PostData    *PostData `json:"postData,omitempty"`
	HeadersSize int64     `json:"headersSize"`
	BodySize    int64     `json:"bodySize"`
}

type Response struct {
	Status      int      `json:"status"`
	StatusText  string   `json:"statusText"`
	HTTPVersion string   `json:"httpVersion"`
	Cookies     []Cookie `json:"cookies"`
	Headers     []Pair   `json:"headers"`
	Content     Content  `json:"content"`
	RedirectURL string   `json:"redirectURL"`
	HeadersSize int64    `json:"headersSize"`
	BodySize    int64    `json:"bodySize"`
}

// Pair is a header, a part of a query or a field of a form.
type Pair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Expires  string `json:"expires,omitempty"`
	HTTPOnly bool   `json:"httpOnly,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	SameSite string `json:"_sameSite,omitempty"`
}

// PostData is what a request carried: Text as it was sent, and Params
// its fields when it is a form's. Encoding is "base64" for what is no
// text, and Truncated says Text is not all of it, Comment why.
type PostData struct {
	MimeType  string `json:"mimeType"`
	Params    []Pair `json:"params,omitempty"`
	Text      string `json:"text"`
	Encoding  string `json:"_encoding,omitempty"`
	Truncated bool   `json:"_truncated,omitempty"`
	Comment   string `json:"comment,omitempty"`
}

// Content is an answer's body: Size its bytes once any encoding is
// undone, Text those bytes as text, or in base64 when Encoding says so.
// Truncated says Text is not all of it, Comment why.
type Content struct {
	Size        int64  `json:"size"`
	Compression int64  `json:"compression,omitempty"`
	MimeType    string `json:"mimeType"`
	Text        string `json:"text,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
	Truncated   bool   `json:"_truncated,omitempty"`
	Comment     string `json:"comment,omitempty"`
}

// Timings are the milliseconds of each step, -1 for one not taken or not
// known. Connect includes SSL.
type Timings struct {
	Blocked float64 `json:"blocked"`
	DNS     float64 `json:"dns"`
	Connect float64 `json:"connect"`
	Send    float64 `json:"send"`
	Wait    float64 `json:"wait"`
	Receive float64 `json:"receive"`
	SSL     float64 `json:"ssl"`
}

// Encode writes a as a HAR file.
func Encode(w io.Writer, a *Archive) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(a)
}

// secret reports a header whose value is a credential.
func secret(name string) bool {
	switch strings.ToLower(name) {
	case "cookie", "set-cookie", "authorization", "proxy-authorization":
		return true
	}
	return false
}

// Sanitize takes the credentials out of a, as Chrome's sanitized export
// does: the values of the Cookie, Set-Cookie, Authorization and
// Proxy-Authorization headers and of the cookies are Redacted. Addresses
// and the bodies of requests and answers stay as they were, and the
// archive's comment says so.
func Sanitize(a *Archive) {
	for i := range a.Log.Entries {
		e := &a.Log.Entries[i]
		for _, headers := range [][]Pair{e.Request.Headers, e.Response.Headers} {
			for j := range headers {
				if secret(headers[j].Name) {
					headers[j].Value = Redacted
				}
			}
		}
		for _, cookies := range [][]Cookie{e.Request.Cookies, e.Response.Cookies} {
			for j := range cookies {
				cookies[j].Value = Redacted
			}
		}
	}
	a.Log.Comment = "Sanitized: the values of the Cookie, Set-Cookie, Authorization and Proxy-Authorization " +
		"headers and of the cookies are " + Redacted + ". Addresses and the bodies of requests and answers are " +
		"as they were sent."
}
