//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package script

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	v8 "github.com/gost-dom/v8go"
)

// The engine's binding compiles, links and runs modules, and no more:
// it says neither what a module imports before it is linked, nor why
// one does not compile, and has no hook for import() or import.meta.
// What it lacks is made up for here, with the engine's own parser as
// the judge of every change to a source.

const (
	// importer is the function an import() is a call of.
	importer = "__light_import__"
	// unlinkedSource is a module that never links, and unlinkedName the
	// module it imports.
	unlinkedSource = `import "light:unlinked"`
	unlinkedName   = "light:unlinked"
	// strict has a classic script read as a module is.
	strict = `"use strict";`
	// probed is how much source the import() of one script may have
	// read again to be told from what only looks like them.
	probed = 512 << 20
	// unsupported is what the engine rejects an import() it was not
	// given by light with, and unreached what light says of it.
	unsupported = "Error: Not supported"
	unreached   = "Error: import() is not supported here: light answers the import() of a page's scripts and " +
		"modules, not one of eval, of new Function, of an on… attribute, of the Console's line or of a script " +
		"that is not strict code"
)

// modules are the modules of a context.
type modules struct {
	// read are the modules read, by the engine's number for each.
	read map[int]*module
	// unlinked stands for every import of a module asked what it
	// imports.
	unlinked *v8.Module
	// callers are the names import() is a call of, by the address of
	// the script it is called from.
	callers map[string]string
}

// module is a module the engine compiled.
type module struct {
	c       *context
	mod     *v8.Module
	imports []Import
}

type resolver func(spec string, attributes v8.ImportAttributes, from *v8.Module) (*v8.Module, error)

func (r resolver) ResolveModule(_ *v8.Context, spec string, attributes v8.ImportAttributes,
	from *v8.Module) (*v8.Module, error) {
	return r(spec, attributes, from)
}

// importing gives the context the function import() calls.
func (c *context) importing() error {
	c.read = map[int]*module{}
	define, err := c.ctx.RunScript(`(call, resolve) => {
	Object.defineProperty(call, "resolve", { value: resolve });
	Object.defineProperty(call, "from", { value: (name, from) => {
		Object.defineProperty(globalThis, name, { value: (...asked) => call(from, ...asked) });
	} });
	Object.defineProperty(globalThis, "`+importer+`", { value: call });
}`, "light:import")
	if err != nil {
		return err
	}
	defer define.Release()
	fn, err := define.AsFunction()
	if err != nil {
		return err
	}
	call := v8.NewFunctionTemplateWithError(c.iso, c.imported).GetFunction(c.ctx)
	resolve := v8.NewFunctionTemplateWithError(c.iso, c.resolved).GetFunction(c.ctx)
	defer call.Release()
	defer resolve.Release()
	_, err = fn.Call(v8.Undefined(c.iso), call, resolve)
	return err
}

func (m *modules) close() {
	for _, kept := range m.read {
		kept.mod.Delete()
	}
	if m.unlinked != nil {
		m.unlinked.Delete()
	}
	m.read, m.unlinked = nil, nil
}

func (c *context) Module(source, address string) (Module, error) {
	if c.ctx == nil {
		return nil, ErrClosed
	}
	mod, err := v8.CompileModule(c.iso, c.headed(c.calls(source, address), address), address)
	if err != nil {
		return nil, c.unread(source, address)
	}
	return c.keep(mod), nil
}

func (c *context) JSON(text, address string) (Module, error) {
	if c.ctx == nil {
		return nil, ErrClosed
	}
	if !json.Valid([]byte(text)) {
		return nil, &Error{Message: "SyntaxError: " + address + " is not valid JSON", Source: address}
	}
	quoted, err := json.Marshal(text)
	if err != nil {
		return nil, err
	}
	mod, err := v8.CompileModule(c.iso, "export default JSON.parse("+string(quoted)+");", address)
	if err != nil {
		return nil, &Error{Message: "SyntaxError: " + address + " is not valid JSON", Source: address}
	}
	return c.keep(mod), nil
}

func (c *context) keep(mod *v8.Module) *module {
	m := &module{c: c, mod: mod, imports: c.imports(mod)}
	c.read[mod.ScriptID()] = m
	return m
}

// headed is the source of a module that begins by saying what its
// import.meta holds, on its first line so that every line keeps its
// number.
func (c *context) headed(source, address string) string {
	quoted, err := json.Marshal(address)
	if err != nil {
		return source
	}
	head := "import.meta.url=" + string(quoted) + ";"
	if c.opts.Resolve != nil {
		head += "import.meta.resolve=(specifier)=>" + importer + ".resolve(" + string(quoted) + ",specifier);"
	}
	if rest, hashbang := strings.CutPrefix(source, "#!"); hashbang {
		return head + "//" + rest
	}
	return head + source
}

