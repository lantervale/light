// Package recall is what the pages visited said, in plain Go: for each
// page a few passages of its text with the vectors a model gave them,
// searched by the vector of a question, forgotten a page, a site or
// whole, the oldest dropped past a cap, and written to and read from a
// file. It is the most private thing light keeps: nothing here logs a
// passage or puts one in an error.
package recall

import (
	"cmp"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// What is kept and what matches. None of these was tried with a real
// embedding model: only the vectors of model.Fake were. They are to be
// tuned once one runs.
const (
	// PassageSize is the most bytes of a passage, and PassagesPerPage
	// the most passages kept of a page: its start.
	PassageSize     = 480
	PassagesPerPage = 12
	// MaxBytes is the most the pages take together, vectors included;
	// the oldest go past it.
	MaxBytes = 24 << 20
	// Threshold is the least a passage's vector shares with a
	// question's for the page to match (the cosine of the two), and
	// MostMatches the most pages a question is answered with.
	Threshold   = 0.35
	MostMatches = 8
)

// maxDimensions is the longest vector a file may name.
const maxDimensions = 1 << 14

// Match is a page a question matched: the passage of it that is the
// closest, and how close.
type Match struct {
	Address, Title string
	At             time.Time
	Passage        string
	Score          float32
}

// Query is what a search keeps: the pages visited since Since (all of
// them when it is zero) with a passage at least Threshold close, the
// Most closest.
type Query struct {
	Since     time.Time
	Threshold float32
	Most      int
}

type passage struct {
	text   string
	vector []float32
}

type page struct {
	address, title string
	at             time.Time
	passages       []passage
}

func (p page) size() int {
	size := 4 + len(p.address) + 4 + len(p.title) + 8 + 4
	for _, said := range p.passages {
		size += 4 + len(said.text) + 4*len(said.vector)
	}
	return size
}

// Store is the pages, the one remembered longest ago first, their
// vectors all of one length, never more than its cap together.
type Store struct {
	pages      []page
	dimensions int
	size       int
	limit      int
}

// New is an empty store keeping at most limit bytes.
func New(limit int) *Store { return &Store{limit: limit} }

func (s *Store) Len() int { return len(s.pages) }

// Size is the bytes the pages take in the file.
func (s *Store) Size() int { return s.size }

// Has reports whether what the page at address said is kept.
func (s *Store) Has(address string) bool {
	return slices.ContainsFunc(s.pages, func(p page) bool { return p.address == address })
}

// At is when the page at address was visited as it is kept, and
// whether it is kept.
func (s *Store) At(address string) (time.Time, bool) {
	if i := slices.IndexFunc(s.pages, func(p page) bool { return p.address == address }); i >= 0 {
		return s.pages[i].at, true
	}
	return time.Time{}, false
}

// Add keeps what the page at address said when it was visited at at:
// its first PassagesPerPage texts, each cut to PassageSize, with their
// vectors. It replaces what the page said before, and everything when
// the vectors are not of the length of those kept: they are another
// model's. A page that says nothing is not kept, and one over the cap
// alone neither.
func (s *Store) Add(address, title string, at time.Time, texts []string, vectors [][]float32) error {
	switch {
	case address == "":
		return errors.New("a page remembered needs an address")
	case len(texts) != len(vectors):
		return errors.New("a page remembered needs a vector a passage")
	}
	for _, v := range vectors {
		if len(v) == 0 || len(v) > maxDimensions || len(v) != len(vectors[0]) {
			return errors.New("the vectors of a page are not of one length")
		}
	}
	s.Forget(address)
	if len(texts) == 0 {
		return nil
	}
	if len(vectors[0]) != s.dimensions {
		s.Clear()
		s.dimensions = len(vectors[0])
	}
	added := page{address: address, title: title, at: at}
	for i, text := range texts[:min(len(texts), PassagesPerPage)] {
		added.passages = append(added.passages, passage{cut(text, PassageSize), slices.Clone(vectors[i])})
	}
	s.pages = append(s.pages, added)
	s.size += added.size()
	s.trim()
	return nil
}

func (s *Store) trim() {
	for len(s.pages) > 0 && s.size > s.limit {
		s.size -= s.pages[0].size()
		s.pages = slices.Delete(s.pages, 0, 1)
	}
}

// cut is the start of text, at most size bytes of it, whole runes.
func cut(text string, size int) string {
	if len(text) <= size {
		return text
	}
	for size > 0 && !utf8.RuneStart(text[size]) {
		size--
	}
	return text[:size]
}

// Search is the pages whose closest passage to the vector of a
// question is close enough, the closest first, and of two as close the
// one visited last.
func (s *Store) Search(question []float32, q Query) []Match {
	if len(question) != s.dimensions {
		return nil
	}
	var out []Match
	for _, p := range s.pages {
		if p.at.Before(q.Since) {
			continue
		}
		best := Match{Address: p.address, Title: p.title, At: p.at, Score: -1}
		for _, said := range p.passages {
			if score := dot(question, said.vector); score > best.Score {
				best.Passage, best.Score = said.text, score
			}
		}
		if best.Score >= q.Threshold {
			out = append(out, best)
		}
	}
	slices.SortStableFunc(out, func(a, b Match) int {
		return cmp.Or(cmp.Compare(b.Score, a.Score), b.At.Compare(a.At))
	})
	return out[:min(len(out), max(q.Most, 0))]
}

func dot(a, b []float32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// Keep forgets the pages keep does not hold, and reports how many
// went.
func (s *Store) Keep(keep func(address string) bool) int {
	before := len(s.pages)
	s.pages = slices.DeleteFunc(s.pages, func(p page) bool {
		if keep(p.address) {
			return false
		}
		s.size -= p.size()
		return true
	})
	if len(s.pages) == 0 {
		s.dimensions = 0
	}
	return before - len(s.pages)
}

// Forget forgets what the page at address said, and reports whether
// anything was kept of it.
func (s *Store) Forget(address string) bool {
	return s.Keep(func(kept string) bool { return kept != address }) > 0
}

// ForgetSite forgets what every page of a site said, and reports how
// many.
func (s *Store) ForgetSite(site string) int {
	return s.Keep(func(kept string) bool { return Site(kept) != site })
}

// Clear forgets everything.
func (s *Store) Clear() { s.pages, s.size, s.dimensions = nil, 0, 0 }

// Site names the site of an address of the web: its host in lower
// case, without its port or a leading www.; "" for another address.
func Site(address string) string {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// Passages cuts what a page says into at most most passages of at most
// size bytes, each as many whole sentences as fit; a sentence that does
// not fit is cut between two words, and a word that does not within
// itself.
func Passages(text string, size, most int) []string {
	var out []string
	current := ""
	for line := range strings.Lines(text) {
		for _, sentence := range sentences(strings.Join(strings.Fields(line), " ")) {
			for _, piece := range pieces(sentence, size) {
				if current != "" && len(current)+1+len(piece) > size {
					if out = append(out, current); len(out) == most {
						return out
					}
					current = ""
				}
				if current != "" {
					current += " "
				}
				current += piece
			}
		}
	}
	if current != "" && len(out) < most {
		out = append(out, current)
	}
	return out
}

// sentences are the sentences of a line.
func sentences(line string) []string {
	var out []string
	start := 0
	for at, r := range line {
		end := at + utf8.RuneLen(r)
		if strings.ContainsRune(".!?", r) && end < len(line) && unicode.IsSpace(rune(line[end])) {
			out = append(out, line[start:end])
			start = end + 1
		}
	}
	if start < len(line) {
		out = append(out, line[start:])
	}
	return out
}

// pieces are a sentence as it is when it fits size bytes, else cut
// between its words, and within one that is longer.
func pieces(sentence string, size int) []string {
	if len(sentence) <= size {
		return []string{sentence}
	}
	var out []string
	current := ""
	for word := range strings.FieldsSeq(sentence) {
		for len(word) > size {
			if current != "" {
				out, current = append(out, current), ""
			}
			head := cut(word, size)
			out, word = append(out, head), word[len(head):]
		}
		switch {
		case word == "":
		case current == "":
			current = word
		case len(current)+1+len(word) > size:
			out, current = append(out, current), word
		default:
			current += " " + word
		}
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}
