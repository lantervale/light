package model_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/model"
)

func TestLightsOwnManifestIsPinnedToItsPublishers(t *testing.T) {
	m := model.Pinned()
	if err := m.Check(); err != nil {
		t.Fatal(err)
	}
	// The version the library of that tag says it is, which the helper
	// compares before it uses it.
	if m.Runtime.Tag != "b12359" || m.Runtime.Version != "1.0.0-dev" {
		t.Errorf("the library is %s, which says it is %q", m.Runtime.Tag, m.Runtime.Version)
	}
	for _, system := range []string{"darwin/arm64", "darwin/amd64", "linux/amd64", "linux/arm64", "windows/amd64"} {
		goos, goarch, _ := strings.Cut(system, "/")
		a, ok := m.Archive(goos, goarch)
		if !ok || !strings.Contains(a.URL, "/ggml-org/llama.cpp/releases/download/"+m.Runtime.Tag+"/") {
			t.Errorf("the library for %s comes from %q", system, a.URL)
		}
	}
	if _, ok := m.Archive("plan9", "386"); ok {
		t.Error("the manifest has a library for plan9")
	}
	proposed := 0
	for _, d := range m.Models {
		if !strings.HasPrefix(d.URL, "https://huggingface.co/") || !strings.Contains(d.URL, "/resolve/") ||
			strings.Contains(d.URL, "/resolve/main/") {
			t.Errorf("%s is not pinned to a commit of its publisher's repository: %s", d.ID, d.URL)
		}
		if d.Licence != "Apache-2.0" && d.Licence != "MIT" {
			t.Errorf("%s is under %s", d.ID, d.Licence)
		}
		if d.RecommendedFrom != nil {
			proposed++
			if !d.Tried {
				t.Errorf("%s is proposed and was never run", d.ID)
			}
		}
		if model.Known(d.ID) != (d.Kind == model.KindChat) {
			t.Errorf("Known(%s) is %v", d.ID, model.Known(d.ID))
		}
	}
	if proposed == 0 || model.Known("unheard-of") || model.Known(model.RuntimeID) {
		t.Fatal("the manifest proposes nothing, or knows what it has not")
	}
	if e, ok := m.Embedding(); !ok || e.Kind != model.KindEmbedding {
		t.Fatalf("the manifest's embedding model is %+v", e)
	}
	if got := m.Recommended(0); got.ID != "qwen2.5-1.5b-instruct-q4" {
		t.Fatalf("a device of unknown memory is proposed %q", got.ID)
	}
}

func TestTheBiggerModelIsProposedOnlyWhereItWasFoundQuick(t *testing.T) {
	const small, big = "qwen2.5-1.5b-instruct-q4", "qwen3-4b-q4"
	m := model.Pinned()
	for _, c := range []struct {
		gb           int64
		goos, goarch string
		want         string
	}{
		{0, "darwin", "arm64", small}, {8, "darwin", "arm64", small}, {15, "darwin", "arm64", small},
		{16, "darwin", "arm64", big}, {64, "darwin", "arm64", big},
		{64, "darwin", "amd64", small}, {64, "linux", "amd64", small}, {64, "linux", "arm64", small},
		{64, "windows", "amd64", small},
	} {
		if got := m.RecommendedFor(c.gb<<30, c.goos, c.goarch); got.ID != c.want {
			t.Errorf("%s/%s with %d GB is proposed %q, want %q", c.goos, c.goarch, c.gb, got.ID, c.want)
		}
	}
	if got := m.Recommended(64 << 30); got != m.RecommendedFor(64<<30, runtime.GOOS, runtime.GOARCH) {
		t.Errorf("this system is proposed %q", got.ID)
	}
	if d, _ := m.Model(big); d.MinMemory > *d.RecommendedFrom {
		t.Errorf("%s is proposed from %d GB and needs %d", big, *d.RecommendedFrom, d.MinMemory)
	}
}

