package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func parsed(t *testing.T, args ...string) CLI {
	t.Helper()
	cli, err := parse(args)
	if err != nil {
		t.Fatalf("light %s: %v", strings.Join(args, " "), err)
	}
	return cli
}

func TestTheHistoryFolderIsTheOneGivenAndNoneForAHiddenRun(t *testing.T) {
	t.Setenv("LIGHT_DIR", "/somewhere")
	t.Setenv("EMBER_HIDDEN", "1")
	if got := parsed(t).dir(); got != "/somewhere" {
		t.Fatalf("LIGHT_DIR gives %q", got)
	}
	if got := parsed(t, "--dir", "/flagged").dir(); got != "/flagged" {
		t.Fatalf("--dir over LIGHT_DIR gives %q", got)
	}
	t.Setenv("LIGHT_DIR", "")
	if got := parsed(t).dir(); got != "" {
		t.Fatalf("a hidden run keeps its history in %q", got)
	}
	t.Setenv("EMBER_HIDDEN", "")
	if got := parsed(t).dir(); filepath.Base(got) != "light" {
		t.Fatalf("a shown run keeps its history in %q", got)
	}
}

func TestTheEnvironmentAndTheFlagsOnlyOverrideWhatTheSettingsSay(t *testing.T) {
	t.Setenv("EMBER_HIDDEN", "")
	t.Setenv("LIGHT_DIR", "/kept")
	t.Setenv("LIGHT_DOWNLOADS", "/elsewhere")
	t.Setenv("HOME", "/home/someone")
	usual := filepath.Join("/home/someone", "Downloads")
	if got := parsed(t).options(); got.Dir != "/kept" || got.Downloads != "/elsewhere" ||
		got.DefaultDownloads != usual || got.Files == nil {
		t.Fatalf("the options are %+v", got)
	}
	if got := parsed(t, "--downloads", "/flagged").options(); got.Downloads != "/flagged" {
		t.Fatalf("--downloads over LIGHT_DOWNLOADS gives %q", got.Downloads)
	}
	t.Setenv("LIGHT_DOWNLOADS", "")
	if got := parsed(t).options(); got.Downloads != "" || got.DefaultDownloads != usual {
		t.Fatalf("with no folder given, the settings' is overridden by %q, the system's is %q", got.Downloads,
			got.DefaultDownloads)
	}
}

func TestTheAddressIsTheArgumentAndMayBeLeftOut(t *testing.T) {
	if got := parsed(t).Address; got != "" {
		t.Fatalf("with no argument the address is %q", got)
	}
	if got := parsed(t, "example.com").Address; got != "example.com" {
		t.Fatalf("the address is %q", got)
	}
	if got := parsed(t, "--dir", "/x", "light://scripts").Address; got != "light://scripts" {
		t.Fatalf("after a flag the address is %q", got)
	}
}

func TestWhatLightDoesNotKnowIsRefused(t *testing.T) {
	for _, args := range [][]string{{"--nope"}, {"one", "two"}} {
		if _, err := parse(args); err == nil {
			t.Errorf("light %s is taken", strings.Join(args, " "))
		}
	}
}

func TestVersionSaysWhatWasBuilt(t *testing.T) {
	var out bytes.Buffer
	exited := -1
	_, err := parse([]string{"--version"}, kong.Writers(&out, &out), kong.Exit(func(code int) { exited = code }))
	if err != nil {
		t.Fatal(err)
	}
	if exited != 0 || !strings.Contains(out.String(), version) {
		t.Fatalf("--version exits with %d saying %q", exited, out.String())
	}
}

func TestHelpNamesTheAddressAndTheFolders(t *testing.T) {
	var out bytes.Buffer
	if _, err := parse([]string{"--help"}, kong.Writers(&out, &out), kong.Exit(func(int) {})); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[<address>]", "--dir", "LIGHT_DIR", "--downloads", "LIGHT_DOWNLOADS"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the help lacks %q:\n%s", want, out.String())
		}
	}
}
