// Package instant is the clock that times what light records: each
// instant it gives is after every one it gave before, so that what
// happened in order sorts in order even on a clock as coarse as
// Windows', which moves every 15.6 ms.
package instant

import (
	"sync"
	"time"
)

//nolint:gochecknoglobals // the one clock of the process
var clock struct {
	mu   sync.Mutex
	last time.Time
}

// Now is the time now, or a nanosecond after the last instant given when
// the clock has not moved since.
func Now() time.Time {
	now := time.Now()
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if !now.After(clock.last) {
		now = clock.last.Add(time.Nanosecond)
	}
	clock.last = now
	return now
}

// Since is the time from t to Now.
func Since(t time.Time) time.Duration { return Now().Sub(t) }
