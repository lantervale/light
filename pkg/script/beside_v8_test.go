//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package script_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/script"
)

// lend gives to the function from's source makes, under name.
func lend(t *testing.T, from, to script.Context, source, name string) {
	t.Helper()
	made, err := from.Run(source, "test")
	fn, is := made.(script.Function)
	if err != nil || !is {
		t.Fatalf("%s gave %v, %v", source, made, err)
	}
	taken, err := to.Run(`(given) => { globalThis.`+name+` = given; }`, "test")
	take, is := taken.(script.Function)
	if err != nil || !is {
		t.Fatalf("the taker is %v, %v", taken, err)
	}
	if _, err = take.Call(fn); err != nil {
		t.Fatal(err)
	}
}

func TestAContextBesideAnotherSharesItsEngineAndNoGlobal(t *testing.T) {
	engines, contexts := script.Live(), script.Contexts()
	a := open(t, script.Options{Kin: "https://site.test"})
	b, err := script.New(script.Options{Kin: "https://site.test", Beside: a})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if script.Live() != engines+1 || script.Contexts() != contexts+2 {
		t.Fatalf("two contexts of one engine count %d engines and %d contexts more", script.Live()-engines,
			script.Contexts()-contexts)
	}
	if _, err = a.Run(`var secret = "of a"; var kept = { n: 1 };`, "test"); err != nil {
		t.Fatal(err)
	}
	if got, failed := b.Run(`typeof secret + " " + typeof kept + " " + (Object === Object)`, "test"); failed != nil ||
		got != "undefined undefined true" {
		t.Fatalf("a context beside another reads its globals as %v, %v", got, failed)
	}
	lend(t, a, b, `() => kept`, "theirs")
	lend(t, a, b, `() => secret`, "told")
	for source, want := range map[string]string{
		`String(theirs() === theirs())`:                                "true",
		`theirs().n = 5; String(theirs().n)`:                           "5",
		`String(theirs() instanceof Object)`:                           "false",
		`String(Object.getPrototypeOf(theirs()) === Object.prototype)`: "false",
		`told()`: "of a",
	} {
		if got, failed := b.Run(source, "test"); failed != nil || got != want {
			t.Errorf("%s gave %v, %v, want %s", source, got, failed, want)
		}
	}
	if got, failed := a.Run(`String(kept.n)`, "test"); failed != nil || got != "5" {
		t.Errorf("what the other wrote reads %v, %v", got, failed)
	}
	b.Close()
	if script.Live() != engines+1 || script.Contexts() != contexts+1 {
		t.Fatalf("with one of two closed there are %d engines and %d contexts more", script.Live()-engines,
			script.Contexts()-contexts)
	}
	if got, failed := a.Run(`1 + 1`, "test"); failed != nil || got != float64(2) {
		t.Fatalf("the context left gave %v, %v", got, failed)
	}
	if _, failed := b.Run(`1`, "test"); !errors.Is(failed, script.ErrClosed) {
		t.Fatalf("the closed one answered %v", failed)
	}
	a.Close()
	if script.Live() != engines || script.Contexts() != contexts {
		t.Fatalf("with both closed there are %d engines and %d contexts more", script.Live()-engines,
			script.Contexts()-contexts)
	}
}

func TestOnlyKinShareAnEngine(t *testing.T) {
	a := open(t, script.Options{Kin: "https://site.test"})
	alone := open(t, script.Options{})
	for name, opts := range map[string]script.Options{
		"of another origin":  {Kin: "https://other.test", Beside: a},
		"with no kin named":  {Beside: a},
		"beside one of none": {Kin: "https://site.test", Beside: alone},
	} {
		if made, err := script.New(opts); !errors.Is(err, script.ErrNotKin) {
			t.Errorf("a context %s was made beside another: %v", name, err)
			if made != nil {
				made.Close()
			}
		}
	}
	closed, err := script.New(script.Options{Kin: "https://site.test"})
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	if _, err = script.New(script.Options{Kin: "https://site.test", Beside: closed}); !errors.Is(err, script.ErrClosed) {
		t.Errorf("a context beside a closed one: %v", err)
	}
}

func TestTheBudgetIsTheEnginesWhicheverContextSpins(t *testing.T) {
	var rejected []string
	a := open(t, script.Options{Kin: "k", Budget: 150 * time.Millisecond, Rejected: func(e *script.Error) {
		rejected = append(rejected, "a: "+e.Message)
	}})
	b, err := script.New(script.Options{Kin: "k", Beside: a, Budget: time.Hour, Rejected: func(e *script.Error) {
		rejected = append(rejected, "b: "+e.Message)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	lend(t, b, a, `() => { for (;;) {} }`, "spin")
	lend(t, b, a, `() => { Promise.reject(new Error("of b")); }`, "reject")
	began := time.Now()
	if _, err = a.Run(`spin()`, "test"); !errors.Is(err, script.ErrInterrupted) || time.Since(began) > time.Second {
		t.Fatalf("a function of the other context that never ends ended with %v after %s", err, time.Since(began))
	}
	for _, c := range []script.Context{a, b} {
		if got, failed := c.Run(`1 + 1`, "test"); failed != nil || got != float64(2) {
			t.Fatalf("after one was stopped a context gave %v, %v", got, failed)
		}
	}
	if _, err = a.Run(`reject(); Promise.reject(new Error("of a"));`, "test"); err != nil {
		t.Fatal(err)
	}
	slices.Sort(rejected)
	if got := strings.Join(rejected, ", "); got != "a: Uncaught (in promise) Error: of a, b: Uncaught (in promise) "+
		"Error: of b" {
		t.Errorf("the rejections nobody handled were told as %q", got)
	}
	a.Close()
	a.Interrupt()
	if got, failed := b.Run(`2 + 2`, "test"); failed != nil || got != float64(4) {
		t.Fatalf("after the other closed and was interrupted a context gave %v, %v", got, failed)
	}
}

func TestWhatAClosedContextLeftItsKinStillAnswers(t *testing.T) {
	a := open(t, script.Options{Kin: "https://site.test"})
	b, err := script.New(script.Options{Kin: "https://site.test", Beside: a})
	if err != nil {
		t.Fatal(err)
	}
	asked := 0
	if err = b.Bind("ask", func([]any) (any, error) {
		asked++
		return "answered", nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Run(`var kept = { said: "kept" };`, "test"); err != nil {
		t.Fatal(err)
	}
	lend(t, b, a, `() => kept.said`, "plain")
	lend(t, b, a, `() => ask()`, "bound")
	lend(t, b, a, `() => ask`, "taken")
	lend(t, b, a, `() => import("/gone.js")`, "imports")
	if got, failed := a.Run(`bound() + " " + taken()()`, "test"); failed != nil || got != "answered answered" {
		t.Fatalf("a function light bound for a context answers its kin %v, %v", got, failed)
	}
	b.Close()
	said := `(fn) => { try { return String(fn()); } catch (e) { return "throws " + e.message; } }`
	for source, want := range map[string]string{
		`(` + said + `)(plain)`:   "kept",
		`(` + said + `)(bound)`:   "throws light: the document this was of is gone",
		`(` + said + `)(taken())`: "throws light: the document this was of is gone",
		`(` + said + `)(imports)`: "throws light: the document this was of is gone",
	} {
		if got, failed := a.Run(source, "test"); failed != nil || got != want {
			t.Errorf("%s gave %v, %v once its context closed, want %s", source, got, failed, want)
		}
	}
	if asked != 2 {
		t.Errorf("the function a closed context bound was asked %d times, want 2", asked)
	}
}
