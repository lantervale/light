package cookies_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/cookies"
)

func TestTheFileKeepsWhatOutlivesTheRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile", "cookies.json")
	j, clock := jar()
	j.Store(visit(t, "https://www.site.test/dir/page"), []string{
		"kept=1; Max-Age=3600; Domain=site.test; Path=/dir; Secure; HttpOnly; SameSite=Strict",
		"short=1; Max-Age=60",
		"session=1",
	})
	if !j.Changed() {
		t.Fatal("a jar given a cookie that outlives the run is not changed")
	}
	if err := j.Save(path); err != nil {
		t.Fatal(err)
	}
	if j.Changed() {
		t.Error("a saved jar is still changed")
	}
	j.Store(visit(t, "https://www.site.test/"), []string{"other=1"})
	if j.Changed() {
		t.Error("a cookie that ends with the run changed what the file holds")
	}

	clock.at = clock.at.Add(10 * time.Minute)
	back, err := cookies.Load(path, cookies.Clock(clock.now))
	if err != nil {
		t.Fatal(err)
	}
	if back.Len() != 1 || back.Changed() {
		t.Fatalf("the jar read back holds %d cookies, changed %v: want the one neither ended nor of the run alone",
			back.Len(), back.Changed())
	}
	got := back.For(at(t, "https://api.site.test/dir/"))
	if len(got) != 1 {
		t.Fatalf("the cookie read back goes to %d hosts under its domain, want 1", len(got))
	}
	c := got[0]
	if c.Name != "kept" || c.Value != "1" || c.HostOnly || !c.Secure || !c.HTTPOnly || c.SameSite != cookies.Strict ||
		!c.Expires.Equal(start().Add(time.Hour)) || !c.Created.Equal(start()) {
		t.Error("the cookie read back has other attributes than it was saved with")
	}
	if back.Header(visit(t, "http://api.site.test/dir/")) != "" || back.Script(at(t, "https://api.site.test/dir/")) != "" {
		t.Error("the cookie read back is no longer Secure or HttpOnly")
	}
}

func TestTheFileIsThePlayersAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no file modes")
	}
	dir := filepath.Join(t.TempDir(), "profile")
	path := filepath.Join(dir, "cookies.json")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, loose := range []string{path, path + ".tmp"} {
		if err := os.WriteFile(loose, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	j, _ := jar()
	j.Store(visit(t, "https://site.test/"), []string{"a=1; Max-Age=60"})
	if err := j.Save(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the file's mode is %o, want 600", mode)
	}
	if _, err = os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("the draft is left beside the file")
	}
}

func TestAFileThatIsNotOneIsAnEmptyJar(t *testing.T) {
	const secret = "s3cr3t-value"
	for name, data := range map[string]string{
		"not JSON":        `{"version": 1, "cookies": [{"name": "a", "value": "` + secret,
		"another version": `{"version": 99, "cookies": [{"name": "a", "value": "` + secret + `"}]}`,
		"another form":    `{"version": 1, "cookies": {"a": "` + secret + `"}}`,
		"empty":           ``,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cookies.json")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			j, err := cookies.Load(path)
			if err == nil || j == nil || j.Len() != 0 {
				t.Fatalf("a file that is not one is read as a jar: error %v", err != nil)
			}
			if strings.Contains(err.Error(), secret) {
				t.Error("the reason quotes the file")
			}
			j.Store(visit(t, "https://site.test/"), []string{"a=1; Max-Age=60"})
			if err = j.Save(path); err != nil {
				t.Fatal(err)
			}
			if again, failed := cookies.Load(path); failed != nil || again.Len() != 1 {
				t.Error("the jar was not written over the file that was not one")
			}
		})
	}
}

func TestAFileThatIsNotThereIsAnEmptyJar(t *testing.T) {
	j, err := cookies.Load(filepath.Join(t.TempDir(), "none", "cookies.json"))
	if err != nil || j.Len() != 0 {
		t.Errorf("no file is read as %d cookies, error %v", j.Len(), err)
	}
}

func TestWhatTheFileHoldsThatIsNoCookieIsLeftOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.json")
	data := `{"version": 1, "cookies": [
		{"name": "", "value": "1", "domain": "site.test", "path": "/", "expires": "2030-01-01T00:00:00Z"},
		{"name": "nodomain", "value": "1", "domain": "", "path": "/", "expires": "2030-01-01T00:00:00Z"},
		{"name": "session", "value": "1", "domain": "site.test", "path": "/"},
		{"name": "ended", "value": "1", "domain": "site.test", "path": "/", "expires": "2020-01-01T00:00:00Z"},
		{"name": "kept", "value": "1", "domain": "site.test", "path": "/", "same_site": "odd",
			"expires": "2030-01-01T00:00:00Z"}
	]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	j, err := cookies.Load(path, cookies.Clock(start))
	if err != nil {
		t.Fatal(err)
	}
	got := j.For(at(t, "https://site.test/"))
	if len(got) != 1 || got[0].Name != "kept" || got[0].SameSite != cookies.Lax {
		t.Errorf("the jar read back holds %d cookies, want the one that is whole, Lax", len(got))
	}
}
