//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package script_test

import (
	"errors"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/script"
)

func TestTheTimeAScriptIsPausedIsNoneOfItsBudget(t *testing.T) {
	ctx := open(t, script.Options{Budget: 150 * time.Millisecond})
	if err := ctx.Bind("wait", func([]any) (any, error) {
		resume := ctx.Pause()
		time.Sleep(400 * time.Millisecond)
		resume()
		return "waited", nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := ctx.Run(`wait() + " " + wait()`, "test"); err != nil || got != "waited waited" {
		t.Fatalf("a script paused longer than its budget gave %v, %v", got, err)
	}
	began := time.Now()
	_, err := ctx.Run(`wait(); for (;;) {}`, "test")
	if took := time.Since(began); !errors.Is(err, script.ErrInterrupted) || took < 500*time.Millisecond ||
		took > 2*time.Second {
		t.Fatalf("a script that loops after a pause ended with %v after %s", err, took)
	}
	if resume := ctx.Pause(); resume == nil {
		t.Fatal("a pause with no script running gives nothing to resume")
	} else {
		resume()
	}
	if got, failed := ctx.Run(`1 + 1`, "test"); failed != nil || got != float64(2) {
		t.Fatalf("after its pauses the context gave %v, %v", got, failed)
	}
}
