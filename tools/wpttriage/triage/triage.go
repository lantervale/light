// Package triage groups the failures of a web-platform-tests run (a wptreport.json) by cause, and ranks the causes by
// the tests that would pass were that cause alone fixed.
package triage

import (
	"cmp"
	"slices"
	"strings"
)

// Report is the part of a wptreport.json triage reads.
type Report struct {
	Results []TestResult `json:"results"`
}

// TestResult is one test of a report: a file, or one variant of it.
type TestResult struct {
	Test     string    `json:"test"`
	Status   string    `json:"status"`
	Message  string    `json:"message"`
	Subtests []Subtest `json:"subtests"`
}

// Subtest is one subtest of a test.
type Subtest struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Result is a report's figures and its causes, ranked.
type Result struct {
	Tests          int     `json:"tests"`
	TestsPassed    int     `json:"testsPassed"`
	Subtests       int     `json:"subtests"`
	SubtestsPassed int     `json:"subtestsPassed"`
	Groups         []Group `json:"groups"`
}

// Group is one cause and the failures it explains. Unlocked counts the tests that fail for this cause alone; Tests,
// those it is one cause of; Subtests, the subtests it fails.
type Group struct {
	Signature   string      `json:"signature"`
	Kind        Kind        `json:"kind"`
	Unlocked    int         `json:"unlocked"`
	Tests       int         `json:"tests"`
	Subtests    int         `json:"subtests"`
	Directories []Directory `json:"directories"`
	Examples    []string    `json:"examples"`
	Fix         string      `json:"fix"`
}

// Directory is a group's share of one directory.
type Directory struct {
	Dir      string `json:"dir"`
	Tests    int    `json:"tests"`
	Subtests int    `json:"subtests"`
}

const (
	examples = 3
	pass     = "PASS"
)

type tally struct {
	cause    Cause
	unlocked []string
	others   []string
	subtests int
	dirs     map[string]*Directory
}

// Passed tells whether a test passed: a reference test that matched, or a script test whose harness is OK and every
// subtest passed.
func Passed(r *TestResult) bool {
	if r.Status == pass {
		return true
	}
	if r.Status != "OK" {
		return false
	}
	for _, s := range r.Subtests {
		if s.Status != pass {
			return false
		}
	}
	return true
}

// Triage groups the failures of a report by cause.
func Triage(report *Report) *Result {
	res := &Result{Tests: len(report.Results)}
	tallies := map[string]*tally{}
	for i := range report.Results {
		r := &report.Results[i]
		res.Subtests += len(r.Subtests)
		for _, s := range r.Subtests {
			if s.Status == pass {
				res.SubtestsPassed++
			}
		}
		if Passed(r) {
			res.TestsPassed++
			continue
		}
		causes := causesOf(r)
		for sig, c := range causes {
			t := tallies[sig]
			if t == nil {
				t = &tally{cause: c.cause, dirs: map[string]*Directory{}}
				tallies[sig] = t
			}
			t.add(r.Test, c.subtests, len(causes) == 1)
		}
	}
	for sig, t := range tallies {
		res.Groups = append(res.Groups, t.group(sig))
	}
	slices.SortFunc(res.Groups, func(a, b Group) int {
		return cmp.Or(
			cmp.Compare(b.Unlocked, a.Unlocked),
			cmp.Compare(b.Subtests, a.Subtests),
			cmp.Compare(b.Tests, a.Tests),
			strings.Compare(a.Signature, b.Signature),
		)
	})
	return res
}

type counted struct {
	cause    Cause
	subtests int
}

// causesOf collects a failing test's causes by signature, with the subtests each fails. A subtest a harness error or
// a timeout kept from running is that cause's.
func causesOf(r *TestResult) map[string]*counted {
	causes := map[string]*counted{}
	top, stopped := ClassifyTest(r)
	if stopped {
		causes[top.Signature()] = &counted{cause: top}
	}
	for _, s := range r.Subtests {
		if s.Status == pass {
			continue
		}
		c := ClassifySubtest(r.Test, s)
		if s.Status == "NOTRUN" && stopped {
			c = top
		}
		sig := c.Signature()
		if causes[sig] == nil {
			causes[sig] = &counted{cause: c}
		}
		causes[sig].subtests++
	}
	if len(causes) == 0 {
		c := Cause{Kind: Other, Key: "failed: " + r.Status, Scope: Scope(r.Test)}
		causes[c.Signature()] = &counted{cause: c}
	}
	return causes
}

func (t *tally) add(test string, subtests int, alone bool) {
	if alone {
		t.unlocked = append(t.unlocked, test)
	} else {
		t.others = append(t.others, test)
	}
	t.subtests += subtests
	dir := Scope(test)
	d := t.dirs[dir]
	if d == nil {
		d = &Directory{Dir: dir}
		t.dirs[dir] = d
	}
	d.Tests++
	d.Subtests += subtests
}

func (t *tally) group(sig string) Group {
	slices.Sort(t.unlocked)
	slices.Sort(t.others)
	g := Group{
		Signature: sig,
		Kind:      t.cause.Kind,
		Unlocked:  len(t.unlocked),
		Tests:     len(t.unlocked) + len(t.others),
		Subtests:  t.subtests,
		Fix:       Fix(t.cause),
	}
	g.Examples = append(g.Examples, t.unlocked[:min(examples, len(t.unlocked))]...)
	g.Examples = append(g.Examples, t.others[:min(examples-len(g.Examples), len(t.others))]...)
	for _, d := range t.dirs {
		g.Directories = append(g.Directories, *d)
	}
	slices.SortFunc(g.Directories, func(a, b Directory) int {
		return cmp.Or(cmp.Compare(b.Tests, a.Tests), cmp.Compare(b.Subtests, a.Subtests), strings.Compare(a.Dir, b.Dir))
	})
	return g
}
