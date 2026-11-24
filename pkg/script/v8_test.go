//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package script_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/script"
)

func open(t *testing.T, opts script.Options) script.Context {
	t.Helper()
	ctx, err := script.New(opts)
	if err != nil {
		t.Fatalf("no context: %v", err)
	}
	t.Cleanup(ctx.Close)
	return ctx
}

func TestAScriptRunsAndItsValueComesBack(t *testing.T) {
	ctx := open(t, script.Options{})
	for _, c := range []struct {
		source string
		want   any
	}{
		{"1 + 2", 3.0},
		{"'a' + 'b'", "ab"},
		{"[1, 2].length === 2", true},
		{"null", nil},
		{"undefined", nil},
		{"({a: [1, 'two', null], b: {c: true}})",
			map[string]any{"a": []any{1.0, "two", nil}, "b": map[string]any{"c": true}}},
		{"[3, 4]", []any{3.0, 4.0}},
	} {
		got, err := ctx.Run(c.source, "test.js")
		if err != nil || fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s gave %#v, %v; want %#v", c.source, got, err, c.want)
		}
	}
}

func TestABoundFunctionIsCalledWithPlainValues(t *testing.T) {
	ctx := open(t, script.Options{})
	var got []any
	err := ctx.Bind("host", func(args []any) (any, error) {
		got = args
		n, _ := args[0].(float64)
		return map[string]any{"sum": n + 1, "list": []int{1, 2}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	back, err := ctx.Run(`const r = host(41, "s", true, null, undefined, {k: [1]}); r.sum + r.list.length`, "test.js")
	if err != nil || back != 44.0 {
		t.Fatalf("the call gave %v, %v", back, err)
	}
	want := []any{41.0, "s", true, nil, nil, map[string]any{"k": []any{1.0}}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("the function was given %#v, want %#v", got, want)
	}
}

func TestABoundFunctionsErrorIsThrown(t *testing.T) {
	ctx := open(t, script.Options{})
	_ = ctx.Bind("host", func([]any) (any, error) { return nil, errors.New("no such thing") })
	got, err := ctx.Run(`let said; try { host() } catch (e) { said = e instanceof Error ? e.message : "?" } said`,
		"test.js")
	if err != nil || got != "no such thing" {
		t.Fatalf("the script caught %v, %v", got, err)
	}
}

func TestAFunctionValueIsCalledLater(t *testing.T) {
	ctx := open(t, script.Options{})
	value, err := ctx.Run(`let total = 0; (function add(n, label) { total += n; return label + total })`, "test.js")
	if err != nil {
		t.Fatal(err)
	}
	add, ok := value.(script.Function)
	if !ok {
		t.Fatalf("a function came back as %T", value)
	}
	defer add.Release()
	for _, want := range []string{"t2", "t4"} {
		if got, failed := add.Call(2, "t"); failed != nil || got != want {
			t.Fatalf("the call gave %v, %v; want %s", got, failed, want)
		}
	}
}

func TestAnUncaughtExceptionSaysWhere(t *testing.T) {
	ctx := open(t, script.Options{})
	_, err := ctx.Run("let a = 1;\nlet b = 2;\nmissing(a, b);", "https://site.test/app.js")
	var thrown *script.Error
	if !errors.As(err, &thrown) {
		t.Fatalf("the error is %T %v", err, err)
	}
	if !strings.Contains(thrown.Message, "missing is not defined") || thrown.Source != "https://site.test/app.js" ||
		thrown.Line != 3 {
		t.Fatalf("the exception is %+v", thrown)
	}
	if got, failed := ctx.Run("a + b", "next.js"); failed != nil || got != 3.0 {
		t.Fatalf("the context after an exception gave %v, %v", got, failed)
	}
}

func TestASyntaxErrorSaysWhere(t *testing.T) {
	ctx := open(t, script.Options{})
	_, err := ctx.Run("let ok = 1;\nlet = = 2;", "bad.js")
	var thrown *script.Error
	if !errors.As(err, &thrown) || thrown.Line != 2 || thrown.Source != "bad.js" {
		t.Fatalf("the error is %+v (%v)", thrown, err)
	}
}

func TestARunawayScriptIsInterrupted(t *testing.T) {
	ctx := open(t, script.Options{Budget: 50 * time.Millisecond})
	start := time.Now()
	_, err := ctx.Run("while (true) {}", "loop.js")
	if !errors.Is(err, script.ErrInterrupted) {
		t.Fatalf("the loop ended with %v", err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the loop ran %s", took)
	}
	if got, failed := ctx.Run("6 * 7", "after.js"); failed != nil || got != 42.0 {
		t.Fatalf("the context after the loop gave %v, %v", got, failed)
	}
}

func TestRunawayMicrotasksAreInterrupted(t *testing.T) {
	ctx := open(t, script.Options{Budget: 50 * time.Millisecond})
	_, err := ctx.Run("function again() { Promise.resolve().then(again) } again()", "tasks.js")
	if !errors.Is(err, script.ErrInterrupted) {
		t.Fatalf("the tasks ended with %v", err)
	}
	if got, failed := ctx.Run("1", "after.js"); failed != nil || got != 1.0 {
		t.Fatalf("the context after the tasks gave %v, %v", got, failed)
	}
}

func TestARunawayFunctionIsInterrupted(t *testing.T) {
	ctx := open(t, script.Options{Budget: 50 * time.Millisecond})
	value, _ := ctx.Run("(function () { for (;;) {} })", "fn.js")
	spin, _ := value.(script.Function)
	if _, err := spin.Call(); !errors.Is(err, script.ErrInterrupted) {
		t.Fatalf("the function ended with %v", err)
	}
}

func TestMicrotasksRunBeforeARunReturns(t *testing.T) {
	ctx := open(t, script.Options{})
	if _, err := ctx.Run("var seen = []; Promise.resolve(1).then(v => seen.push(v)); seen.push(0)", "p.js"); err != nil {
		t.Fatal(err)
	}
	if got, _ := ctx.Run("seen.join()", "p.js"); got != "0,1" {
		t.Fatalf("the tasks ran as %v", got)
	}
}

func TestARejectionNobodyHandlesIsReported(t *testing.T) {
	var reported []*script.Error
	ctx := open(t, script.Options{Rejected: func(err *script.Error) { reported = append(reported, err) }})
	_, err := ctx.Run(`Promise.reject(new Error("handled")).catch(() => {});
Promise.reject(new Error("dropped"));
new Promise((_, no) => no("late")).then(() => {});`, "rejections.js")
	if err != nil {
		t.Fatal(err)
	}
	if len(reported) != 2 || !strings.Contains(reported[0].Message, "dropped") ||
		!strings.Contains(reported[1].Message, "late") {
		t.Fatalf("reported %v", reported)
	}
}

func TestAScriptOverItsMemoryIsStopped(t *testing.T) {
	ctx := open(t, script.Options{Memory: 32 << 20, Budget: 20 * time.Second})
	_, err := ctx.Run(`const hoard = []; for (;;) { hoard.push("x".repeat(1024) + hoard.length) }`, "memory.js")
	if !errors.Is(err, script.ErrOutOfMemory) {
		t.Fatalf("the hoard ended with %v", err)
	}
}

func TestAScriptReachesOnlyWhatIsBound(t *testing.T) {
	ctx := open(t, script.Options{})
	for _, name := range []string{"require", "process", "Deno", "fetch", "document", "window", "setTimeout", "os"} {
		if got, err := ctx.Run("typeof "+name, "probe.js"); err != nil || got != "undefined" {
			t.Errorf("typeof %s is %v, %v", name, got, err)
		}
	}
}

func TestAClosedContextRefusesToRun(t *testing.T) {
	ctx, err := script.New(script.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx.Close()
	ctx.Close()
	if _, err = ctx.Run("1", "closed.js"); !errors.Is(err, script.ErrClosed) {
		t.Fatalf("a closed context ran: %v", err)
	}
}

func TestValuesAreNotKeptAcrossCalls(t *testing.T) {
	ctx := open(t, script.Options{})
	_ = ctx.Bind("host", func(args []any) (any, error) { return args[0], nil })
	before := script.Retained(ctx)
	for range 200 {
		if _, err := ctx.Run(`host({a: 1}); host("s"); [1, 2, 3]`, "loop.js"); err != nil {
			t.Fatal(err)
		}
	}
	if after := script.Retained(ctx); after > before+8 {
		t.Fatalf("%d values are kept after 200 runs, %d before", after, before)
	}
}
