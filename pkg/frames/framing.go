package frames

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Ancestor is a document a frame is in: its address, or Opaque for one
// with an origin nothing shares (a sandboxed frame's).
type Ancestor struct {
	URL    *url.URL
	Opaque bool
}

// RefusedError is a document that does not let itself be shown in the
// frame it was asked for.
type RefusedError struct {
	// Address is the document's.
	Address string
	// Directive is the frame-ancestors directive that refused, "" when
	// Options did: the value of its X-Frame-Options.
	Directive, Options string
}

// Error is the refusal as Chrome's console words it.
func (e *RefusedError) Error() string {
	if e.Directive != "" {
		return fmt.Sprintf("Refused to frame '%s' because an ancestor violates the following Content Security "+
			"Policy directive: \"%s\".", e.Address, e.Directive)
	}
	return fmt.Sprintf("Refused to display '%s' in a frame because it set 'X-Frame-Options' to '%s'.", e.Address,
		e.Options)
}

const frameAncestors = "frame-ancestors"

// Allowed is why the document at framed, answered with header, is not
// shown in a frame under ancestors, the nearest first and the tab's
// page last; nil when it is. A Content-Security-Policy that has a
// frame-ancestors directive decides alone, each policy in its turn and
// every ancestor against it; else X-Frame-Options does: DENY refuses,
// SAMEORIGIN asks every ancestor to be of the document's origin, values
// that disagree refuse, and ALLOW-FROM is read as nothing.
func Allowed(header http.Header, framed *url.URL, ancestors []Ancestor) error {
	decided := false
	for _, policy := range policies(header) {
		sources, has := directive(policy)
		if !has {
			continue
		}
		decided = true
		for _, a := range ancestors {
			if !matches(sources, framed, a) {
				return &RefusedError{Address: framed.String(),
					Directive: strings.TrimSpace(frameAncestors + " " + strings.Join(sources, " "))}
			}
		}
	}
	if decided {
		return nil
	}
	return options(header, framed, ancestors)
}

// policies are the policies a document's Content-Security-Policy
// headers hold: each header may list several, between commas.
func policies(header http.Header) []string {
	var out []string
	for _, line := range header.Values("Content-Security-Policy") {
		for _, policy := range strings.Split(line, ",") {
			if strings.TrimSpace(policy) != "" {
				out = append(out, policy)
			}
		}
	}
	return out
}

// directive is the sources of a policy's frame-ancestors, the first it
// has, and whether it has one.
func directive(policy string) ([]string, bool) {
	for _, d := range strings.Split(policy, ";") {
		fields := strings.Fields(d)
		if len(fields) > 0 && strings.EqualFold(fields[0], frameAncestors) {
			return fields[1:], true
		}
	}
	return nil, false
}

func matches(sources []string, framed *url.URL, a Ancestor) bool {
	if a.Opaque || !web(a.URL) {
		return false
	}
	for _, source := range sources {
		if lowered := strings.ToLower(source); lowered != "'none'" && matchesSource(lowered, framed, a.URL) {
			return true
		}
	}
	return false
}

// matchesSource reports whether at is an address source, lowered,
// names for the document at self: 'self', a scheme, or a host with a
// scheme, a port and a leading wildcard, each optional.
func matchesSource(source string, self, at *url.URL) bool {
	switch {
	case source == "'self'":
		return Origin(self) == Origin(at)
	case source == "*":
		return true
	case strings.HasSuffix(source, ":") && !strings.Contains(source, "/"):
		return schemeMatches(strings.TrimSuffix(source, ":"), at.Scheme)
	case strings.HasPrefix(source, "'"):
		return false
	}
	scheme, rest, has := strings.Cut(source, "://")
	if !has {
		scheme, rest = self.Scheme, source
	}
	if !schemeMatches(scheme, at.Scheme) {
		return false
	}
	rest, _, _ = strings.Cut(rest, "/")
	host, wanted := rest, ""
	if i := strings.LastIndex(rest, ":"); i >= 0 && !strings.HasSuffix(rest, "]") {
		host, wanted = rest[:i], rest[i+1:]
	}
	if !hostMatches(host, strings.ToLower(at.Hostname())) {
		return false
	}
	switch wanted {
	case "*":
		return true
	case "":
		return port(at) == defaultPort(at.Scheme)
	}
	return port(at) == wanted
}

// schemeMatches reports whether an address of that scheme is one a
// source of the scheme wanted names: its own, and https for http.
func schemeMatches(wanted, scheme string) bool {
	return wanted == scheme || wanted == schemeHTTP && scheme == schemeHTTPS
}

func hostMatches(pattern, host string) bool {
	pattern = strings.Trim(pattern, "[]")
	if under, wild := strings.CutPrefix(pattern, "*."); wild {
		return strings.HasSuffix(host, "."+under)
	}
	return pattern == host
}

// options is Allowed by X-Frame-Options alone.
func options(header http.Header, framed *url.URL, ancestors []Ancestor) error {
	var values []string
	known := false
	for _, line := range header.Values("X-Frame-Options") {
		for _, value := range strings.Split(line, ",") {
			value = strings.ToLower(strings.TrimSpace(value))
			if value == "" {
				continue
			}
			known = known || value == "deny" || value == "sameorigin" || value == "allowall"
			if !containsString(values, value) {
				values = append(values, value)
			}
		}
	}
	refused := &RefusedError{Address: framed.String(), Options: strings.Join(values, ", ")}
	switch {
	case len(values) == 0:
		return nil
	case len(values) > 1 && known:
		return refused
	case len(values) > 1:
		return nil
	case values[0] == "deny":
		return refused
	case values[0] != "sameorigin":
		return nil
	}
	for _, a := range ancestors {
		if a.Opaque || Origin(a.URL) != Origin(framed) {
			return refused
		}
	}
	return nil
}

func containsString(list []string, s string) bool {
	for _, have := range list {
		if have == s {
			return true
		}
	}
	return false
}
