package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheTestNetworkIsOnlyForADrivenLight(t *testing.T) {
	t.Setenv("LIGHT_WEBDRIVER", "")
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, []byte("no certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--host-map", "web-platform.test=127.0.0.1"}, {"--trust-ca", ca},
	} {
		if _, err := parse(args); err == nil || !strings.Contains(err.Error(), "--webdriver") {
			t.Errorf("light %s: %v", strings.Join(args, " "), err)
		}
	}
	if transport, err := parsed(t, "--webdriver", "127.0.0.1:4444").transport(); transport != nil || err != nil {
		t.Fatalf("a driven light given neither dials through %v, %v", transport, err)
	}
	if transport, err := parsed(t).transport(); transport != nil || err != nil {
		t.Fatalf("light dials through %v, %v", transport, err)
	}
	cli := parsed(t, "--webdriver", "127.0.0.1:4444", "--host-map", "web-platform.test=127.0.0.1;other.test=::1")
	if cli.HostMap["web-platform.test"] != "127.0.0.1" || cli.HostMap["other.test"] != "::1" {
		t.Fatalf("--host-map gives %v", cli.HostMap)
	}
	transport, err := cli.transport()
	if _, dials := transport.(*http.Transport); !dials || err != nil {
		t.Fatalf("a driven light given names dials through %v, %v", transport, err)
	}
	for _, args := range [][]string{
		{"--webdriver", "127.0.0.1:4444", "--host-map", "web-platform.test=192.168.1.10"},
		{"--webdriver", "127.0.0.1:4444", "--trust-ca", ca},
	} {
		if transport, err = parsed(t, args...).transport(); transport != nil || err == nil {
			t.Errorf("light %s dials through %v, %v", strings.Join(args, " "), transport, err)
		}
	}
}
