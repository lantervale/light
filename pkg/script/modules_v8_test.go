//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package script_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/script"
)

// graph is modules by their address, read as they are asked for.
type graph struct {
	t       *testing.T
	ctx     script.Context
	sources map[string]string
	read    map[string]script.Module
	said    []string
	// asked answers import(), which is rejected without it.
	asked func(from string, what script.Import, done func(script.Module, error))
	// rejected are the rejections nobody handled.
	rejected []*script.Error
}

func modules(t *testing.T, sources map[string]string) *graph {
	t.Helper()
	g := &graph{t: t, sources: sources, read: map[string]script.Module{}}
	g.ctx = open(t, script.Options{
		Rejected: func(err *script.Error) { g.rejected = append(g.rejected, err) },
		Import: func(from string, what script.Import, done func(script.Module, error)) {
			if g.asked == nil {
				done(nil, &script.Error{Message: "TypeError: no module is imported here"})
				return
			}
			g.asked(from, what, done)
		},
		Resolve: func(from, specifier string) (string, error) {
			if specifier == "bare" {
				return "", &script.Error{Message: "TypeError: bare is no address"}
			}
			return from + "/" + specifier, nil
		},
	})
	if err := g.ctx.Bind("say", func(args []any) (any, error) {
		g.said = append(g.said, strings.TrimSpace(fmt.Sprintln(args...)))
		return len(g.said), nil
	}); err != nil {
		t.Fatal(err)
	}
	return g
}

// module reads the module at address and those it imports, by their
// specifiers as addresses.
func (g *graph) module(address string) script.Module {
	g.t.Helper()
	if m, known := g.read[address]; known {
		return m
	}
	var m script.Module
	var err error
	if strings.HasSuffix(address, ".json") {
		m, err = g.ctx.JSON(g.sources[address], address)
	} else {
		m, err = g.ctx.Module(g.sources[address], address)
	}
	if err != nil {
		g.t.Fatalf("%s: %v", address, err)
	}
	g.read[address] = m
	for _, need := range m.Imports() {
		g.module(need.Specifier)
	}
	return m
}

func (g *graph) resolve(_ script.Module, what script.Import) (script.Module, error) {
	return g.read[what.Specifier], nil
}

// run links and runs the module at address, and gives how it ended.
func (g *graph) run(address string) error {
	g.t.Helper()
	m := g.module(address)
	if err := m.Link(g.resolve); err != nil {
		return err
	}
	ended, result := false, error(nil)
	m.Run(func(err error) { ended, result = true, err })
	if !ended {
		g.t.Fatalf("%s has not ended", address)
	}
	return result
}

func (g *graph) heard() string { return strings.Join(g.said, " | ") }

func TestAModuleSaysWhatItImports(t *testing.T) {
	g := modules(t, nil)
	m, err := g.ctx.Module(`import a, { b as c } from "./a.js";
import * as all from "/b.js";
import "https://other.test/c.js";
import data from "./d.json" with { type: "json" };
export { e } from "bare";
export const mine = 1;`, "https://site.test/app.js")
	if err != nil {
		t.Fatal(err)
	}
	want := []script.Import{{Specifier: "./a.js"}, {Specifier: "/b.js"}, {Specifier: "https://other.test/c.js"},
		{Specifier: "./d.json", Type: "json"}, {Specifier: "bare"}}
	if got := m.Imports(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("it imports %v, want %v", got, want)
	}
	alone, err := g.ctx.Module(`export const one = 1;`, "https://site.test/alone.js")
	if err != nil || len(alone.Imports()) != 0 {
		t.Fatalf("a module that imports nothing imports %v, %v", alone.Imports(), err)
	}
}

func TestModulesRunOnceInTheOrderOfTheirImports(t *testing.T) {
	g := modules(t, map[string]string{
		"app.js":    `import "left.js"; import "right.js"; say("app");`,
		"left.js":   `import { n } from "shared.js"; say("left", n);`,
		"right.js":  `import { n } from "shared.js"; say("right", n);`,
		"shared.js": `say("shared"); export const n = 7;`,
		"again.js":  `import { n } from "shared.js"; say("again", n);`,
	})
	if err := g.run("app.js"); err != nil {
		t.Fatal(err)
	}
	if err := g.run("again.js"); err != nil {
		t.Fatal(err)
	}
	if got, want := g.heard(), "shared | left 7 | right 7 | app | again 7"; got != want {
		t.Fatalf("the modules ran as %q, want %q", got, want)
	}
}

