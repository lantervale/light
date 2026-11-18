// Package history is light's history in plain Go: the pages
// visited, oldest dropped past a cap, listed by day, filtered, and
// written to and read from JSON.
package history

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Visit is one page visited: where, under what title, and when.
type Visit struct {
	Address string    `json:"address"`
	Title   string    `json:"title,omitempty"`
	At      time.Time `json:"at"`
}

// Day is the visits of one day, newest first.
type Day struct {
	// Date is the day's midnight, where its visits were made.
	Date   time.Time
	Visits []Visit
}

// Log is the visits, oldest first, never more than its cap.
type Log struct {
	visits []Visit
	limit  int
}

// New is an empty log keeping the last limit visits.
func New(limit int) *Log { return &Log{limit: limit} }

// SetLimit changes how many visits are kept, dropping the oldest past
// it.
func (l *Log) SetLimit(limit int) {
	l.limit = limit
	l.trim()
}

func (l *Log) Len() int { return len(l.visits) }

// Add notes a visit. The address of the visit before it, again, is that
// visit still, at its new time: a reload, or a page whose title landed,
// which it takes.
func (l *Log) Add(address, title string, at time.Time) {
	if n := len(l.visits); n > 0 && l.visits[n-1].Address == address {
		l.visits[n-1].At = at
		if title != "" {
			l.visits[n-1].Title = title
		}
		return
	}
	l.visits = append(l.visits, Visit{Address: address, Title: title, At: at})
	l.trim()
}

func (l *Log) trim() {
	if over := len(l.visits) - l.limit; over > 0 {
		l.visits = slices.Delete(l.visits, 0, over)
	}
}

// Days is the visits whose title or address holds every word of filter,
// whatever its case, newest first and grouped by the day they were made.
func (l *Log) Days(filter string) []Day {
	words := strings.Fields(strings.ToLower(filter))
	var days []Day
	for _, v := range slices.Backward(l.visits) {
		if !holds(strings.ToLower(v.Title+" "+v.Address), words) {
			continue
		}
		y, m, d := v.At.Date()
		date := time.Date(y, m, d, 0, 0, 0, 0, v.At.Location())
		if len(days) == 0 || !days[len(days)-1].Date.Equal(date) {
			days = append(days, Day{Date: date})
		}
		last := &days[len(days)-1]
		last.Visits = append(last.Visits, v)
	}
	return days
}

func holds(text string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

// Remove takes out the visit made to v's address at v's time.
func (l *Log) Remove(v Visit) {
	l.visits = slices.DeleteFunc(l.visits, func(o Visit) bool {
		return o.Address == v.Address && o.At.Equal(v.At)
	})
}

func (l *Log) Clear() { l.visits = nil }

// file is a log as written.
type file struct {
	Visits []Visit `json:"visits"`
}

func (l *Log) Encode() ([]byte, error) {
	data, err := json.Marshal(file{l.visits})
	if err != nil {
		return nil, fmt.Errorf("writing the history: %w", err)
	}
	return data, nil
}

// Decode reads what Encode wrote, keeping the last limit visits. Data it
// cannot read gives an empty log, ready to use, and the reason.
func Decode(data []byte, limit int) (*Log, error) {
	l := New(limit)
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return l, fmt.Errorf("reading the history: %w", err)
	}
	l.visits = f.Visits
	l.trim()
	return l, nil
}
