package dom

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/lantervale/light/pkg/cookies"
)

// viewport is the page's view: its width and height, and how far it is
// scrolled, which light does not tell scripts.
func (r *Runtime) viewport(arguments) (any, error) {
	if r.host.Viewport == nil {
		return []float32{0, 0, 0, 0}, nil
	}
	width, height := r.host.Viewport()
	return []float32{width, height, 0, 0}, nil
}

// scroll is asked to scroll the page's view, which scripts cannot.
func (r *Runtime) scroll(arguments) (any, error) {
	return nothing()
}

// titleElement is the page's title element, nil for none.
func (r *Runtime) titleElement() *html.Node { return element(r.head(), atom.Title) }

func (r *Runtime) titleNow() string {
	if e := r.titleElement(); e != nil {
		return strings.Join(strings.Fields(textOf(e)), " ")
	}
	return r.page.Title
}

func (r *Runtime) title(arguments) (any, error) { return r.titleNow(), nil }

func (r *Runtime) setTitle(a arguments) (any, error) {
	e := r.titleElement()
	if e == nil {
		head := r.head()
		if head == nil {
			return nothing()
		}
		e = r.tree.Element("title", "")
		r.tree.Insert(head, e, nil)
	}
	return r.setText(e, arguments{nil, a.text(0)})
}

// watchTitle tells the browser of the title scripts gave the page.
func (r *Runtime) watchTitle() {
	if title := r.titleNow(); title != r.page.Title {
		r.page.Title = title
		if r.host.Titled != nil {
			r.host.Titled(title)
		}
	}
}

// jar is the browser's cookies as the page sees them; without a
// browser's, cookies of the page's own, kept as long as it is.
func (r *Runtime) jar() Cookies {
	if r.cookies == nil {
		if r.cookies = r.host.Cookies; r.cookies == nil {
			at, _ := url.Parse(r.page.Address)
			r.cookies = cookies.New().Page(func() *url.URL { return at })
		}
	}
	return r.cookies
}

func (r *Runtime) cookie(arguments) (any, error) {
	if r.host.Sandboxed.Opaque {
		return "", nil
	}
	return r.jar().Get(), nil
}

func (r *Runtime) setCookie(a arguments) (any, error) {
	if !r.host.Sandboxed.Opaque {
		r.jar().Set(a.text(0))
	}
	return nothing()
}

func (r *Runtime) cookieEnabled(arguments) (any, error) {
	return !r.host.Sandboxed.Opaque && r.jar().Enabled(), nil
}

func (r *Runtime) log(a arguments) (any, error) {
	source, line, _ := where(a.text(2))
	m := Message{Level: a.text(0), Text: a.text(1), Source: source, Line: line, Depth: int(a.number(3))}
	if a.flag(4) {
		m.Stack = calls(a.text(2))
	}
	r.tell(m)
	return nothing()
}

func (r *Runtime) clear(arguments) (any, error) {
	if r.host.Cleared != nil {
		r.host.Cleared()
	}
	return nothing()
}

func (r *Runtime) uncaught(a arguments) (any, error) {
	source, line, _ := where(a.text(1))
	r.fault("Uncaught "+a.text(0), source, line, a.text(1))
	return nothing()
}

func (r *Runtime) where(a arguments) (any, error) {
	source, line, column := where(a.text(0))
	return []any{source, line, column}, nil
}

func (r *Runtime) navigate(a arguments) (any, error) {
	if r.host.Navigate != nil && r.sends(a.text(0), false) {
		r.host.Navigate(a.text(0), a.flag(1))
	}
	return nothing()
}

func (r *Runtime) reload(arguments) (any, error) {
	if r.host.Reload != nil && r.sends("", false) {
		r.host.Reload()
	}
	return nothing()
}

// history tells the browser the page moved without leaving ("push",
// "replace"), or answers how many steps the tab's history holds
// ("length").
func (r *Runtime) history(a arguments) (any, error) {
	switch how := a.text(0); {
	case how == "length" && r.host.Steps != nil:
		return r.host.Steps(), nil
	case how == "length":
		return 1, nil
	case r.host.Moved != nil:
		r.host.Moved(how, a.text(1))
	}
	return nothing()
}

func (r *Runtime) travel(a arguments) (any, error) {
	if r.host.Travel != nil && r.sends("", false) {
		r.host.Travel(int(a.number(0)))
	}
	return nothing()
}

// parsed reads an address against a base, "" for none: its parts as
// the URL class names them, null for what is no address. A base's
// fragment is never the address's, and nothing but a fragment is read
// against a base that has no path to read it from.
func parsed(a arguments) (any, error) {
	reference := strings.TrimSpace(a.text(0))
	u, err := url.Parse(reference)
	if base := a.text(1); err == nil && base != "" {
		u, err = against(base, reference, u)
	}
	if err != nil || u == nil || u.Scheme == "" {
		return nothing()
	}
	return parts(u), nil
}

// against is a reference read against a base, nil where the base has
// no path to read it from.
func against(base, reference string, u *url.URL) (*url.URL, error) {
	from, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	if from.Opaque != "" && !u.IsAbs() && !strings.HasPrefix(reference, "#") {
		return nil, nil //nolint:nilnil // no address, and no error to tell
	}
	u = from.ResolveReference(u)
	if !strings.Contains(reference, "#") {
		u.Fragment, u.RawFragment = "", ""
	}
	return u, nil
}

