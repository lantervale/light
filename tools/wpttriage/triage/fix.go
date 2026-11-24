package triage

import "strings"

// owners is a guess of where the fix of a directory's assertions lives, longest prefix first.
func owners() []struct{ prefix, owner string } {
	return []struct{ prefix, owner string }{
		{"/html/syntax", "HTML parser and serialiser (golang.org/x/net/html under light internal/dom)"},
		{"/html/semantics/forms", "light DOM forms (internal/dom validity, form elements)"},
		{"/html/dom", "light DOM bindings (internal/dom, reflection)"},
		{"/html", "light DOM bindings for HTML elements (internal/dom)"},
		{"/dom/ranges", "light DOM ranges (internal/dom/js 33-ranges)"},
		{"/dom/events", "light DOM events (internal/dom events)"},
		{"/dom", "light DOM core (internal/dom, internal/dom/js)"},
		{"/domxpath", "light XPath (internal/dom)"},
		{"/domparsing", "light DOM parsing and serialisation (internal/dom)"},
		{"/url", "light URL parser (internal/dom URL binding)"},
		{"/encoding", "light encoding (TextDecoder and TextEncoder bindings)"},
		{"/IndexedDB", "light IndexedDB (pkg/idb, internal/dom indexeddb)"},
		{"/fetch", "light fetch (pkg/fetch, internal/dom requests)"},
		{"/cors", "light fetch CORS (pkg/fetch policy)"},
		{"/console", "light console (pkg/console)"},
		{"/hr-time", "light timing (internal/dom window)"},
		{"/css/cssom", "light CSSOM bindings (internal/dom sheets) over ember ui css"},
		{"/css", "ember ui CSS (values, cascade, computed style)"},
	}
}

// Fix guesses where a cause's fix lives.
func Fix(c Cause) string {
	switch c.Kind {
	case MissingAPI:
		return "light DOM/bindings: add the API (internal/dom, internal/dom/js)"
	case Exception:
		return "light DOM/bindings: the API exists but throws or returns the wrong thing"
	case CSSComputed:
		return "ember ui CSS: the property's computed value (ui/css), light's getComputedStyle"
	case CSSParsing:
		return "ember ui CSS: the property's parser (ui/css)"
	case Reftest:
		if strings.HasPrefix(c.Scope, "/css") {
			return "ember ui layout and paint (" + strings.TrimPrefix(c.Scope, "/css/") + ")"
		}
		return "ember ui rendering of HTML (layout, paint, default styles)"
	case Timeout:
		return "light event loop, or an API whose event never fires (" + owner(c.Scope) + ")"
	case Crash:
		return "light or ember stability: read crash.log"
	case HarnessError:
		return "light script loading or a harness-level exception (" + owner(c.Scope) + ")"
	case Assertion, Other:
	}
	return owner(c.Scope)
}

func owner(scope string) string {
	for _, o := range owners() {
		if scope == o.prefix || strings.HasPrefix(scope, o.prefix+"/") {
			return o.owner
		}
	}
	return "light"
}