// imports asks the engine what a module imports: linking it to a module
// that never links has it name them all, and leaves it unlinked.
func (c *context) imports(mod *v8.Module) []Import {
	if c.unlinked == nil {
		unlinked, err := v8.CompileModule(c.iso, unlinkedSource, unlinkedName)
		if err != nil {
			return nil
		}
		c.unlinked = unlinked
	}
	var found []Import
	_ = mod.InstantiateModule(c.ctx, resolver(func(spec string, attributes v8.ImportAttributes,
		from *v8.Module) (*v8.Module, error) {
		if from.ScriptID() == c.unlinked.ScriptID() {
			return nil, errors.New("it never links")
		}
		found = append(found, Import{Specifier: spec, Type: typeOf(c.ctx, attributes)})
		return c.unlinked, nil
	}))
	return found
}

func typeOf(ctx *v8.Context, attributes v8.ImportAttributes) string {
	for _, a := range attributes.All(ctx) {
		if a.Key == "type" {
			return a.Value
		}
	}
	return ""
}

// unread is why a module does not compile. The binding does not say: a
// classic script of the same text does, when what stops it is not what
// only a module may hold.
func (c *context) unread(source, address string) error {
	unknown := &Error{Message: "SyntaxError: the module is not read, and the engine does not say where it stops",
		Source: address}
	if rest, hashbang := strings.CutPrefix(source, "#!"); hashbang {
		source = "//" + rest
	}
	_, err := c.iso.CompileUnboundScript(strict+source, address, v8.CompileOptions{})
	var why *Error
	if err == nil || !errors.As(c.failure(err, true), &why) || why.Line == 0 {
		return unknown
	}
	for _, modular := range []string{"outside a module", "'export'", "await is only valid", "reserved word"} {
		if strings.Contains(why.Message, modular) {
			return unknown
		}
	}
	if why.Line == 1 {
		why.Column -= len(strict)
	}
	why.Stack = ""
	return why
}

func (m *module) Imports() []Import { return m.imports }

func (m *module) Link(resolve func(from Module, what Import) (Module, error)) error {
	c := m.c
	if c.ctx == nil {
		return ErrClosed
	}
	err := m.mod.InstantiateModule(c.ctx, resolver(func(spec string, attributes v8.ImportAttributes,
		from *v8.Module) (*v8.Module, error) {
		to, err := resolve(c.read[from.ScriptID()], Import{Specifier: spec, Type: typeOf(c.ctx, attributes)})
		if err != nil {
			return nil, err
		}
		linked, ok := to.(*module)
		if !ok || linked.c != c {
			return nil, errors.New("it is no module of this page")
		}
		return linked.mod, nil
	}))
	if err != nil {
		return c.failure(err, true)
	}
	return nil
}

func (m *module) Run(done func(error)) {
	c := m.c
	if c.ctx == nil {
		done(ErrClosed)
		return
	}
	ended, waits := false, false
	end := func(err error) {
		if !ended {
			ended = true
			done(err)
		}
	}
	var thrown error
	err := c.within(func() error {
		value, err := m.mod.Evaluate(c.ctx)
		if err != nil {
			return err
		}
		defer value.Release()
		promise, err := value.AsPromise()
		if err != nil {
			return err
		}
		switch promise.State() {
		case v8.Rejected:
			c.handled(promise)
			thrown = c.caught(promise.Result())
		case v8.Pending:
			waits = true
			promise.Then(func(info *v8.FunctionCallbackInfo) *v8.Value {
				info.Release()
				end(nil)
				return nil
			}, func(info *v8.FunctionCallbackInfo) *v8.Value {
				var reason *v8.Value
				if args := info.Args(); len(args) > 0 {
					reason = args[0]
				}
				end(c.caught(reason))
				return nil
			}).Release()
		case v8.Fulfilled:
		}
		return nil
	})
	switch {
	case err != nil:
		end(err)
	case thrown != nil:
		end(thrown)
	case !waits:
		end(nil)
	}
}

// handled forgets a rejected promise whose rejection is told otherwise.
func (c *context) handled(promise *v8.Promise) {
	for i, r := range c.rejected {
		if r.promise.SameValue(promise.Value) {
			r.promise.Release()
			c.rejected = append(c.rejected[:i], c.rejected[i+1:]...)
			return
		}
	}
}

