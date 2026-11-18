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
		{"// import('a.js') `tick\nconst t = `import('b.js')`", 0},
		{"const o = { import: (x) => x }; o\n  . import('a.js'); o . import ('b.js')", 0},
		{`const u = '\'import("b.js")', v = "\"import('d.js')", w = /[/]import(x)\//; import('c.js')`, 1},
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

// bundle is a script as bundlers write them: long lines of small
// functions, an import() among them every so many, and here and there
// what only looks like one.
func bundle(imports, between int) string {
	var source strings.Builder
	source.WriteString("var loads=[],texts=[];")
	for i := range imports {
		fmt.Fprintf(&source, "loads.push(()=>import(\"./chunk%d.js\"));", i)
		for j := range between {
			fmt.Fprintf(&source, "function f%d_%d(a,b){const s=`x${a}`;return/re\\//.test(s)?a/b:[a,b].map(v=>v*%d)}", i, j, j)
		}
		if i%8 == 0 {
			source.WriteString("\n")
		}
		switch i % 125 {
		case 7:
			source.WriteString(`texts.push("import('a')");`)
		case 19:
			source.WriteString(`texts.push('import("a")');`)
		case 23:
			source.WriteString("texts.push(`import('a')${1}import('b')`);")
		case 31:
			source.WriteString(`texts.push(/import(a)[import(]/.source);/* import("a") */`)
		case 43:
			source.WriteString("texts.push({import(x){return x}}.import('m'));// import('a')\n")
		}
	}
	return source.String()
}

const bundled = `import('a')|import("a")|import('a')1import('b')|import(a)[import(]|m`

func TestEveryImportCallOfALargeBundleAsksTheImporter(t *testing.T) {
	source := bundle(250, 60)
	if len(source) < 1<<20 {
		t.Fatalf("the bundle is %d KB, want over a megabyte", len(source)>>10)
	}
	var asked []string
	ctx := open(t, script.Options{Import: func(_ string, what script.Import, done func(script.Module, error)) {
		asked = append(asked, what.Specifier)
		done(nil, &script.Error{Message: "TypeError: none"})
	}})
	began := time.Now()
	texts, err := ctx.Run(source+`;loads.forEach((load) => load().catch(() => {}));
[...new Set(texts)].join("|")`, "https://site.test/bundle.js")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d KB with 250 import() ran in %s", len(source)>>10, time.Since(began))
	if len(asked) != 250 || asked[0] != "./chunk0.js" || asked[249] != "./chunk249.js" {
		t.Errorf("the importer was asked %d times, want 250", len(asked))
	}
	if texts != bundled {
		t.Errorf("what only looks like an import() reads %v, want %s", texts, bundled)
	}
	if took := time.Since(began); took > 10*time.Second {
		t.Errorf("it took %s", took)
	}
}

func TestAnImportCallMovesNothingOnItsLine(t *testing.T) {
	source := `var at; function f() { import("a.js").catch(() => {}); import ( "b.js" ).catch(() => {}); ` +
		`at = new Error("here").stack } f(); at`
	ctx := open(t, script.Options{Import: func(_ string, _ script.Import, done func(script.Module, error)) {
		done(nil, &script.Error{Message: "TypeError: none"})
	}})
	stack, err := ctx.Run(source, "https://site.test/*/classic.js")
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("https://site.test/*/classic.js:1:%d", strings.Index(source, "new Error")+1)
	if text, _ := stack.(string); !strings.Contains(text, want) {
		t.Errorf("the error says it is from %v, want %s", stack, want)
	}
}

func TestAnImportCallTheEngineIsNotAskedAboutSaysSo(t *testing.T) {
	var said []string
	ctx := open(t, script.Options{Rejected: func(e *script.Error) { said = append(said, e.Message) }})
	if _, err := ctx.Run(`new Function("return import('a.js')")()`, "https://site.test/classic.js"); err != nil {
		t.Fatal(err)
	}
	if len(said) != 1 || !strings.Contains(said[0], "import() is not supported here") {
		t.Errorf("the rejection says %q", said)
	}
}
