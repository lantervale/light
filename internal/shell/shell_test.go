package shell_test

import (
	"strings"
	"testing"

	"github.com/lantervale/light/internal/shell"
)

func TestEachSystemHasItsCommandToOpenAFileAndToShowIt(t *testing.T) {
	for system, want := range map[string][2]string{
		"darwin":  {"open -- /d/a b.pdf", "open -R -- /d/a b.pdf"},
		"linux":   {"xdg-open /d/a b.pdf", "xdg-open /d"},
		"windows": {"explorer /d/a b.pdf", "explorer /select,/d/a b.pdf"},
	} {
		open, _ := shell.Command(system, shell.Open, "/d/a b.pdf")
		reveal, _ := shell.Command(system, shell.Reveal, "/d/a b.pdf")
		if got := [2]string{strings.Join(open, " "), strings.Join(reveal, " ")}; got != want {
			t.Errorf("%s runs %q, want %q", system, got, want)
		}
	}
	if _, err := shell.Command("plan9", shell.Open, "/d/a"); err == nil {
		t.Error("a system light knows no command for gives one")
	}
}

func TestAFileIsHandedAsOneArgumentNeverAsACommand(t *testing.T) {
	for _, system := range []string{"darwin", "linux", "windows"} {
		args, err := shell.Command(system, shell.Open, "/d/-rf; rm x")
		if err != nil || args[len(args)-1] != "/d/-rf; rm x" {
			t.Errorf("%s: %q, %v", system, args, err)
		}
	}
}

func TestTheFolderShownIsTakenByTheSeparatorOfTheSystemAsked(t *testing.T) {
	if args, _ := shell.Command("linux", shell.Reveal, "/home/a/b.pdf"); args[1] != "/home/a" {
		t.Errorf("linux shows %q", args[1])
	}
	if args, _ := shell.Command("linux", shell.Reveal, `/home/a\b.pdf`); args[1] != "/home" {
		t.Errorf("a backslash in a linux path is taken as a folder: %q", args[1])
	}
}
