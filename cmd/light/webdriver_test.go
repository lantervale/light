package main

import "testing"

func TestADrivenLightKeepsNothingOfThePlayers(t *testing.T) {
	t.Setenv("EMBER_HIDDEN", "")
	t.Setenv("EMBER_AUTOMATION", "")
	t.Setenv("LIGHT_DIR", "")
	t.Setenv("LIGHT_WEBDRIVER", "")
	if cli := parsed(t); cli.WebDriver != "" || cli.options().Driven {
		t.Fatalf("light is driven unasked: %+v", cli)
	}
	cli := parsed(t, "--webdriver", "127.0.0.1:4444")
	if cli.WebDriver != "127.0.0.1:4444" || cli.dir() != "" || !cli.options().Driven {
		t.Fatalf("--webdriver gives the address %q, the folder %q and driven %v", cli.WebDriver, cli.dir(),
			cli.options().Driven)
	}
	if got := parsed(t, "--webdriver", "127.0.0.1:4444", "--dir", "/kept").dir(); got != "/kept" {
		t.Fatalf("a driven light given a folder keeps its files in %q", got)
	}
	t.Setenv("LIGHT_WEBDRIVER", "localhost:9515")
	if cli = parsed(t); cli.WebDriver != "localhost:9515" || cli.dir() != "" || !cli.options().Driven {
		t.Fatalf("LIGHT_WEBDRIVER gives %+v", cli)
	}
}
