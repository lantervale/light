package instant_test

import (
	"sync"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/instant"
)

func TestEachInstantIsAfterTheOnesBefore(t *testing.T) {
	last := instant.Now()
	for range 10000 {
		now := instant.Now()
		if !now.After(last) {
			t.Fatalf("%v comes after %v", now, last)
		}
		last = now
	}
	if gap := time.Since(last); gap < -time.Millisecond || gap > time.Second {
		t.Fatalf("the instants are %v from the clock", gap)
	}
	if instant.Since(last) <= 0 {
		t.Fatal("no time passed since the last instant")
	}
}

func TestInstantsTakenAtOnceAreAllApart(t *testing.T) {
	const takers, each = 8, 1000
	var mu sync.Mutex
	seen := map[time.Time]bool{}
	var wg sync.WaitGroup
	for range takers {
		wg.Go(func() {
			got := make([]time.Time, each)
			for i := range got {
				got[i] = instant.Now()
			}
			mu.Lock()
			defer mu.Unlock()
			for _, at := range got {
				seen[at] = true
			}
		})
	}
	wg.Wait()
	if len(seen) != takers*each {
		t.Fatalf("%d instants of %d are apart", len(seen), takers*each)
	}
}
