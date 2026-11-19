package commands_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lantervale/light/cmd/light/commands"
	"github.com/lantervale/light/cmd/light/globals"
)

func driven(t *testing.T, d *commands.Drive, profile *globals.Profile) (http.RoundTripper, time.Duration, bool,
	string) {
	t.Helper()
	opts, err := d.Options(&globals.Build{}, profile)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Driven || !opts.ExactText {
		t.Fatalf("a light a test runner drives is driven: %v, and draws its pages' text exact: %v", opts.Driven,
			opts.ExactText)
	}
	return opts.Transport, opts.ScriptBudget, opts.Patient, opts.Dir
}

func TestADrivenLightKeepsNothingOfThePlayers(t *testing.T) {
	shown(t)
	drive := &commands.Drive{Address: "127.0.0.1:4444"}
	if _, _, _, dir := driven(t, drive, &globals.Profile{}); dir != "" {
		t.Fatalf("a driven light keeps its files in %q", dir)
	}
	if _, _, _, dir := driven(t, drive, &globals.Profile{Dir: "/kept"}); dir != "/kept" {
		t.Fatalf("a driven light given a folder keeps its files in %q", dir)
	}
}

func TestADrivenLightStopsASlowScriptAtItsBudget(t *testing.T) {
	shown(t)
	drive := &commands.Drive{Address: "127.0.0.1:4444"}
	if _, budget, patient, _ := driven(t, drive, &globals.Profile{}); budget != 0 || patient {
		t.Fatalf("a driven light given none gives its scripts %v and waits: %v", budget, patient)
	}
	ten := 10 * time.Second
	drive.ScriptBudget = &ten
	if _, budget, patient, _ := driven(t, drive, &globals.Profile{}); budget != ten || patient {
		t.Fatalf("--script-budget 10s gives the scripts %v and waits: %v", budget, patient)
	}
	if err := drive.Validate(); err != nil {
		t.Fatalf("a budget of %v is refused: %v", ten, err)
	}
	for _, budget := range []time.Duration{0, -time.Second} {
		drive.ScriptBudget = &budget
		if drive.Validate() == nil {
			t.Errorf("a budget of %v is taken", budget)
		}
	}
	if (&commands.Drive{}).Validate() == nil {
		t.Error("a light driven from no address is taken")
	}
}

func TestTheTestNetworkReachesThisMachineOnly(t *testing.T) {
	shown(t)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, []byte("no certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	drive := &commands.Drive{Address: "127.0.0.1:4444"}
	if transport, _, _, _ := driven(t, drive, &globals.Profile{}); transport != nil {
		t.Fatalf("a driven light given neither dials through %v", transport)
	}
	drive.HostMap = map[string]string{"web-platform.test": "127.0.0.1", "other.test": "::1"}
	transport, _, _, _ := driven(t, drive, &globals.Profile{})
	if _, dials := transport.(*http.Transport); !dials {
		t.Fatalf("a driven light given names dials through %v", transport)
	}
	for _, refused := range []*commands.Drive{
		{Address: "127.0.0.1:4444", HostMap: map[string]string{"web-platform.test": "192.168.1.10"}},
		{Address: "127.0.0.1:4444", TrustCA: ca},
		{Address: "127.0.0.1:4444", TrustCA: filepath.Join(t.TempDir(), "missing.pem")},
	} {
		if opts, err := refused.Options(&globals.Build{}, &globals.Profile{}); err == nil {
			t.Errorf("light drive with %+v dials through %v", refused, opts.Transport)
		}
	}
}

func TestDriveListensOnThisMachineOnly(t *testing.T) {
	shown(t)
	t.Setenv("EMBER_HIDDEN", "1")
	for _, address := range []string{"0.0.0.0:0", "192.168.1.10:4444", "example.com:4444", "4444"} {
		drive := &commands.Drive{Address: address}
		err := drive.Run(context.Background(), &globals.Build{}, &globals.Profile{})
		if err == nil {
			t.Fatalf("light drive --address %s runs", address)
		}
	}
}
