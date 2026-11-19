package assist_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lantervale/light/pkg/assist"
)

func TestAFeatureHelpsUntilItIsTurnedOff(t *testing.T) {
	f := assist.New()
	if !f.On("answers") {
		t.Fatal("a feature nobody turned off does not help")
	}
	if !f.Set("answers", false) || f.On("answers") || f.Set("answers", false) {
		t.Fatal("turning a feature off does not change it once")
	}
	if !f.On("find") {
		t.Fatal("turning one feature off turned another off")
	}
	if !f.Set("answers", true) || !f.On("answers") {
		t.Fatal("a feature turned on again does not help")
	}
}

func TestTheFeaturesAreKeptInAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assist.json")
	f, err := assist.Load(path)
	if err != nil || !f.On("answers") {
		t.Fatalf("no file reads as %v, %v", f, err)
	}
	f.Set("find", false)
	f.Set("answers", false)
	if err = assist.Save(path, f); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{\n  \"off\": [\n    \"answers\",\n    \"find\"\n  ]\n}\n" {
		t.Fatalf("the file holds %q", data)
	}
	again, err := assist.Load(path)
	if err != nil || again.On("find") || again.On("answers") || !again.On("categories") {
		t.Fatalf("read again the features are %v, %v", again, err)
	}
	if err = os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if broken, unread := assist.Load(path); unread == nil || !broken.On("find") {
		t.Fatalf("a broken file reads as %v, %v", broken, unread)
	}
}
