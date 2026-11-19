// Package complete is what the address field offers of this device
// while it is typed in, in plain Go: the pages visited and kept that
// match the words typed. Nothing of it asks anything of anybody.
package complete

import (
	"slices"
	"strings"
	"time"
)

// Page is a page this device knows: a visit, or a bookmark when Kept,
// under its title or the name it is kept by, at the time it was made.
type Page struct {
	Address, Title string
	At             time.Time
	Kept           bool
}

// offered is an address as it is offered: its page, and how many times
// it was visited.
type offered struct {
	Page

	visits int
}

// Match is the pages whose title or address holds every word of typed,
// whatever its case, each address once: the most visited first, then
// the most recent, and never more than limit. An address takes the
// name it is kept by, else the last title it was visited under.
func Match(typed string, pages []Page, limit int) []Page {
	words := strings.Fields(strings.ToLower(typed))
	if len(words) == 0 {
		return nil
	}
	var found []*offered
	by := map[string]*offered{}
	for _, p := range pages {
		if !holds(strings.ToLower(p.Title+" "+p.Address), words) {
			continue
		}
		o := by[p.Address]
		if o == nil {
			o = &offered{Page: Page{Address: p.Address}}
			by[p.Address] = o
			found = append(found, o)
		}
		o.take(p)
	}
	slices.SortStableFunc(found, func(a, b *offered) int {
		if a.visits != b.visits {
			return b.visits - a.visits
		}
		return b.At.Compare(a.At)
	})
	out := make([]Page, 0, min(limit, len(found)))
	for _, o := range found[:min(limit, len(found))] {
		out = append(out, o.Page)
	}
	return out
}

// take counts p for its address.
func (o *offered) take(p Page) {
	if !p.Kept {
		o.visits++
	}
	later := p.At.After(o.At)
	if p.Title != "" && (p.Kept || !o.Kept && (later || o.Title == "")) {
		o.Title = p.Title
	}
	if later {
		o.At = p.At
	}
	o.Kept = o.Kept || p.Kept
}

func holds(text string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}