func TestBindingsAreLiveAndCyclesLink(t *testing.T) {
	g := modules(t, map[string]string{
		"a.js": `import { b, grow } from "b.js"; export function name() { return "a" }
say("a sees", b); grow(); say("a sees", b);`,
		"b.js": `import { name } from "a.js"; export let b = 1; export function grow() { b += 1 }
say("b sees", name());`,
	})
	if err := g.run("a.js"); err != nil {
		t.Fatal(err)
	}
	if got, want := g.heard(), "b sees a | a sees 1 | a sees 2"; got != want {
		t.Fatalf("the cycle ran as %q, want %q", got, want)
	}
}

func TestAModuleKnowsItsAddressAndIsStrict(t *testing.T) {
	g := modules(t, map[string]string{
		"https://site.test/dir/app.js": "say(import.meta.url, typeof this, import.meta.resolve('./next.js'));\n" +
			"try { undeclared = 1 } catch (e) { say(e.name) }\n" +
			"try { import.meta.resolve('bare') } catch (e) { say(e.name, e.message) }",
		"https://site.test/\"quoted\".js": `say(import.meta.url)`,
		"https://site.test/hash.js":       "#!/usr/bin/env node\nsay(import.meta.url)",
	})
	for address := range g.sources {
		if err := g.run(address); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{
		"https://site.test/dir/app.js undefined https://site.test/dir/app.js/./next.js", "ReferenceError",
		"TypeError bare is no address",
		`https://site.test/"quoted".js`, "https://site.test/hash.js"} {
		if !strings.Contains(g.heard(), want) {
			t.Errorf("the modules said %q, without %q", g.heard(), want)
		}
	}
}

func TestAnAwaitAtTheTopHoldsTheModulesThatImportIt(t *testing.T) {
	g := modules(t, map[string]string{
		"app.js":  `import { late } from "slow.js"; say("app", late);`,
		"slow.js": `say("slow begins"); export let late = "early"; await gate; late = "late"; say("slow ends");`,
	})
	if _, err := g.ctx.Run(`var open; var gate = new Promise((yes) => { open = yes })`, "gate.js"); err != nil {
		t.Fatal(err)
	}
	m := g.module("app.js")
	if err := m.Link(g.resolve); err != nil {
		t.Fatal(err)
	}
	ended := 0
	m.Run(func(err error) {
		if err != nil {
			t.Errorf("the module ended with %v", err)
		}
		ended++
	})
	if ended != 0 || g.heard() != "slow begins" {
		t.Fatalf("before the gate opened the modules said %q and ended %d times", g.heard(), ended)
	}
	if _, err := g.ctx.Run(`open()`, "gate.js"); err != nil {
		t.Fatal(err)
	}
	if ended != 1 || g.heard() != "slow begins | slow ends | app late" {
		t.Fatalf("after it the modules said %q and ended %d times", g.heard(), ended)
	}
}

func TestAModuleThatThrowsSaysWhereAndFailsThoseThatImportIt(t *testing.T) {
	g := modules(t, map[string]string{
		"https://site.test/app.js":  `import "https://site.test/deep.js"; say("app ran");`,
		"https://site.test/deep.js": "say('deep');\nfunction fail() { throw new TypeError('no luck') }\nfail();",
		"https://site.test/late.js": "await 0;\nthrow new Error('after an await');",
		"https://site.test/next.js": `say("next ran");`,
	})
	var thrown *script.Error
	err := g.run("https://site.test/app.js")
	if !errors.As(err, &thrown) || thrown.Message != "TypeError: no luck" ||
		!strings.Contains(thrown.Stack, "https://site.test/deep.js:2:") {
		t.Fatalf("the module ended with %v, its stack %q", err, thrown.Stack)
	}
	err = g.run("https://site.test/late.js")
	if !errors.As(err, &thrown) || thrown.Message != "Error: after an await" ||
		!strings.Contains(thrown.Stack, "https://site.test/late.js:2:") {
		t.Fatalf("the module that waited ended with %v", err)
	}
	if err = g.run("https://site.test/next.js"); err != nil || g.heard() != "deep | next ran" {
		t.Fatalf("the next module ended with %v after %q", err, g.heard())
	}
	if len(g.rejected) != 0 {
		t.Fatalf("what the modules threw was also reported as %d rejections", len(g.rejected))
	}
}

func TestAModuleThatDoesNotReadSaysSo(t *testing.T) {
	g := modules(t, nil)
	var thrown *script.Error
	_, err := g.ctx.Module("const fine = 1;\nconst = broken;", "https://site.test/bad.js")
	if !errors.As(err, &thrown) || !strings.HasPrefix(thrown.Message, "SyntaxError") ||
		thrown.Source != "https://site.test/bad.js" || thrown.Line != 2 {
		t.Fatalf("a module with an error in its second line gave %+v (%v)", thrown, err)
	}
	_, err = g.ctx.Module("const fine = 1 +;", "https://site.test/first.js")
	if !errors.As(err, &thrown) || thrown.Line != 1 || thrown.Column != 17 {
		t.Fatalf("a module with an error in its first line gave %+v (%v)", thrown, err)
	}
	_, err = g.ctx.Module("import { a } from './a.js';\nconst = broken;", "https://site.test/after.js")
	if !errors.As(err, &thrown) || !strings.HasPrefix(thrown.Message, "SyntaxError") ||
		thrown.Source != "https://site.test/after.js" {
		t.Fatalf("a module with an error after an import gave %+v (%v)", thrown, err)
	}
	if got, failed := g.ctx.Run("1 + 1", "after.js"); failed != nil || got != 2.0 {
		t.Fatalf("the context after them gave %v, %v", got, failed)
	}
}

func TestAnImportNoModuleExportsFailsTheLink(t *testing.T) {
	g := modules(t, map[string]string{
		"https://site.test/app.js": "import { one } from 'https://site.test/lib.js';\n" +
			"import { missing } from 'https://site.test/lib.js';",
		"https://site.test/lib.js": `export const one = 1; say("lib ran");`,
		"https://site.test/ok.js":  `import { one } from 'https://site.test/lib.js'; say("ok", one);`,
	})
	var thrown *script.Error
	err := g.run("https://site.test/app.js")
	if !errors.As(err, &thrown) || !strings.Contains(thrown.Message, "does not provide an export named 'missing'") ||
		thrown.Source != "https://site.test/app.js" || thrown.Line != 2 {
		t.Fatalf("the link failed with %+v (%v)", thrown, err)
	}
	if err = g.run("https://site.test/ok.js"); err != nil || g.heard() != "lib ran | ok 1" {
		t.Fatalf("a module importing the same one ended with %v after %q", err, g.heard())
	}
}

func TestAJSONModuleExportsItsValue(t *testing.T) {
	g := modules(t, map[string]string{
		"app.js": `import data from "data.json" with { type: "json" };
say(data.list.length, data["__proto__"], Object.keys(data).join());`,
		"data.json": `{"list": [1, 2, 3], "__proto__": "own", "line": "a b"}`,
	})
	if err := g.run("app.js"); err != nil {
		t.Fatal(err)
	}
	if got, want := g.heard(), "3 own list,__proto__,line"; got != want {
		t.Fatalf("the module read %q, want %q", got, want)
	}
	var thrown *script.Error
	if _, err := g.ctx.JSON(`{"cut": `, "https://site.test/cut.json"); !errors.As(err, &thrown) ||
		!strings.HasPrefix(thrown.Message, "SyntaxError") || thrown.Source != "https://site.test/cut.json" {
		t.Fatalf("JSON cut short gave %+v (%v)", thrown, err)
	}
}

// importer answers import() from the graph, at once or when told.
func (g *graph) importer(hold *[]func()) {
	g.asked = func(from string, what script.Import, done func(script.Module, error)) {
		g.said = append(g.said, "import "+what.Specifier+" "+what.Type+" from "+from)
		answer := func() {
			if _, known := g.sources[what.Specifier]; !known {
				done(nil, &script.Error{
					Message: "TypeError: Failed to fetch dynamically imported module: " + what.Specifier,
				})
				return
			}
			m := g.module(what.Specifier)
			if err := m.Link(g.resolve); err != nil {
				done(nil, err)
				return
			}
			m.Run(func(err error) { done(m, err) })
		}
		if hold == nil {
			answer()
			return
		}
		*hold = append(*hold, answer)
	}
}

func TestImportCallsAskTheImporter(t *testing.T) {
	g := modules(t, map[string]string{
		"https://site.test/app.js": `const lib = await import("lib.js"); say("app got", lib.default, lib.named);
import("data.json", { with: { type: "json" } }).then((m) => say("json", m.default.n));
import("nowhere.js").catch((e) => say(e.name, e.message));
import("throws.js").catch((e) => say("caught", e === globalThis.thrown));
const text = "import('never.js')"; const re = /import("never.js")/; // import("never.js")
const o = { import(x) { return "method " + x } }; say(o.import("called"), text.length, re.source.length);
say(` + "`in a ${typeof import(\"lib.js\").then} template import('never.js')`" + `);`,
		"lib.js":    `export default "lib"; export const named = "named"; say("lib ran");`,
		"data.json": `{"n": 5}`,
		"throws.js": `globalThis.thrown = new Error("thrown by a module"); throw globalThis.thrown;`,
	})
	g.importer(nil)
	if err := g.run("https://site.test/app.js"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"import lib.js  from https://site.test/app.js | lib ran | app got lib named",
		"import data.json json from https://site.test/app.js", "json 5",
		"TypeError Failed to fetch dynamically imported module: nowhere.js", "caught true", "method called 18 18",
		"in a function template import('never.js')",
	} {
		if !strings.Contains(g.heard(), want) {
			t.Errorf("the module said %q, without %q", g.heard(), want)
		}
	}
	if strings.Contains(g.heard(), "never.js from") {
		t.Errorf("an import in a text, a pattern or a comment was asked for: %q", g.heard())
	}
}

func TestAClassicScriptImportsToo(t *testing.T) {
	g := modules(t, map[string]string{"lib.js": `export const n = 3;`})
	var held []func()
	g.importer(&held)
	if _, err := g.ctx.Run(`var got = "nothing";
import("lib.js").then((m) => { got = m.n }, (e) => { got = String(e) });
import("gone.js").catch((e) => say("gone", e instanceof TypeError));`, "https://site.test/classic.js"); err != nil {
		t.Fatal(err)
	}
	if got, _ := g.ctx.Run("got", "probe.js"); got != "nothing" || len(held) != 2 {
		t.Fatalf("before its module came the script had %v, %d imports asked", got, len(held))
	}
	if g.said[0] != "import lib.js  from https://site.test/classic.js" {
		t.Fatalf("the import was asked as %q", g.said[0])
	}
	for _, answer := range held {
		answer()
	}
	got, err := g.ctx.Run("got", "probe.js")
	if err != nil {
		t.Fatal(err)
	}
	if got != 3.0 || !strings.Contains(g.heard(), "gone true") {
		t.Fatalf("after it the script had %v, and said %q", got, g.heard())
	}
}

func TestAScriptWithNoImportCallIsRunAsWritten(t *testing.T) {
	g := modules(t, nil)
	source := `(function important(a) { return important.toString().length })`
	fn, err := g.ctx.Run(source, "plain.js")
	if err != nil {
		t.Fatal(err)
	}
	read, ok := fn.(script.Function)
	if !ok {
		t.Fatalf("the script gave a %T, want a function", fn)
	}
	size, err := read.Call()
	if err != nil || size != float64(len(source)-2) {
		t.Fatalf("the function reads itself as %v characters, wrote %d", size, len(source)-2)
	}
}

func TestWithoutAnImporterAnImportCallIsRejected(t *testing.T) {
	ctx := open(t, script.Options{})
	_, err := ctx.Run(`var said; import("lib.js").catch((e) => { said = e instanceof Error })`, "classic.js")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := ctx.Run("said", "probe.js"); got != true {
		t.Fatalf("the import ended with %v", got)
	}
}

func TestModulesOfAClosedContextDoNothing(t *testing.T) {
	g := modules(t, map[string]string{"app.js": `say("ran")`})
	m := g.module("app.js")
	g.ctx.Close()
	if err := m.Link(g.resolve); !errors.Is(err, script.ErrClosed) {
		t.Fatalf("a module of a closed context linked: %v", err)
	}
	var ended error
	m.Run(func(err error) { ended = err })
	if !errors.Is(ended, script.ErrClosed) || g.heard() != "" {
		t.Fatalf("a module of a closed context ran: %v, %q", ended, g.heard())
	}
	if _, err := g.ctx.Module(`1`, "late.js"); !errors.Is(err, script.ErrClosed) {
		t.Fatalf("a closed context read a module: %v", err)
	}
}
