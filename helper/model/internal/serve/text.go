package serve

import (
	"strings"
	"unicode/utf8"
)

// Stopper passes an answer on piece by piece, whole characters only,
// and ends it before the first of its stop texts: a piece that may be
// the start of one is held until what follows tells.
type Stopper struct {
	stops []string
	held  string
	emit  func(string)
	// Stopped is whether a stop text was met.
	Stopped bool
}

// NewStopper is a Stopper writing through emit.
func NewStopper(stops []string, emit func(string)) *Stopper {
	kept := make([]string, 0, len(stops))
	for _, stop := range stops {
		if stop != "" {
			kept = append(kept, stop)
		}
	}
	return &Stopper{stops: kept, emit: emit}
}

// Write takes the next piece. It reports false once the answer is over.
func (s *Stopper) Write(piece string) bool {
	if s.Stopped {
		return false
	}
	s.held += piece
	first := -1
	for _, stop := range s.stops {
		if at := strings.Index(s.held, stop); at >= 0 && (first < 0 || at < first) {
			first = at
		}
	}
	if first >= 0 {
		s.Stopped = true
		s.pass(s.held[:first])
		s.held = ""
		return false
	}
	keep := 0
	for _, stop := range s.stops {
		for n := min(len(stop)-1, len(s.held)); n > keep; n-- {
			if strings.HasSuffix(s.held, stop[:n]) {
				keep = n
				break
			}
		}
	}
	whole := len(s.held) - keep
	// A character cut by the end of a piece waits for its other bytes.
	for cut := 1; cut <= min(utf8.UTFMax-1, whole); cut++ {
		if r := s.held[whole-cut]; utf8.RuneStart(r) {
			if !utf8.FullRuneInString(s.held[whole-cut : whole]) {
				whole -= cut
			}
			break
		}
	}
	s.pass(s.held[:whole])
	s.held = s.held[whole:]
	return true
}

// Flush passes on what was held, at the answer's end.
func (s *Stopper) Flush() {
	if !s.Stopped {
		s.pass(s.held)
	}
	s.held = ""
}

func (s *Stopper) pass(text string) {
	if text != "" {
		s.emit(strings.ToValidUTF8(text, "�"))
	}
}
