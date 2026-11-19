//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package script_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/script"
)

// told is what a Context told of a run that is late, from the
// watchdog's goroutine.
type told struct {
	sync.Mutex
	ran []time.Duration
}

func (t *told) late(again time.Duration, then func(ran time.Duration)) func(time.Duration) time.Duration {
	return func(ran time.Duration) time.Duration {
		t.Lock()
		t.ran = append(t.ran, ran)
		t.Unlock()
		if then != nil {
			then(ran)
		}
		return again
	}
}

func (t *told) times() []time.Duration {
	t.Lock()
	defer t.Unlock()
	return append([]time.Duration{}, t.ran...)
}

func TestARunPastItsBudgetGoesOnWhenItsLatenessIsToldOf(t *testing.T) {
	var heard told
	ctx, err := script.New(script.Options{Budget: 40 * time.Millisecond, Late: heard.late(30*time.Millisecond, nil)})
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	began := time.Now()
	got, err := ctx.Run(`const end = Date.now() + 200; let n = 0; while (Date.now() < end) n++; "ended"`, "test")
	if err != nil || got != "ended" || time.Since(began) < 190*time.Millisecond {
		t.Fatalf("a run past its budget that is waited for gave %v, %v after %s", got, err, time.Since(began))
	}
	ran := heard.times()
	if len(ran) < 3 || ran[0] != 40*time.Millisecond || ran[1] != 70*time.Millisecond || ran[2] != 100*time.Millisecond {
		t.Fatalf("its lateness was told of at %v", ran)
	}
	if got, err = ctx.Run(`1 + 1`, "test"); err != nil || got != float64(2) || len(heard.times()) != len(ran) {
		t.Fatalf("the run after gave %v, %v, told of %d times more", got, err, len(heard.times())-len(ran))
	}
}

func TestARunThatIsLateIsStoppedWhenAskedTo(t *testing.T) {
	var heard told
	var ctx script.Context
	stop := func(ran time.Duration) {
		if ran >= 90*time.Millisecond {
			ctx.Interrupt()
		}
	}
	ctx, err := script.New(script.Options{Budget: 30 * time.Millisecond, Late: heard.late(30*time.Millisecond, stop)})
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	began := time.Now()
	_, err = ctx.Run(`for (;;) {}`, "test")
	took := time.Since(began)
	if !errors.Is(err, script.ErrInterrupted) || took < 90*time.Millisecond || took > time.Second {
		t.Fatalf("a late run that is stopped ended with %v after %s", err, took)
	}
	if got, failed := ctx.Run(`"after"`, "test"); failed != nil || got != "after" {
		t.Fatalf("the run after a stopped one gave %v, %v", got, failed)
	}
}

func TestARunIsNotToldOfAgainWhenItsLatenessSaysSo(t *testing.T) {
	var heard told
	ctx, err := script.New(script.Options{Budget: 30 * time.Millisecond, Late: heard.late(0, nil)})
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if _, err = ctx.Run(`const end = Date.now() + 150; while (Date.now() < end) {}`, "test"); err != nil {
		t.Fatal(err)
	}
	if ran := heard.times(); len(ran) != 1 || ran[0] != 30*time.Millisecond {
		t.Fatalf("its lateness was told of at %v", ran)
	}
}

func TestTheTimeALateRunIsPausedIsNotItsOwn(t *testing.T) {
	var heard told
	ctx, err := script.New(script.Options{Budget: 60 * time.Millisecond, Late: heard.late(60*time.Millisecond, nil)})
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err = ctx.Bind("wait", func([]any) (any, error) {
		resume := ctx.Pause()
		time.Sleep(200 * time.Millisecond)
		resume()
		return 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = ctx.Run(`wait(); const end = Date.now() + 20; while (Date.now() < end) {}`, "test"); err != nil {
		t.Fatal(err)
	}
	if ran := heard.times(); len(ran) != 0 {
		t.Fatalf("a run paused longer than its budget was told late at %v", ran)
	}
}

func TestARunWithNobodyToTellIsStoppedAtItsBudget(t *testing.T) {
	ctx, err := script.New(script.Options{Budget: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	began := time.Now()
	_, err = ctx.Run(`for (;;) {}`, "test")
	if took := time.Since(began); !errors.Is(err, script.ErrInterrupted) || took < 50*time.Millisecond ||
		took > time.Second {
		t.Fatalf("a run past its budget ended with %v after %s", err, took)
	}
}