func TestAManifestThatCouldFetchAnythingIsRefused(t *testing.T) {
	good, err := json.Marshal(model.Pinned())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = model.ParseManifest(good); err != nil {
		t.Fatal(err)
	}
	spoil := map[string]func(m *model.Manifest){
		"a model from elsewhere": func(m *model.Manifest) { m.Models[0].URL = "https://example.com/model.gguf" },
		"a model over http": func(m *model.Manifest) {
			m.Models[0].URL = strings.Replace(m.Models[0].URL,
				"https", "http", 1)
		},
		"a model with no digest":        func(m *model.Manifest) { m.Models[0].SHA256 = "" },
		"a model with a short digest":   func(m *model.Manifest) { m.Models[0].SHA256 = "abcd" },
		"a model with no size":          func(m *model.Manifest) { m.Models[0].Size = 0 },
		"a model with no licence":       func(m *model.Manifest) { m.Models[0].Licence = "" },
		"a file name with a folder":     func(m *model.Manifest) { m.Models[0].File = "../" + m.Models[0].File },
		"two models of one id":          func(m *model.Manifest) { m.Models[1].ID = m.Models[0].ID },
		"a model named as the library":  func(m *model.Manifest) { m.Models[1].ID = model.RuntimeID },
		"a model of no kind":            func(m *model.Manifest) { m.Models[0].Kind = "vision" },
		"no model proposed":             func(m *model.Manifest) { m.Models[0].RecommendedFrom = nil },
		"none proposed to every system": func(m *model.Manifest) { m.Models[0].RecommendedOn = "darwin/arm64" },
		"two models proposed alike": func(m *model.Manifest) {
			m.Models[1].RecommendedFrom = m.Models[0].RecommendedFrom
		},
		"an embedding model proposed": func(m *model.Manifest) {
			e := len(m.Models) - 1
			m.Models[e].RecommendedFrom = new(8)
		},
		"a library from elsewhere": func(m *model.Manifest) {
			m.Runtime.Archives[0].URL = "https://example.com/llama.tar.gz"
		},
		"a library of another release": func(m *model.Manifest) { m.Runtime.Tag = "b1" },
		"a library with a path that climbs": func(m *model.Manifest) {
			m.Runtime.Archives[0].URL = strings.Replace(m.Runtime.Archives[0].URL, "/"+m.Runtime.Tag+"/",
				"/"+m.Runtime.Tag+"/../x/", 1)
		},
		"two libraries for a system": func(m *model.Manifest) { m.Runtime.Archives[1] = m.Runtime.Archives[0] },
		"no library version":         func(m *model.Manifest) { m.Runtime.Version = "" },
	}
	for name, change := range spoil {
		var m model.Manifest
		if err = json.Unmarshal(good, &m); err != nil {
			t.Fatal(err)
		}
		change(&m)
		data, _ := json.Marshal(m)
		if _, err = model.ParseManifest(data); err == nil {
			t.Errorf("a manifest with %s is taken", name)
		}
	}
	if _, err = model.ParseManifest([]byte("{")); err == nil {
		t.Error("half a manifest is taken")
	}
}

func TestTheModelProposedFollowsTheDevicesMemory(t *testing.T) {
	for memory, want := range map[int64]string{0: small, 4 << 30: small, 16<<30 - 1: small, 16 << 30: big, 64 << 30: big} {
		opts, b := options(t, "ok")
		opts.Memory = memory
		if memory == 0 {
			opts.Memory = 1
		}
		b.open(opts, model.Settings{On: true})
		got, ok := b.Recommended()
		if !ok || got.ID != want || !got.Recommended {
			t.Errorf("a device of %d GB is proposed %q", memory>>30, got.ID)
		}
		proposed := 0
		for _, e := range b.Catalogue() {
			if e.Recommended {
				proposed++
			}
		}
		if proposed != 1 {
			t.Errorf("a device of %d GB is proposed %d models", memory>>30, proposed)
		}
		if chosen := b.Status().Chosen.ID; chosen != want {
			t.Errorf("with no model named, a device of %d GB has %q chosen", memory>>30, chosen)
		}
	}
}

