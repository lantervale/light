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

func TestAFeatureToBeAskedForIsOffUntilItIsTurnedOn(t *testing.T) {
	f := assist.New()
	f.Ask("recall")
	if f.On("recall") || !f.On("answers") {
		t.Fatal("a feature to be asked for helps before it was turned on, or took another with it")
	}
	if f.Set("recall", false) || !f.Set("recall", true) || !f.On("recall") || f.Set("recall", true) {
		t.Fatal("turning it on does not change it once")
	}
}

func TestAFeatureAskedForIsKeptInTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assist.json")
	f := assist.New()
	f.Ask("recall")
	f.Set("recall", true)
	if err := assist.Save(path, f); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{\n  \"off\": [],\n  \"on\": [\n    \"recall\"\n  ]\n}\n" {
		t.Fatalf("the file holds %q", data)
	}
	again, err := assist.Load(path)
	again.Ask("recall")
	if err != nil || !again.On("recall") {
		t.Fatalf("read again the feature is %v, %v", again.On("recall"), err)
	}
	if !again.Set("recall", false) || again.On("recall") {
		t.Fatal("it does not turn off again")
	}
	if err = assist.Save(path, again); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{\n  \"off\": []\n}\n" {
		t.Fatalf("turned off again the file holds %q", data)
	}
}
