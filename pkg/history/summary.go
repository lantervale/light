package history

import (
	"slices"
	"strings"
)

// Summarise notes what the page of the last visit to address says, and
// reports whether there is one.
func (l *Log) Summarise(address, summary string) bool {
	for i, v := range slices.Backward(l.visits) {
		if v.Address == address {
			l.visits[i].Summary = strings.TrimSpace(summary)
			return true
		}
	}
	return false
}

// Last is the last visit to address.
func (l *Log) Last(address string) (Visit, bool) {
	for _, v := range slices.Backward(l.visits) {
		if v.Address == address {
			return v, true
		}
	}
	return Visit{}, false
}
