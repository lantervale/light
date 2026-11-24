package triage_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lantervale/light/tools/wpttriage/triage"
)

func TestClassifySubtest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		test, name, message string
		want                triage.Cause
	}{
		{
			"/dom/nodes/a.html", "x", "x.foo is not a function",
			triage.Cause{Kind: triage.MissingAPI, Key: "not a function: foo"},
		},
		{
			"/dom/nodes/a.html", "x", "document.createFoo(...).bar is not a function",
			triage.Cause{Kind: triage.MissingAPI, Key: "not a function: bar"},
		},
		{
			"/dom/nodes/a.html", "x", "Cannot read properties of undefined (reading 'baz')",
			triage.Cause{Kind: triage.Exception, Key: "reading 'baz' of undefined"},
		},
		{
			"/html/dom/a.html", "x", "FooBar is not defined",
			triage.Cause{Kind: triage.MissingAPI, Key: "not defined: FooBar"},
		},
		{
			"/html/dom/a.html", "x", "Illegal constructor",
			triage.Cause{Kind: triage.Exception, Key: "Illegal constructor", Scope: "/html/dom"},
		},
		{
			"/fetch/api/a.html", "x",
			`promise_test: Unhandled rejection with value: object "TypeError: r.bytes is not a function"`,
			triage.Cause{Kind: triage.MissingAPI, Key: "not a function: bytes"},
		},
		{
			"/url/a.html", "x", "Failed to construct 'URL': Invalid URL 'http://a\tb'",
			triage.Cause{Kind: triage.Exception, Key: "Failed to construct '…': Invalid URL '…'", Scope: "/url"},
		},
		{
			"/url/a.html", "x", "Failed to construct 'URL': Invalid URL 'a!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~'",
			triage.Cause{Kind: triage.Exception, Key: "Failed to construct '…': Invalid URL '…'", Scope: "/url"},
		},
		{
			"/dom/a.html", "x", `assert_equals: the "it's" label expected "a" but got "b"`,
			triage.Cause{Kind: triage.Assertion, Key: `assert_equals: the "…" label`, Scope: "/dom"},
		},
		{
			"/dom/a.html", "x", `assert_true: don't drop 'x' now expected true got false`,
			triage.Cause{Kind: triage.Assertion, Key: `assert_true: don't drop '…' now`, Scope: "/dom"},
		},
		{
			"/url/a.html", "x", `assert_equals: href expected "file:///a/" but got "file:/a/"`,
			triage.Cause{Kind: triage.Assertion, Key: "assert_equals: href", Scope: "/url"},
		},
		{
			"/dom/ranges/r.html", "x", `assert_equals: expected 3 but got undefined`,
			triage.Cause{Kind: triage.Assertion, Key: "assert_equals: → undefined", Scope: "/dom/ranges"},
		},
		{
			"/html/semantics/forms/f/a.html", "x", `assert_true: item 12 is checked expected true got false`,
			triage.Cause{Kind: triage.Assertion, Key: "assert_true: item N is checked", Scope: "/html/semantics/forms"},
		},
		{
			"/dom/idlharness.html", "Node interface: attribute fooBar",
			`assert_true: The prototype object must have a property "fooBar" expected true got false`,
			triage.Cause{Kind: triage.MissingAPI, Key: "missing IDL member Node.fooBar"},
		},
		{
			"/dom/x.html", "x", `assert_own_property: self does not have own property "Bar" expected property "Bar" missing`,
			triage.Cause{Kind: triage.MissingAPI, Key: "missing property Bar"},
		},
		{
			"/css/css-align/parsing/a.html", "Property align-self value 'anchor-center'",
			`assert_equals: expected "anchor-center" but got ""`,
			triage.Cause{Kind: triage.CSSComputed, Key: "computed value unsupported: align-self", Scope: "/css/css-align"},
		},
		{
			"/css/css-align/parsing/a.html", "Property align-self value 'first baseline'",
			`assert_equals: expected "baseline" but got "first baseline"`,
			triage.Cause{Kind: triage.CSSComputed, Key: "computed value: align-self", Scope: "/css/css-align"},
		},
		{
			"/css/css-flexbox/parsing/a.html", "e.style['flex-basis'] = \"calc(1px)\" should set the property value",
			`assert_not_equals: property should be set got disallowed value ""`,
			triage.Cause{Kind: triage.CSSParsing, Key: "value not parsed: flex-basis", Scope: "/css/css-flexbox"},
		},
		{
			"/css/css-flexbox/parsing/a.html", "e.style['flex-basis'] = \"1\" should not set the property value",
			`assert_equals: expected "" but got "1px"`,
			triage.Cause{Kind: triage.CSSParsing, Key: "invalid value accepted: flex-basis", Scope: "/css/css-flexbox"},
		},
		{
			"/css/css-flexbox/parsing/a.html", "e.style['flex-basis'] = \"1\" should not set the property value",
			`x.foo is not a function`,
			triage.Cause{Kind: triage.MissingAPI, Key: "not a function: foo"},
		},
	}
	for _, c := range cases {
		got := triage.ClassifySubtest(c.test, triage.Subtest{Name: c.name, Status: "FAIL", Message: c.message})
		if got != c.want {
			t.Errorf("%s %q: got %+v, want %+v", c.test, c.message, got, c.want)
		}
	}
}

