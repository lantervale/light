package dom

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/lantervale/light/internal/web"
)

// importMap is what a page's import maps say of the specifiers its
// modules import: an address for a name, or for what begins with one.
type importMap struct {
	imports map[string]string
	scopes  []importScope
}

// importScope is the names of the modules under one address.
type importScope struct {
	under   string
	imports map[string]string
}

// bareError is a specifier that is neither an address nor a mapped name.
type bareError struct{ specifier string }

func (e *bareError) Error() string {
	return fmt.Sprintf(`Failed to resolve module specifier %q. Relative references must start with either "/", `+
		`"./", or "../".`, e.specifier)
}

// located is a specifier that is an address by itself: one that begins
// with /, ./ or ../, read from base, or a whole address; nil for a name.
func located(specifier string, base *url.URL) *url.URL {
	if strings.HasPrefix(specifier, "/") || strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../") {
		to, err := base.Parse(specifier)
		if err != nil {
			return nil
		}
		return to
	}
	to, err := url.Parse(specifier)
	if err != nil || to.Scheme == "" {
		return nil
	}
	return to
}

// add reads the text of an import map of the page at base into the
// map; what it says of a name the map already has is left out, as
// browsers leave it. It gives what the map does not take, a line each.
func (m *importMap) add(text string, base *url.URL) ([]string, error) {
	var read struct {
		Imports map[string]json.RawMessage            `json:"imports"`
		Scopes  map[string]map[string]json.RawMessage `json:"scopes"`
	}
	if err := json.Unmarshal([]byte(text), &read); err != nil {
		return nil, errors.New("The import map is not read: " + err.Error())
	}
	var refused []string
	if m.imports == nil {
		m.imports = map[string]string{}
	}
	refused = append(refused, addresses(m.imports, read.Imports, base)...)
	for under, names := range read.Scopes {
		prefix, err := base.Parse(under)
		if err != nil {
			refused = append(refused, fmt.Sprintf("The scope %q of the import map is no address.", under))
			continue
		}
		at := slices.IndexFunc(m.scopes, func(s importScope) bool { return s.under == prefix.String() })
		if at < 0 {
			m.scopes = append(m.scopes, importScope{under: prefix.String(), imports: map[string]string{}})
			at = len(m.scopes) - 1
		}
		refused = append(refused, addresses(m.scopes[at].imports, names, base)...)
	}
	slices.SortFunc(m.scopes, func(a, b importScope) int { return strings.Compare(b.under, a.under) })
	return refused, nil
}

// addresses reads the names of an import map into the names kept: each
// with its address, "" for one that names none.
func addresses(into map[string]string, names map[string]json.RawMessage, base *url.URL) []string {
	var refused []string
	for name, value := range names {
		if name == "" {
			refused = append(refused, "A name of the import map is empty.")
			continue
		}
		if at := located(name, base); at != nil {
			name = at.String()
		}
		if _, known := into[name]; known {
			continue
		}
		var address string
		to := (*url.URL)(nil)
		if json.Unmarshal(value, &address) == nil {
			to = located(address, base)
		}
		var said any
		switch {
		case json.Unmarshal(value, &said) == nil && said == nil:
			into[name] = ""
		case to == nil:
			refused = append(refused, fmt.Sprintf("The import map gives %q no address.", name))
			into[name] = ""
		case strings.HasSuffix(name, "/") && !strings.HasSuffix(to.String(), "/"):
			refused = append(refused, fmt.Sprintf("The import map gives %q an address that does not end with /.", name))
			into[name] = ""
		default:
			into[name] = to.String()
		}
	}
	return refused
}

// resolve is the address of the module a specifier names in the module
// at from: what the map says of it, the scopes from is under first,
// else the specifier itself when it is an address.
func (m *importMap) resolve(specifier string, from *url.URL) (string, error) {
	at := located(specifier, from)
	name := specifier
	if at != nil {
		name = at.String()
	}
	for _, scope := range m.scopes {
		if scope.under != from.String() && (!strings.HasSuffix(scope.under, "/") ||
			!strings.HasPrefix(from.String(), scope.under)) {
			continue
		}
		if to, found, err := match(scope.imports, name, at); found || err != nil {
			return to, err
		}
	}
	if to, found, err := match(m.imports, name, at); found || err != nil {
		return to, err
	}
	if at == nil {
		return "", &bareError{specifier}
	}
	return at.String(), nil
}

// match is what names say of one name: its own address, else that of
// the longest name ending with / it begins with.
func match(names map[string]string, name string, at *url.URL) (string, bool, error) {
	blocked := fmt.Errorf("the import map gives %q no address", name)
	if to, known := names[name]; known {
		if to == "" {
			return "", false, blocked
		}
		return to, true, nil
	}
	if at != nil && !web.Fetched(at.String()) {
		return "", false, nil
	}
	longest := ""
	for prefix := range names {
		if strings.HasSuffix(prefix, "/") && strings.HasPrefix(name, prefix) && len(prefix) > len(longest) {
			longest = prefix
		}
	}
	if longest == "" {
		return "", false, nil
	}
	under, err := url.Parse(names[longest])
	if names[longest] == "" || err != nil {
		return "", false, blocked
	}
	to, err := under.Parse(name[len(longest):])
	if err != nil || !strings.HasPrefix(to.String(), names[longest]) {
		return "", false, blocked
	}
	return to.String(), true, nil
}
