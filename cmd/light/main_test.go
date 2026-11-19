package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
)

func parse(args []string, opts ...kong.Option) (*CLI, *kong.Context, error) {
	cli := newCLI()
	ctx, err := newParser(cli, opts...).Parse(args)
	return cli, ctx, err
}

func parsed(t *testing.T, args ...string) (*CLI, string) {
	t.Helper()
	cli, ctx, err := parse(args)
	if err != nil {
		t.Fatalf("light %s: %v", strings.Join(args, " "), err)
	}
	return cli, ctx.Command()
}

func said(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	exited := -1
	_, _, err := parse(args, kong.Writers(&out, &out), kong.Exit(func(code int) { exited = code }))
	if exited != 0 {
		t.Fatalf("light %s exits with %d: %v", strings.Join(args, " "), exited, err)
	}
	return out.String()
}

func TestLightWithNoCommandIsAPersonsBrowser(t *testing.T) {
	t.Setenv("LIGHT_WEBDRIVER", "127.0.0.1:4444")
	t.Setenv("LIGHT_DIR", "")
	for _, c := range []struct {
		args             []string
		command, address string
	}{
		{nil, "browse", ""},
		{[]string{"example.com"}, "browse <address>", "example.com"},
		{[]string{"browse"}, "browse", ""},
		{[]string{"browse", "https://example.com"}, "browse <address>", "https://example.com"},
		{[]string{"--dir", "/x", "light://scripts"}, "browse <address>", "light://scripts"},
		{[]string{"light://scripts", "--dir", "/x"}, "browse <address>", "light://scripts"},
	} {
		cli, command := parsed(t, c.args...)
		if command != c.command || cli.Browse.Address != c.address {
			t.Errorf("light %s runs %q at %q", strings.Join(c.args, " "), command, cli.Browse.Address)
		}
	}
	if cli, _ := parsed(t, "--dir", "/x", "--downloads", "/y", "--patient"); cli.Dir != "/x" || cli.Downloads != "/y" ||
		!cli.Browse.Patient {
		t.Errorf("the folders and the patience given are %+v, %+v", cli.Profile, cli.Browse)
	}
	if cli, _ := parsed(t, "browse", "light://scripts"); cli.Browse.Patient {
		t.Error("a browser not told to is told to be patient")
	}
}

func TestDriveIsALightATestRunnerDrives(t *testing.T) {
	t.Setenv("LIGHT_WEBDRIVER", "")
	cli, command := parsed(t, "drive", "--address", "127.0.0.1:4444")
	if command != "drive" || cli.Drive.Address != "127.0.0.1:4444" || cli.Drive.Open != "" ||
		cli.Drive.ScriptBudget != nil {
		t.Fatalf("light drive runs %q with %+v", command, cli.Drive)
	}
	cli, command = parsed(t, "drive", "--address", "127.0.0.1:4444", "--dir", "/kept", "--host-map",
		"web-platform.test=127.0.0.1;other.test=::1", "--trust-ca", "ca.pem", "--script-budget", "10s",
		"about:blank")
	d := cli.Drive
	if command != "drive <address>" || d.Open != "about:blank" || d.HostMap["web-platform.test"] != "127.0.0.1" ||
		d.HostMap["other.test"] != "::1" || d.TrustCA != "ca.pem" || d.ScriptBudget == nil ||
		*d.ScriptBudget != 10*time.Second || cli.Dir != "/kept" {
		t.Fatalf("light drive with everything runs %q with %+v, %+v", command, d, cli.Profile)
	}
	if _, _, err := parse([]string{"drive", "--address", "127.0.0.1:4444", "--patient"}); err == nil {
		t.Fatal("a driven light takes --patient")
	}
	if cli, _ = parsed(t, "--dir", "/before", "drive", "--address", "[::1]:4444"); cli.Dir != "/before" {
		t.Fatalf("a folder given before the command is %q", cli.Dir)
	}
	t.Setenv("LIGHT_WEBDRIVER", "localhost:9515")
	if cli, command = parsed(t, "drive"); command != "drive" || cli.Drive.Address != "localhost:9515" {
		t.Fatalf("LIGHT_WEBDRIVER gives light drive the address %q", cli.Drive.Address)
	}
}

func TestWhatLightDoesNotKnowIsRefused(t *testing.T) {
	t.Setenv("LIGHT_WEBDRIVER", "")
	for _, args := range [][]string{
		{"--nope"}, {"one", "two"}, {"browse", "one", "two"},
		{"drive"}, {"drive", "--address", ""}, {"drive", "--address", "127.0.0.1:4444", "one", "two"},
		{"drive", "--address", "127.0.0.1:4444", "--script-budget", "0s"},
		{"drive", "--address", "127.0.0.1:4444", "--script-budget", "-1s"},
		{"drive", "--address", "127.0.0.1:4444", "--script-budget", "soon"},
	} {
		if _, _, err := parse(args); err == nil {
			t.Errorf("light %s is taken", strings.Join(args, " "))
		}
	}
}

func TestATestRunnersOptionsAreNotABrowsers(t *testing.T) {
	for _, args := range [][]string{
		{"--webdriver", "127.0.0.1:4444"},
		{"--address", "127.0.0.1:4444"},
		{"--host-map", "web-platform.test=127.0.0.1"},
		{"--trust-ca", "ca.pem"},
		{"--script-budget", "10s"},
		{"browse", "--address", "127.0.0.1:4444"},
		{"browse", "--host-map", "web-platform.test=127.0.0.1"},
		{"browse", "--trust-ca", "ca.pem"},
		{"browse", "--script-budget", "10s"},
		{"--host-map", "web-platform.test=127.0.0.1", "drive", "--address", "127.0.0.1:4444"},
	} {
		if _, _, err := parse(args); err == nil || !strings.Contains(err.Error(), "unknown flag") {
			t.Errorf("light %s: %v", strings.Join(args, " "), err)
		}
	}
}

func TestVersionSaysWhatWasBuilt(t *testing.T) {
	out := said(t, "--version")
	if !strings.Contains(out, "light "+version) || !strings.Contains(out, commit) {
		t.Fatalf("--version says %q", out)
	}
}

func TestHelpNamesTheCommandsTheAddressAndTheFolders(t *testing.T) {
	out := said(t, "--help")
	for _, want := range []string{
		"Usage: light <command> [flags]", "browse [<address>]", "drive --address=HOST:PORT [<address>]",
		"--dir=FOLDER", "$LIGHT_DIR", "--downloads=FOLDER", "$LIGHT_DOWNLOADS", "--version",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the help lacks %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"--webdriver", "--host-map", "--trust-ca", "--script-budget", "--patient"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("the help of light offers %q:\n%s", unwanted, out)
		}
	}
}

func TestHelpOfDriveNamesATestRunnersOptions(t *testing.T) {
	out := said(t, "drive", "--help")
	for _, want := range []string{
		"Usage: light drive --address=HOST:PORT [<address>] [flags]", "$LIGHT_WEBDRIVER",
		"--host-map=NAME=ADDRESS;…", "--trust-ca=FILE", "--script-budget=DURATION", "--dir=FOLDER",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the help of light drive lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "--patient") || strings.Contains(said(t, "browse", "--help"), "--patient") {
		t.Error("the help offers --patient, which is for captures and tests")
	}
}
