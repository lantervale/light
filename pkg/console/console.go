// Package console is a page's console as a browser's developer tools
// list it: what its scripts printed and the exceptions nothing caught,
// a line each, one said again counted in place of listed twice, the
// oldest dropped past a cap.
package console

import (
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Level is how much a line matters.
type Level string

const (
	Log   Level = "log"
	Info  Level = "info"
	Warn  Level = "warn"
	Error Level = "error"
	Debug Level = "debug"
	// Input is what was typed in the console, and Result what it gave.
	Input  Level = "input"
	Result Level = "result"
)

// Line is one line of the console.
type Line struct {
	Level Level
	Text  string
	// Source is the address of the script the line is from and Line
	// where in it, "" and 0 when unknown.
	Source string
	Line   int
	// Stack is the calls that led to it, a line each, "" when it has
	// none to show.
	Stack string
	// Depth is how many groups it was said in.
	Depth int
	// Count is how many times in a row it was said past the first.
	Count int
	At    time.Time
}

// Times is how many times in a row the line was said.
func (l Line) Times() int { return l.Count + 1 }

// typed reports whether the line is the console's own: what was typed
// in it, or what that gave.
func (l Line) typed() bool { return l.Level == Input || l.Level == Result }

// again reports whether next says what l did.
func (l Line) again(next Line) bool {
	return !l.typed() && l.Level == next.Level && l.Text == next.Text && l.Source == next.Source &&
		l.Line == next.Line && l.Stack == next.Stack && l.Depth == next.Depth
}

// Where is the script and the line a Line is from, as short as a panel
// shows it: "" when unknown.
func (l Line) Where() string {
	if l.Source == "" {
		return ""
	}
	name := l.Source
	if u, err := url.Parse(l.Source); err == nil {
		if name = path.Base(u.Path); name == "." || name == "/" {
			name = u.Host
		}
	}
	if l.Line == 0 {
		return name
	}
	return name + ":" + strconv.Itoa(l.Line)
}

// Lines is a console's lines, safe to add to from any goroutine.
type Lines struct {
	mu      sync.Mutex
	lines   []Line
	limit   int
	dropped int
	version uint64
}

// New is an empty console keeping limit lines.
func New(limit int) *Lines { return &Lines{limit: limit} }

// Add appends a line, timed now when it has no time.
func (l *Lines) Add(line Line) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if line.At.IsZero() {
		line.At = time.Now()
	}
	l.version++
	if last := len(l.lines) - 1; last >= 0 && l.lines[last].again(line) {
		l.lines[last].Count += line.Times()
		l.lines[last].At = line.At
		return
	}
	l.lines = append(l.lines, line)
	if over := len(l.lines) - l.limit; over > 0 {
		l.lines = slices.Delete(l.lines, 0, over)
		l.dropped += over
	}
}

// Lines are the lines kept, oldest first.
func (l *Lines) Lines() []Line {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.lines)
}

// Count is how many times the lines kept of level were said.
func (l *Lines) Count(level Level) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, line := range l.lines {
		if line.Level == level {
			n += line.Times()
		}
	}
	return n
}

// Said is how many times the page said the lines kept: what was typed
// in the console and what that gave are not counted.
func (l *Lines) Said() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, line := range l.lines {
		if !line.typed() {
			n += line.Times()
		}
	}
	return n
}

// Dropped is how many lines went past the cap since the last Clear.
func (l *Lines) Dropped() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.dropped
}

// Version changes whenever the lines do.
func (l *Lines) Version() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.version
}

// Clear drops every line.
func (l *Lines) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines, l.dropped = nil, 0
	l.version++
}

// Filter keeps the lines whose level is not muted and whose text or
// place holds text, whatever its case. What was typed in the console
// and what that gave are of every level.
func Filter(lines []Line, text string, muted map[Level]bool) []Line {
	text = strings.ToLower(text)
	var kept []Line
	for _, l := range lines {
		if muted[l.Level] && !l.typed() {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(l.Text), text) &&
			!strings.Contains(strings.ToLower(l.Where()), text) {
			continue
		}
		kept = append(kept, l)
	}
	return kept
}