func TestTheCatalogueSaysWhatEachEntryIs(t *testing.T) {
	b := started(t, "ok")
	entries := b.Catalogue()
	if len(entries) != 4 || entries[0].ID != model.RuntimeID || entries[0].Kind != model.KindRuntime {
		t.Fatalf("the catalogue is %+v", entries)
	}
	if lib := entries[0]; lib.Name != "llama.cpp 1.0.0" || lib.Licence != "MIT" || lib.Size == 0 ||
		lib.Publisher != "ggml-org" ||
		lib.Install.State != model.Installed {
		t.Fatalf("the library's entry is %+v", lib)
	}
	got := b.entry(big)
	if got.Name != "Big" || got.Publisher != "A publisher" || got.Kind != model.KindChat || got.Size == 0 ||
		got.Licence != "Apache-2.0" || got.Context != 4096 || got.MinMemory != 8<<30 || got.Recommended || got.Tried ||
		!strings.HasPrefix(got.Source, "https://") || got.Install.State != model.NotInstalled {
		t.Fatalf("an entry is %+v", got)
	}
	if e := b.entry(small); !e.Recommended || !e.Tried || e.Install.State != model.Installed {
		t.Fatalf("the installed, proposed entry is %+v", e)
	}
	if e := b.entry(embeds); e.Kind != model.KindEmbedding || e.Recommended {
		t.Fatalf("the embedding entry is %+v", e)
	}
	if err := b.Install("unheard-of"); err == nil {
		t.Fatal("an unknown entry is installed")
	}
}

func TestGrammarsKeepAnAnswerToWordsOrJSON(t *testing.T) {
	if got := model.Choice("news", `a "shop"`); got != `root ::= "news" | "a \"shop\""` {
		t.Fatalf("the grammar of a choice is %s", got)
	}
	if !strings.HasPrefix(model.JSON, "root ::= ") {
		t.Fatal("the JSON grammar has no root")
	}
}

func TestADevelopersFilesAreNamedByOneVariable(t *testing.T) {
	dev, err := model.ParseDev(
		"library=/opt/homebrew/lib; backends=/opt/homebrew/opt/ggml/libexec;model=/m/q.gguf;embedding=/m/e.gguf")
	want := model.Dev{Library: "/opt/homebrew/lib", Backends: "/opt/homebrew/opt/ggml/libexec", Model: "/m/q.gguf",
		Embedding: "/m/e.gguf"}
	if err != nil || dev != want {
		t.Fatalf("%+v, %v", dev, err)
	}
	if dev, err = model.ParseDev("library=/lib;model=/m.gguf"); err != nil || dev.Backends != "/lib" {
		t.Fatalf("the backends are not the library's folder: %+v, %v", dev, err)
	}
	if dev, err = model.ParseDev("  "); err != nil || dev != (model.Dev{}) {
		t.Fatalf("no value is %+v, %v", dev, err)
	}
	for _, bad := range []string{"library=/lib", "model=/m.gguf", "library=lib;model=/m.gguf",
		"library=/lib;model=/m.gguf;port=/x", "nonsense"} {
		if _, err = model.ParseDev(bad); err == nil {
			t.Errorf("%q is taken", bad)
		}
	}
}

func TestTheHelperIsFoundBesideLight(t *testing.T) {
	name := model.HelperName
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "MacOS", "light")
	for _, d := range []string{"MacOS", "Helpers"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if got := model.FindHelper(exe); got != "" {
		t.Fatalf("with none there the helper is %q", got)
	}
	inApp := filepath.Join(dir, "Helpers", name)
	if err := os.WriteFile(inApp, []byte("helper"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, _ := filepath.EvalSymlinks(model.FindHelper(exe)); got != evaluated(t, inApp) {
		t.Fatalf("in an app the helper is %q", got)
	}
	beside := filepath.Join(dir, "MacOS", name)
	if err := os.WriteFile(beside, []byte("helper"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, _ := filepath.EvalSymlinks(model.FindHelper(exe)); got != evaluated(t, beside) {
		t.Fatalf("beside light the helper is %q", got)
	}
}

func evaluated(t *testing.T, path string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestThreadsLeaveHalfTheFastCoresToTheRest(t *testing.T) {
	if n := model.Threads(); n < 2 || n > 8 {
		t.Fatalf("light computes on %d threads", n)
	}
	if memory := model.Memory(); memory < 1<<30 {
		t.Fatalf("this machine is said to have %d bytes of memory", memory)
	}
}
