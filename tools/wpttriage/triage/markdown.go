package triage

import (
	"fmt"
	"io"
	"strings"
)

const directoriesShown = 3

// Markdown writes the report's figures and its top groups as a table.
func Markdown(w io.Writer, res *Result, top int) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# WPT failure triage\n\n")
	fmt.Fprintf(&b, "Tests passed: %d / %d. Subtests passed: %d / %d. %d causes.\n\n",
		res.TestsPassed, res.Tests, res.SubtestsPassed, res.Subtests, len(res.Groups))
	b.WriteString("A cause's tests unlocked are the tests that fail for that cause alone; its tests, those it is one " +
		"cause of; its subtests, those it fails. A cause in brackets is local to that directory.\n\n")
	b.WriteString("| # | Signature | Tests unlocked | Tests | Subtests | Directories | Examples | Fix likely in |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for i, g := range res.Groups[:min(top, len(res.Groups))] {
		fmt.Fprintf(&b, "| %d | %s | %d | %d | %d | %s | %s | %s |\n",
			i+1, cell(g.Signature), g.Unlocked, g.Tests, g.Subtests, dirs(g.Directories), paths(g.Examples), cell(g.Fix))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func dirs(ds []Directory) string {
	var parts []string
	for _, d := range ds[:min(directoriesShown, len(ds))] {
		parts = append(parts, fmt.Sprintf("`%s` %d", d.Dir, d.Tests))
	}
	if more := len(ds) - directoriesShown; more > 0 {
		parts = append(parts, fmt.Sprintf("+%d", more))
	}
	return cell(strings.Join(parts, ", "))
}

func paths(ps []string) string {
	var parts []string
	for _, p := range ps {
		parts = append(parts, "`"+p+"`")
	}
	return cell(strings.Join(parts, "<br>"))
}

func cell(s string) string {
	return strings.NewReplacer("|", `\|`, "\n", " ").Replace(s)
}
