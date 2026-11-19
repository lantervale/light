package console_test

import (
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/console"
)

func TestLinesAreKeptInOrderUpToTheCap(t *testing.T) {
	log := console.New(3)
	began := log.Version()
	for _, text := range []string{"a", "b", "c", "d"} {
		log.Add(console.Line{Level: console.Info, Text: text, Source: "https://site.test/app.js", Line: 4})
	}
	lines := log.Lines()
	if len(lines) != 3 || lines[0].Text != "b" || lines[2].Text != "d" || log.Dropped() != 1 {
		t.Fatalf("the log keeps %+v and dropped %d", lines, log.Dropped())
	}
	if lines[2].At.IsZero() || lines[2].Where() != "app.js:4" {
		t.Errorf("a line is timed %v and from %q", lines[2].At, lines[2].Where())
	}
	if log.Version() == began {
		t.Error("the version did not change with the lines")
	}
	lines[0].Text = "changed"
	if log.Lines()[0].Text != "b" {
		t.Error("the lines given are the log's own")
	}
}

func TestClearEmptiesTheLog(t *testing.T) {
	log := console.New(2)
	log.Add(console.Line{Level: console.Error, Text: "a"})
	log.Add(console.Line{Level: console.Warn, Text: "b"})
	log.Add(console.Line{Level: console.Log, Text: "c"})
	if log.Count(console.Error) != 0 || log.Count(console.Warn) != 1 {
		t.Fatalf("the log counts %d errors and %d warnings", log.Count(console.Error), log.Count(console.Warn))
	}
	seen := log.Version()
	log.Clear()
	if len(log.Lines()) != 0 || log.Dropped() != 0 || log.Version() == seen {
		t.Fatalf("after Clear the log keeps %d lines, dropped %d", len(log.Lines()), log.Dropped())
	}
}

func TestALineSaysWhereItIsFrom(t *testing.T) {
	for _, c := range []struct {
		source string
		line   int
		want   string
	}{
		{"", 0, ""}, {"https://site.test/", 3, "site.test:3"}, {"https://site.test/js/app.min.js?v=2", 120, "app.min.js:120"},
		{"https://site.test/page", 0, "page"}, {"light://scripts", 7, "scripts:7"},
	} {
		if got := (console.Line{Source: c.source, Line: c.line}).Where(); got != c.want {
			t.Errorf("%s line %d is from %q, want %q", c.source, c.line, got, c.want)
		}
	}
}

func TestALineSaidAgainIsCountedNotListedTwice(t *testing.T) {
	log := console.New(10)
	tick := console.Line{Level: console.Log, Text: "tick", Source: "https://site.test/app.js", Line: 4}
	for range 3 {
		log.Add(tick)
	}
	log.Add(console.Line{Level: console.Warn, Text: "tick", Source: "https://site.test/app.js", Line: 4})
	log.Add(tick)
	for range 2 {
		log.Add(console.Line{Level: console.Input, Text: "n"})
		log.Add(console.Line{Level: console.Error, Text: "Uncaught ReferenceError: n is not defined"})
	}
	lines := log.Lines()
	var got []int
	for _, l := range lines {
		got = append(got, l.Times())
	}
	if len(lines) != 7 || got[0] != 3 || got[1] != 1 || got[2] != 1 {
		t.Fatalf("the log keeps %d lines said %v times", len(lines), got)
	}
	if log.Count(console.Log) != 4 || log.Count(console.Error) != 2 || log.Count(console.Input) != 2 {
		t.Errorf("the log counts %d logs, %d errors, %d inputs", log.Count(console.Log), log.Count(console.Error),
			log.Count(console.Input))
	}
	if said := log.Said(); said != 7 {
		t.Errorf("the page said %d lines, want 7: what was typed and what it gave are not its own", said)
	}
}

func TestLinesAreKeptByTheirLevelAndTheirText(t *testing.T) {
	lines := []console.Line{
		{Level: console.Log, Text: "Ready"},
		{Level: console.Info, Text: "loaded", Source: "https://site.test/app.js", Line: 2},
		{Level: console.Warn, Text: "slow"}, {Level: console.Error, Text: "Uncaught TypeError"},
		{Level: console.Debug, Text: "ready to debug"}, {Level: console.Input, Text: "ready()"},
		{Level: console.Result, Text: "undefined"},
	}
	for _, c := range []struct {
		text  string
		muted []console.Level
		want  string
	}{
		{"", nil, "Ready,loaded,slow,Uncaught TypeError,ready to debug,ready(),undefined"},
		{"", []console.Level{console.Log, console.Info, console.Warn, console.Debug}, "Uncaught TypeError,ready(),undefined"},
		{"", []console.Level{console.Error}, "Ready,loaded,slow,ready to debug,ready(),undefined"},
		{"", []console.Level{console.Log}, "loaded,slow,Uncaught TypeError,ready to debug,ready(),undefined"},
		{"", []console.Level{console.Info, console.Input}, "Ready,slow,Uncaught TypeError,ready to debug,ready(),undefined"},
		{"READY", nil, "Ready,ready to debug,ready()"},
		{"app.js", nil, "loaded"},
		{"ready", []console.Level{console.Debug}, "Ready,ready()"},
	} {
		muted := make(map[console.Level]bool)
		for _, level := range c.muted {
			muted[level] = true
		}
		var got []string
		for _, l := range console.Filter(lines, c.text, muted) {
			got = append(got, l.Text)
		}
		if strings.Join(got, ",") != c.want {
			t.Errorf("%q without %q keeps %q, want %q", c.text, c.muted, got, c.want)
		}
	}
}
