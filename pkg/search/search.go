// Package search is the search engines light knows, in plain Go: what
// each is called and the address its results are at.
package search

import (
	"net/url"
	"slices"
	"strings"

	"github.com/lantervale/light/pkg/tabs"
)

// The ids of the engines, as the settings' file names them.
const (
	DuckDuckGo = "duckduckgo"
	Own        = "light"
	// Default is the engine light starts with.
	Default = DuckDuckGo
)

// slot is where an engine's address takes the words searched for.
const slot = "{query}"

// Engine is one search engine: what the settings' file calls it, what
// it is called, and the address of its results, the words in place of
// {query}. Plain is the address of the results it has for a browser
// that runs no script, "" when Results needs none.
type Engine struct {
	ID, Name, Results, Plain string
}

// Engines are the engines, the default first.
func Engines() []Engine {
	return []Engine{
		{DuckDuckGo, "DuckDuckGo", "https://duckduckgo.com/?q=" + slot, "https://html.duckduckgo.com/html/?q=" + slot},
		{Own, "light's own pages", tabs.Page(tabs.SearchName) + "?q=" + slot, ""},
	}
}

// Known reports whether id names an engine.
func Known(id string) bool {
	return slices.ContainsFunc(Engines(), func(e Engine) bool { return e.ID == id })
}

// Named is the engine id names, the default for one it does not know.
func Named(id string) Engine {
	engines := Engines()
	for _, e := range engines {
		if e.ID == id {
			return e
		}
	}
	return engines[0]
}

// Serves reports whether address is a page of the engine's site.
func (e Engine) Serves(address string) bool {
	at, err := url.Parse(address)
	if err != nil || at.Host == "" {
		return false
	}
	for _, results := range []string{e.Results, e.Plain} {
		if of, bad := url.Parse(results); bad == nil && of.Scheme == at.Scheme && of.Host == at.Host {
			return true
		}
	}
	return false
}

// Address is the address of what the engine finds for query, for a
// browser that runs a page's scripts or not: the words as a form's
// value, and nothing else added.
func (e Engine) Address(query string, scripts bool) string {
	results := e.Results
	if !scripts && e.Plain != "" {
		results = e.Plain
	}
	return strings.Replace(results, slot, url.QueryEscape(query), 1)
}
