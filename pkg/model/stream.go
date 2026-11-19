package model

import (
	"strings"
	"sync"
)

// Stream is an answer as it comes. It is safe for use from several
// goroutines.
type Stream struct {
	mu     sync.Mutex
	held   bool
	text   strings.Builder
	taken  int
	result Result
	err    error
	ended  bool
	done   chan struct{}
}

// newStream is a stream nothing came on yet. A held one gives its text
// only once whole: a Background request may start over.
func newStream(held bool) *Stream {
	return &Stream{held: held, done: make(chan struct{})}
}

// Take is the text that came since the last Take, and whether the
// answer ended (Err then says how). It never waits.
func (s *Stream) Take() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held && !s.ended {
		return "", false
	}
	text := s.text.String()[s.taken:]
	s.taken = s.text.Len()
	return text, s.ended
}

// Done is closed when the answer ended.
func (s *Stream) Done() <-chan struct{} { return s.done }

// Err is why the answer ended early, nil for one that ended well or has
// not ended.
func (s *Stream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Wait waits for the end of the answer and returns it whole.
func (s *Stream) Wait() (Result, error) {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.result
	result.Text = s.text.String()
	return result, s.err
}

func (s *Stream) push(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ended {
		s.text.WriteString(text)
	}
}

// restart forgets what came: the request starts over.
func (s *Stream) restart() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ended {
		s.text.Reset()
		s.taken = 0
	}
}

// finish ends the stream, once: a later end is ignored. It reports
// whether this call ended it.
func (s *Stream) finish(result Result, err error) bool {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return false
	}
	s.ended, s.result, s.err = true, result, err
	s.mu.Unlock()
	close(s.done)
	return true
}

func (s *Stream) over() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ended
}
