//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package script_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/script"
)

func TestOnlyAnImportCallAsksTheImporter(t *testing.T) {
	for _, c := range []struct {
		source string
		asked  int
	}{
		{`import("a.js")`, 1},
		{`import ( "a.js" )`, 1},
		{"import\n(\"a.js\")", 1},
		{`const load = () => import("a.js"); load(); load()`, 2},
		{`const o = { import(x) { return x } }; o.import("a.js")`, 0},
		{`class Store { import(x) { return x } static import(x) { return x } }
new Store().import("a.js"); Store.import("b.js")`, 0},
		{`const o = { get import() { return (x) => x } }; o.import("a.js"); o?.import("a.js")`, 0},
		{`const $import = (x) => x, _import = $import, reimport = $import;
$import("a.js"); _import("a.js"); reimport("a.js")`, 0},
		{`const s = "import('a.js')" + 'import("a.js")' + ` + "`import('a.js')`", 0},
		{"const s = `${import('a.js') && 1} import('b.js') ${`${import('c.js') && 2}`}`", 2},
		{`const r = /import("a.js")/.test("x") / 2; const d = 4 / 2 / import("a.js").length`, 1},
		{"// import('a.js')\n/* import('a.js') */ 1", 0},
		{`const x = 1
import("a.js")
;[2].forEach(() => import("b.js"))`, 2},
		{`async function f() { await import("a.js"); return import("b.js") } f()`, 1},
		{`label: for (;;) { import("a.js"); break label }`, 1},
		{`import("a.js", { with: { type: "json" } })`, 1},
		{`var a = { "import": 1 }; with (a) { import("a.js") }`, 0},
	} {
		asked := 0
		ctx := open(t, script.Options{Import: func(_ string, _ script.Import, done func(script.Module, error)) {
			asked++
			done(nil, &script.Error{Message: "TypeError: none"})
		}})
		_, err := ctx.Run(c.source+"\n;0", "https://site.test/classic.js")
		if err != nil && c.asked > 0 {
			t.Errorf("%s: %v", c.source, err)
		}
		if asked != c.asked {
			t.Errorf("%s asked the importer %d times, want %d", c.source, asked, c.asked)
		}
		ctx.Close()
	}
}

func TestManyImportCallsOfALargeScriptAreToldApartInTime(t *testing.T) {
	var source strings.Builder
	for i := range 40 {
		fmt.Fprintf(&source, "function load%d() { return import(\"chunk%d.js\") }\n", i, i)
		for j := range 100 {
			fmt.Fprintf(&source, "function f%d_%d(a, b) { const s = `x${a}`; return /re\\//.test(s) ? a / b : "+
				"[a, b].map((v) => v * %d) }\n", i, j, j)
		}
	}
	asked := 0
	ctx := open(t, script.Options{Import: func(_ string, _ script.Import, done func(script.Module, error)) {
		asked++
		done(nil, &script.Error{Message: "TypeError: none"})
	}})
	began := time.Now()
	if _, err := ctx.Run(source.String()+"load0().catch(() => {}); load39().catch(() => {})", "big.js"); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d KB with 40 import() ran in %s", source.Len()>>10, time.Since(began))
	if asked != 2 {
		t.Fatalf("the importer was asked %d times", asked)
	}
	if took := time.Since(began); took > 5*time.Second {
		t.Fatalf("it took %s", took)
	}
}
