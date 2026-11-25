// Package release holds the tests of the release scripts beside it.
package release_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// needs skips t without the programs the scripts run.
func needs(t *testing.T, programs ...string) {
	t.Helper()
	for _, p := range programs {
		if _, err := exec.LookPath(p); err != nil {
			t.Skipf("no %s", p)
		}
	}
}

// script runs the script name of this folder in dir with env, and returns
// its output and whether it failed.
func script(t *testing.T, dir, name string, env []string, args ...string) (string, bool) {
	t.Helper()
	path, err := filepath.Abs(name)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", append([]string{path}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err != nil
}

// repo makes a git repository in a new folder, with a commit per subject,
// tagged where a subject is "tag <name>".
func repo(t *testing.T, subjects ...string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	for k, s := range subjects {
		if name, ok := strings.CutPrefix(s, "tag "); ok {
			git(t, dir, "tag", "-a", name, "-m", name)
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, "file"), []byte{byte(k)}, 0o600); err != nil {
			t.Fatal(err)
		}
		git(t, dir, "add", "file")
		git(t, dir, "commit", "-q", "-m", s)
	}
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// light is a folder whose go.mod pins ember at the commit pin.
func light(t *testing.T, pin string) string {
	t.Helper()
	dir := t.TempDir()
	mod := "module github.com/lantervale/light\n\ngo 1.27.0\n\nrequire (\n" +
		"\tgithub.com/lantervale/ember v0.0.0-20261124193741-" + pin[:12] + "\n)\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func TestTheBumpLeavesAPinnedMainAlone(t *testing.T) {
	needs(t, "sh", "git", "awk")
	ember := repo(t, "feat: one")
	main := git(t, ember, "rev-parse", "HEAD")
	dir := light(t, main)
	out, failed := script(t, dir, "bump-ember.sh", []string{"EMBER_REPO=" + ember, "DRY_RUN=1"})
	if failed || !strings.HasPrefix(lastLine(out), "ember: up to date") {
		t.Fatalf("ember's main pinned already: %s", out)
	}
}

func TestTheBumpPinsMainAsByHandOnceItMoved(t *testing.T) {
	needs(t, "sh", "git", "awk")
	ember := repo(t, "feat: one", "feat: two")
	pinned := git(t, ember, "rev-parse", "HEAD~1")
	main := git(t, ember, "rev-parse", "HEAD")
	dir := light(t, pinned)
	before, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	out, failed := script(t, dir, "bump-ember.sh", []string{"EMBER_REPO=" + ember, "DRY_RUN=1"})
	if failed {
		t.Fatal(out)
	}
	for _, want := range []string{
		"would run env GOWORK=off GOFLAGS=-mod=mod go get github.com/lantervale/ember@" + main,
		"would run env GOWORK=off go mod tidy",
		"would run env GOWORK=off sh ",
		"preflight.sh false",
		"ember: pinned " + main,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
	after, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if string(after) != string(before) {
		t.Error("a dry run changed go.mod")
	}
	out, failed = script(t, dir, "bump-ember.sh", []string{"DRY_RUN=1"}, "main")
	if !failed {
		t.Errorf("a branch name taken for a commit: %s", out)
	}
}

const notes = `## light v1.2.3

light is a small web browser.

## What is new

- Something hand-written for v1.2.3.

## What to download

| System | Archive |
|---|---|
| macOS | ` + "`light_<version>_darwin_arm64.dmg`" + ` |

## Known limits

- No cache.
`

func TestTheNotesAreDraftedFromTheConventionalCommitsSinceTheLastTag(t *testing.T) {
	needs(t, "sh", "git", "awk")
	dir := repo(t, "feat: before the tag", "tag v1.2.3",
		"feat(dom): blur() takes the focus off", "fix: the window loads once its pictures show",
		"chore: pin ember 778b8c7", "docs: a page's fonts", "perf(layout): lines are cut once",
		"fix(webdriver)!: scroll by pixels", "docs(release): the notes of v1.2.3", "feat: JavaScript on Windows")
	path := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(path, []byte(notes), 0o600); err != nil {
		t.Fatal(err)
	}
	out, failed := script(t, dir, "draft-notes.sh", []string{"NOTES=" + path}, "v1.2.4")
	if failed {
		t.Fatal(out)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `## light v1.2.4

light is a small web browser.

## What is new

### New

- **dom**: blur() takes the focus off
- JavaScript on Windows

### Fixed

- the window loads once its pictures show
- **webdriver**: scroll by pixels

### Faster

- **layout**: lines are cut once

## What to download

| System | Archive |
|---|---|
| macOS | ` + "`light_<version>_darwin_arm64.dmg`" + ` |

## Known limits

- No cache.
`
	if string(got) != want {
		t.Errorf("the notes drafted:\n%s\nwant:\n%s", got, want)
	}
}