// thrownError is the Error of a value a module threw, kept with the
// value: an import() of the module is rejected with it.
type thrownError struct {
	told  Error
	value *v8.Value
}

func (e *thrownError) Error() string { return e.told.Error() }

func (e *thrownError) Unwrap() error { return &e.told }

// caught is the error of a value a module threw.
func (c *context) caught(value *v8.Value) error {
	if value == nil {
		return &Error{Message: "undefined"}
	}
	e := &thrownError{told: Error{Message: value.DetailString()}, value: value}
	if object, err := value.AsObject(); err == nil && value.IsNativeError() {
		if stack, failed := object.Get("stack"); failed == nil {
			e.told.Stack = stack.String()
			stack.Release()
		}
	}
	return e
}

// imported is an import(): the address it is called from, what it
// names and its options. It gives the promise of the module.
func (c *context) imported(info *v8.FunctionCallbackInfo) (*v8.Value, error) {
	defer info.Release()
	promised, err := v8.NewPromiseResolver(c.ctx)
	if err != nil {
		return nil, err
	}
	args := info.Args()
	from, what := "", Import{}
	if len(args) > 0 {
		from = args[0].String()
	}
	if len(args) > 1 {
		what.Specifier = args[1].String()
	}
	if len(args) > 2 {
		what.Type = c.asked(args[2])
	}
	answer := func(m Module, err error) {
		if c.ctx == nil {
			return
		}
		_ = c.within(func() error {
			c.settleImport(promised, m, err)
			return nil
		})
	}
	if c.opts.Import == nil {
		answer(nil, &Error{Message: "Error: Not supported"})
	} else {
		c.opts.Import(from, what, answer)
	}
	return promised.GetPromise().Value, nil
}

// asked is the type the options of an import() name.
func (c *context) asked(options *v8.Value) string {
	if !options.IsObject() {
		return ""
	}
	text, err := v8.JSONStringify(c.ctx, options)
	if err != nil {
		return ""
	}
	var read struct {
		With struct {
			Type string `json:"type"`
		} `json:"with"`
	}
	if json.Unmarshal([]byte(text), &read) != nil {
		return ""
	}
	return read.With.Type
}

// settleImport gives an import() its module's namespace, or rejects it
// with what the module threw, else with an error that says why.
func (c *context) settleImport(promised *v8.PromiseResolver, m Module, err error) {
	read, ok := m.(*module)
	if err == nil && (!ok || read.c != c) {
		err = &Error{Message: "TypeError: it is no module of this page"}
	}
	if err == nil {
		namespace := read.mod.GetModuleNamespace()
		promised.Resolve(namespace)
		return
	}
	var thrown *thrownError
	if errors.As(err, &thrown) {
		promised.Reject(thrown.value)
		return
	}
	if value := c.errorOf(err.Error()); value != nil {
		promised.Reject(value)
		value.Release()
	}
}

// errorOf is an Error of the page for a message, of the kind the
// message begins with.
func (c *context) errorOf(text string) *v8.Value {
	kind, rest, named := strings.Cut(text, ": ")
	if !named || !strings.HasSuffix(kind, "Error") {
		kind, rest = "Error", text
	}
	message, err := v8.NewValue(c.iso, rest)
	if err != nil {
		return nil
	}
	defer message.Release()
	name, err := v8.NewValue(c.iso, kind)
	if err != nil {
		return nil
	}
	defer name.Release()
	made, err := c.errors.Call(v8.Undefined(c.iso), message, name)
	if err != nil {
		return nil
	}
	return made
}

// typed is an exception of the page for a message, of the kind the
// message begins with.
func (c *context) typed(text string) error {
	thrown := c.errorOf(text)
	if thrown == nil {
		return errors.New(text)
	}
	c.answered = thrown
	return &v8.Exception{Value: thrown}
}

// resolved is import.meta.resolve: the address of the module, and a
// specifier.
func (c *context) resolved(info *v8.FunctionCallbackInfo) (*v8.Value, error) {
	defer info.Release()
	c.forget()
	args := info.Args()
	if len(args) < 2 || c.opts.Resolve == nil {
		return nil, c.typed("TypeError: import.meta.resolve is given a specifier")
	}
	address, err := c.opts.Resolve(args[0].String(), args[1].String())
	if err != nil {
		return nil, c.typed(err.Error())
	}
	value, err := v8.NewValue(c.iso, address)
	c.answered = value
	return value, err
}

