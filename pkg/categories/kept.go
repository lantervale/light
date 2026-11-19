package categories

import (
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
	"time"
)

// Judgment is what a model made of a page, kept by the page's address:
// the category it answered, why, and when.
type Judgment struct {
	Address  string    `json:"address"`
	Category string    `json:"category"`
	Reason   string    `json:"reason,omitempty"`
	At       time.Time `json:"at"`
}

// question names what a model is asked, for the judgments kept to be
// read only by the list they were made for.
func (l *List) question() string {
	h := fnv.New64a()
	h.Write([]byte(l.Question()))
	return fmt.Sprintf("%016x", h.Sum64())
}

// onTheWeb reports whether a page at address is one a model judges: one
// of the web.
func onTheWeb(address string) bool {
	return strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://")
}

// Keep keeps what a model answered of the page at address, at at: for
// that page, never for its site, and only for a page of the web. An
// answer that names no category is kept as Default, and the reason as
// the sentence it is shown as.
func (l *List) Keep(address, answer, reason string, at time.Time) {
	c, known := l.Named(answer)
	if !known {
		c = Default()
	}
	l.keep(Judgment{Address: address, Category: c.Name, Reason: reason, At: at})
}

func (l *List) keep(j Judgment) {
	c, known := l.Named(j.Category)
	if j.Address = pageOf(j.Address); !known || !onTheWeb(j.Address) || j.At.IsZero() {
		return
	}
	j.Category, j.Reason = c.Name, Sentence(j.Reason)
	l.judged = append(slices.DeleteFunc(l.judged, func(k Judgment) bool { return k.Address == j.Address }), j)
	if len(l.judged) > maxKept {
		l.judged = slices.Delete(l.judged, 0, len(l.judged)-maxKept)
	}
	l.changed(false)
}

// Kept is the judgment kept of the page at address, made since since.
func (l *List) Kept(address string, since time.Time) (Judgment, bool) {
	page := pageOf(address)
	i := slices.IndexFunc(l.judged, func(j Judgment) bool { return j.Address == page })
	if i < 0 || !l.judged[i].At.After(since) {
		return Judgment{}, false
	}
	return l.judged[i], true
}

// KeptOfSite is the category of the page of the site of address that
// was last judged since since: a hint of what another page of it is.
func (l *List) KeptOfSite(address string, since time.Time) (Category, bool) {
	site := SiteOf(address)
	for _, j := range slices.Backward(l.judged) {
		if site != "" && SiteOf(j.Address) == site && j.At.After(since) {
			return l.Named(j.Category)
		}
	}
	return Category{}, false
}

// Expire drops the judgments made before before.
func (l *List) Expire(before time.Time) {
	kept := len(l.judged)
	l.judged = slices.DeleteFunc(l.judged, func(j Judgment) bool { return j.At.Before(before) })
	if len(l.judged) != kept {
		l.changed(false)
	}
}

// ForgetKept forgets the judgments kept.
func (l *List) ForgetKept() {
	if len(l.judged) == 0 {
		return
	}
	l.judged = nil
	l.changed(false)
}