func TestSubtestTimeoutIsScoped(t *testing.T) {
	t.Parallel()
	got := triage.ClassifySubtest("/IndexedDB/a.any.html", triage.Subtest{Name: "x", Status: "TIMEOUT"})
	want := triage.Cause{Kind: triage.Timeout, Key: "subtest timeout", Scope: "/IndexedDB"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSignature(t *testing.T) {
	t.Parallel()
	if got := (triage.Cause{Key: "a", Scope: "/b"}).Signature(); got != "a [/b]" {
		t.Errorf("scoped: %q", got)
	}
	if got := (triage.Cause{Key: "a"}).Signature(); got != "a" {
		t.Errorf("unscoped: %q", got)
	}
}

const report = `{"results": [
 {"test": "/dom/a.html", "status": "OK", "subtests": [
   {"name": "1", "status": "PASS"},
   {"name": "2", "status": "FAIL", "message": "x.foo is not a function"}]},
 {"test": "/dom/b.html", "status": "OK", "subtests": [
   {"name": "1", "status": "FAIL", "message": "y.foo is not a function"},
   {"name": "2", "status": "FAIL", "message": "Bar is not defined"}]},
 {"test": "/html/c.html", "status": "OK", "subtests": [
   {"name": "1", "status": "FAIL", "message": "Bar is not defined"},
   {"name": "2", "status": "FAIL", "message": "Bar is not defined"},
   {"name": "3", "status": "FAIL", "message": "Bar is not defined"}]},
 {"test": "/html/d.html", "status": "OK", "subtests": [
   {"name": "1", "status": "FAIL", "message": "z.foo is not a function"}]},
 {"test": "/dom/e.html", "status": "TIMEOUT", "message": "", "subtests": [
   {"name": "1", "status": "PASS"},
   {"name": "2", "status": "NOTRUN"}]},
 {"test": "/css/f.html", "status": "FAIL", "subtests": []},
 {"test": "/css/g.html", "status": "PASS", "subtests": []},
 {"test": "/dom/h.html", "status": "ERROR", "message": "Uncaught TypeError: q.foo is not a function", "subtests": []},
 {"test": "/dom/i.html", "status": "CRASH", "subtests": []},
 {"test": "/dom/j.html", "status": "OK", "subtests": [{"name": "1", "status": "PASS"}]}
]}`

func load(t *testing.T) *triage.Result {
	t.Helper()
	var r triage.Report
	if err := json.Unmarshal([]byte(report), &r); err != nil {
		t.Fatal(err)
	}
	return triage.Triage(&r)
}

func TestTriageCounts(t *testing.T) {
	t.Parallel()
	res := load(t)
	if res.Tests != 10 || res.TestsPassed != 2 {
		t.Errorf("tests %d passed %d, want 10 and 2", res.Tests, res.TestsPassed)
	}
	if res.Subtests != 11 || res.SubtestsPassed != 3 {
		t.Errorf("subtests %d passed %d, want 11 and 3", res.Subtests, res.SubtestsPassed)
	}
}

func TestTriageRanksByTestsUnlocked(t *testing.T) {
	t.Parallel()
	res := load(t)
	first := res.Groups[0]
	if first.Signature != "not a function: foo" {
		t.Fatalf("first group %q, want not a function: foo", first.Signature)
	}
	// a, d and h fail for foo alone; b also needs Bar.
	if first.Unlocked != 3 || first.Tests != 4 || first.Subtests != 3 {
		t.Errorf("foo: unlocked %d tests %d subtests %d, want 3, 4, 3", first.Unlocked, first.Tests, first.Subtests)
	}
	if first.Kind != triage.MissingAPI || first.Fix == "" {
		t.Errorf("foo: kind %q fix %q", first.Kind, first.Fix)
	}
	if got := first.Examples; len(got) != 3 || got[0] != "/dom/a.html" {
		t.Errorf("foo examples %v: the unlocked ones first", got)
	}
	if got := first.Directories; len(got) != 2 || got[0].Dir != "/dom" || got[0].Tests != 3 {
		t.Errorf("foo directories %+v", got)
	}
	bar := find(t, res, "not defined: Bar")
	if bar.Unlocked != 1 || bar.Subtests != 4 {
		t.Errorf("Bar: unlocked %d subtests %d, want 1 and 4", bar.Unlocked, bar.Subtests)
	}
}

func TestTriageTestLevelCauses(t *testing.T) {
	t.Parallel()
	res := load(t)
	timeout := find(t, res, "test timeout [/dom]")
	if timeout.Unlocked != 1 || timeout.Subtests != 1 {
		t.Errorf("timeout %+v: the NOTRUN subtest is the timeout's", timeout)
	}
	find(t, res, "reference mismatch [/css]")
	find(t, res, "crash [/dom]")
	for _, g := range res.Groups {
		if strings.Contains(g.Signature, "not run") {
			t.Errorf("a NOTRUN subtest of a timed-out test makes no group of its own: %q", g.Signature)
		}
	}
}

func find(t *testing.T, res *triage.Result, signature string) triage.Group {
	t.Helper()
	for _, g := range res.Groups {
		if g.Signature == signature {
			return g
		}
	}
	t.Fatalf("no group %q", signature)
	return triage.Group{}
}

func TestMarkdown(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := triage.Markdown(&buf, load(t), 2); err != nil {
		t.Fatal(err)
	}
	md := buf.String()
	for _, want := range []string{"2 / 10", "3 / 11", "| 1 | not a function: foo | 3 |", "`/dom/a.html`"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
	if rows := strings.Count(md, "\n| "); rows != 3 {
		t.Errorf("markdown has %d table lines, want a header and 2 groups:\n%s", rows, md)
	}
}

func TestMarkdownEscapesPipes(t *testing.T) {
	t.Parallel()
	var r triage.Report
	r.Results = []triage.TestResult{{Test: "/dom/a.html", Status: "OK", Subtests: []triage.Subtest{
		{Name: "1", Status: "FAIL", Message: `assert_equals: a|b expected 1 but got 2`},
	}}}
	var buf bytes.Buffer
	if err := triage.Markdown(&buf, triage.Triage(&r), 60); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `a\|b`) {
		t.Errorf("pipe not escaped:\n%s", buf.String())
	}
}