// importCall is what an import() begins with.
var importCall = regexp.MustCompile(`import\s*\(`)

// What stands where an import stands when the engine is asked whether
// it is a call: each is one in the engine's eyes there, and in no other
// place an import( is written in.
const (
	// called is no name of a method, and ends the line of a pattern.
	called = "`   `\n"
	// quoted ends a text in double quotes, and a template on a
	// substitution that is none; single does so of one in single quotes.
	quoted = `"${)" `
	single = `'${)' `
)

// calls is source with each import() a call of a function of the
// context that knows the address it is called from: the binding has no
// hook for them. The function's name is as long as the word, so that
// nothing moves on its line. An import that only looks like one, a
// method of that name or the words in a text, is left as written, and
// the engine's parser tells them apart.
func (c *context) calls(source, from string) string {
	if !strings.Contains(source, "import") {
		return source
	}
	var found [][]int
	for _, at := range importCall.FindAllStringIndex(source, -1) {
		before, _ := utf8.DecodeLastRuneInString(strings.TrimRightFunc(source[:at[0]], unicode.IsSpace))
		if at[0] == 0 || (!named(rune(source[at[0]-1])) && before != '.') {
			found = append(found, at)
		}
	}
	p := probe{c: c, source: source, left: probed}
	found = p.calls(found)
	if p.left < 0 && c.opts.Rejected != nil {
		c.opts.Rejected(&Error{Source: from, Message: "light read this script again as often as it does to tell " +
			"its import() from what looks like one: those left are rejected as not supported."})
	}
	name := c.caller(from)
	if len(found) == 0 || name == "" {
		return source
	}
	var out strings.Builder
	rest := 0
	for _, at := range found {
		out.WriteString(source[rest:at[0]])
		out.WriteString(name)
		rest = at[0] + len("import")
	}
	out.WriteString(source[rest:])
	return out.String()
}

// caller is the name of the function an import() of the script at an
// address is a call of: a word of six letters no page writes, made the
// first time it is asked for, "" when none is left.
func (c *context) caller(from string) string {
	if name, made := c.callers[from]; made {
		return name
	}
	number := strconv.FormatInt(int64(len(c.callers)), 36)
	quoted, err := json.Marshal(from)
	if err != nil || len(number) > 4 {
		return ""
	}
	name := "ªº" + strings.Repeat("0", 4-len(number)) + number
	made, err := c.ctx.RunScript(importer+`.from("`+name+`",`+string(quoted)+`)`, "light:import")
	if err != nil {
		return ""
	}
	made.Release()
	if c.callers == nil {
		c.callers = map[string]string{}
	}
	c.callers[from] = name
	return name
}

// probe asks the engine which import( of a source are calls, a whole
// set of them at once: left is how much source it may still read.
type probe struct {
	c      *context
	source string
	left   int
}

// calls are those of found that are import() calls. A word that reads
// with every import replaced reads with any of them replaced, so only
// the words that do not are asked again, of half the set and of half
// of that.
func (p *probe) calls(found [][]int) [][]int {
	var words []string
	for _, word := range []string{called, quoted, single} {
		if len(found) > 0 && !p.reads(word, found) {
			words = append(words, word)
		}
	}
	return p.sift(found, words)
}

func (p *probe) sift(found [][]int, words []string) [][]int {
	if len(found) == 0 || len(words) == 0 {
		return found
	}
	if p.all(found, words) {
		return found
	}
	if len(found) == 1 {
		return nil
	}
	half := len(found) / 2
	return slices.Concat(p.sift(found[:half], words), p.sift(found[half:], words))
}

func (p *probe) all(found [][]int, words []string) bool {
	for _, word := range words {
		if !p.reads(word, found) {
			return false
		}
	}
	return true
}

// reads reports whether the engine reads the source with word where
// each import of found stands.
func (p *probe) reads(word string, found [][]int) bool {
	if p.left -= len(p.source); p.left < 0 {
		return false
	}
	return p.c.reads(standing(p.source, word, found...))
}

// named reports whether r may be of a name.
func named(r rune) bool {
	return r == '_' || r == '$' || r >= utf8.RuneSelf || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// standing is source with word in place of the import at each place.
func standing(source, word string, places ...[]int) string {
	text := []byte(source)
	for _, at := range places {
		copy(text[at[0]:], word)
	}
	return string(text)
}

// reads reports whether the engine reads source as a module.
func (c *context) reads(source string) bool {
	mod, err := v8.CompileModule(c.iso, source, "light:probe")
	if err != nil {
		return false
	}
	mod.Delete()
	return true
}
