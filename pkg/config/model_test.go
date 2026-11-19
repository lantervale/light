package config_test

import (
	"path/filepath"
	"testing"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/model"
)

func TestTheModelIsOffUntilTurnedOnAndKept(t *testing.T) {
	c := config.Default()
	if c.Model || c.ModelID != "" || c.ModelFile != "" || c.ModelThreads != 0 || c.ModelCPU {
		t.Fatalf("the model's defaults are %+v", c)
	}
	if got := c.ModelSettings(); got != (model.Settings{GPU: true}) {
		t.Fatalf("by default the model follows %+v", got)
	}
	known := model.Pinned().Recommended(0).ID
	c.Model, c.ModelID, c.ModelThreads, c.ModelCPU = true, known, 4, true
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(path)
	if err != nil || got != c {
		t.Fatalf("the model's settings are read back as %+v (%v)", got, err)
	}
	if s := got.ModelSettings(); s != (model.Settings{On: true, ID: known, Threads: 4}) {
		t.Fatalf("the model follows %+v", s)
	}
	old, err := config.Decode([]byte(`{"version": 1, "zoom": 150}`))
	if err != nil || old.Model {
		t.Fatalf("a file written before light had a model turns it on: %+v (%v)", old, err)
	}
}

func TestTheModelIsOneOfTheManifestOrAFileNeverBoth(t *testing.T) {
	known := model.Pinned().Recommended(0).ID
	file := filepath.Join(t.TempDir(), "mine.gguf")
	for name, c := range map[string]struct {
		id, file         string
		threads          int
		wantID, wantFile string
		wantThreads      int
	}{
		"a model of the manifest":      {id: known, wantID: known},
		"a model light never heard of": {id: "unheard-of"},
		"the library is no model":      {id: model.RuntimeID},
		"a file of the user's":         {file: " " + file + " ", wantFile: file},
		"a file with no whole path":    {file: "mine.gguf"},
		"a file with a line break":     {file: file + "\n/etc/passwd"},
		"both: the manifest's wins":    {id: known, file: file, wantID: known},
		"a file and an unknown id":     {id: "unheard-of", file: file, wantFile: file},
		"threads within bounds":        {threads: config.MaxModelThreads, wantThreads: config.MaxModelThreads},
		"too many threads":             {threads: config.MaxModelThreads + 1},
		"fewer threads than none":      {threads: -1},
	} {
		in := config.Default()
		in.ModelID, in.ModelFile, in.ModelThreads = c.id, c.file, c.threads
		got := in.Valid()
		if got.ModelID != c.wantID || got.ModelFile != c.wantFile || got.ModelThreads != c.wantThreads {
			t.Errorf("%s: the model is %q, the file %q, on %d threads", name, got.ModelID, got.ModelFile, got.ModelThreads)
		}
		if s := got.ModelSettings(); s.ID != "" && s.File != "" {
			t.Errorf("%s: the model follows both %q and %q", name, s.ID, s.File)
		}
	}
}