func parts(u *url.URL) map[string]any {
	if u.Path == "" && u.Host != "" {
		u.Path = "/"
	}
	u.RawQuery = queryInASCII(u.RawQuery, u.Scheme)
	password, _ := u.User.Password()
	out := map[string]any{
		"href": u.String(), "protocol": u.Scheme + ":", "username": u.User.Username(), "password": password,
		"host": u.Host, "hostname": u.Hostname(), "port": u.Port(), "pathname": u.EscapedPath(),
		"search": "", "hash": "", "origin": originOf(u),
	}
	if u.Opaque != "" {
		out["pathname"] = u.Opaque
	}
	if u.RawQuery != "" {
		out["search"] = "?" + u.RawQuery
	}
	if fragment := u.EscapedFragment(); fragment != "" {
		out["hash"] = "#" + fragment
	}
	return out
}

// webSchemes are the schemes of the addresses the URL standard calls
// special.
const webSchemes = " http https ws wss ftp file "

// queryInASCII is a query as an address writes it: what is not ASCII,
// a space, a quote and the angle brackets as the bytes they are, and
// an apostrophe too in the addresses of the web.
func queryInASCII(query, scheme string) string {
	special := "\"<>"
	if strings.Contains(webSchemes, " "+scheme+" ") {
		special += "'"
	}
	var out strings.Builder
	for i := range len(query) {
		if c := query[i]; c <= ' ' || c >= 0x7f || strings.IndexByte(special, c) >= 0 {
			fmt.Fprintf(&out, "%%%02X", c)
		} else {
			out.WriteByte(c)
		}
	}
	return out.String()
}

// originOf is the origin of an address: null for one that has none.
func originOf(u *url.URL) string {
	switch u.Scheme {
	case "http", "https", "light":
		return u.Scheme + "://" + u.Host
	}
	return opaqueOrigin
}

// changed is an address with one of its parts set.
func changed(a arguments) (any, error) {
	u, err := url.Parse(a.text(0))
	if err != nil {
		return nothing()
	}
	value := a.text(2)
	switch a.text(1) {
	case "protocol":
		u.Scheme = strings.TrimSuffix(value, ":")
	case "host":
		u.Host = value
	case "hostname":
		u.Host = joined(value, u.Port())
	case "port":
		u.Host = joined(u.Hostname(), value)
	case "pathname":
		written := "/" + strings.TrimPrefix(value, "/")
		u.Path, u.RawPath = written, ""
		if plain, escaped := url.PathUnescape(written); escaped == nil {
			u.Path, u.RawPath = plain, written
		}
	case "search":
		u.RawQuery = strings.TrimPrefix(value, "?")
	case "hash":
		u.Fragment, u.RawFragment = strings.TrimPrefix(value, "#"), ""
	case "username":
		password, _ := u.User.Password()
		u.User = url.UserPassword(value, password)
	case "password":
		u.User = url.UserPassword(u.User.Username(), value)
	}
	return parts(u), nil
}

func joined(hostname, port string) string {
	if port == "" {
		return hostname
	}
	return hostname + ":" + port
}

// memory is a storage area kept for as long as its page.
type memory map[string]string

func (m memory) Get(key string) (string, bool) { value, ok := m[key]; return value, ok }
func (m memory) Set(key, value string)         { m[key] = value }
func (m memory) Remove(key string)             { delete(m, key) }
func (m memory) Clear()                        { clear(m) }
func (m memory) Keys() []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// area is a storage area of the page's site: the browser's, or one
// kept for as long as the page.
func (r *Runtime) area(name string) Storage {
	if kept, known := r.stores[name]; known {
		return kept
	}
	var area Storage
	if r.host.Storage != nil {
		area = r.host.Storage(name)
	}
	if area == nil {
		area = memory{}
	}
	r.stores[name] = area
	return area
}

// maxStored is how many bytes one key of a storage area may hold.
const maxStored = 1 << 20

func (r *Runtime) storage(a arguments) (any, error) {
	if r.host.Sandboxed.Opaque {
		//nolint:revive,staticcheck // a DOMException's message, as Chrome words it
		return nil, errors.New("SecurityError: The document is sandboxed and lacks the 'allow-same-origin' flag.")
	}
	area, key := r.area(a.text(0)), a.text(2)
	switch a.text(1) {
	case "get":
		if value, ok := area.Get(key); ok {
			return value, nil
		}
	case "set":
		if len(key)+len(a.text(3)) > maxStored {
			return nil, fmt.Errorf("QuotaExceededError: light stores at most %d bytes under one key", maxStored)
		}
		area.Set(key, a.text(3))
	case "remove":
		area.Remove(key)
	case "clear":
		area.Clear()
	case "keys":
		if keys := area.Keys(); keys != nil {
			return keys, nil
		}
		return []string{}, nil
	}
	return nothing()
}

// atob decodes base64 into a string of one character a byte, null for
// what is not base64.
func atob(a arguments) (any, error) {
	text := strings.TrimRight(strings.Join(strings.Fields(a.text(0)), ""), "=")
	bytes, err := base64.RawStdEncoding.DecodeString(text)
	if err != nil {
		return nothing()
	}
	runes := make([]rune, len(bytes))
	for i, b := range bytes {
		runes[i] = rune(b)
	}
	return string(runes), nil
}

// btoa encodes a string of one character a byte as base64.
func btoa(a arguments) (any, error) {
	runes := []rune(a.text(0))
	bytes := make([]byte, len(runes))
	for i, c := range runes {
		bytes[i] = byte(c & 0xff)
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// maxRandom is how many random bytes a script may ask for at once.
const maxRandom = 65536

func random(a arguments) (any, error) {
	bytes := make([]byte, min(max(int(a.number(0)), 0), maxRandom))
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	out := make([]int, len(bytes))
	for i, b := range bytes {
		out[i] = int(b)
	}
	return out, nil
}
