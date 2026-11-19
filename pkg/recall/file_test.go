package recall_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/recall"
)

func TestWhatPagesSaidIsKeptInAFileOnlyItsUserReads(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profile")
	path := filepath.Join(dir, "recall.bin")
	s, err := recall.Load(path, recall.MaxBytes)
	if err != nil || s.Len() != 0 {
		t.Fatalf("no file reads as %d pages, %v", s.Len(), err)
	}
	s = ferries(t)
	if err = recall.Save(path, s); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		for name, want := range map[string]os.FileMode{dir: 0o700, path: 0o600} {
			if info, statErr := os.Stat(name); statErr != nil || info.Mode().Perm() != want {
				t.Fatalf("%s is %v, %v", name, info.Mode().Perm(), statErr)
			}
		}
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(left) != 0 {
		t.Fatalf("the draft is left: %v", left)
	}
	again, err := recall.Load(path, recall.MaxBytes)
	if err != nil || again.Len() != 3 || again.Size() != s.Size() {
		t.Fatalf("read again there are %d pages in %d bytes, %v", again.Len(), again.Size(), err)
	}
	got := again.Search(axis(0), recall.Query{Threshold: 0.5, Most: 8})
	if found(got) != found(s.Search(axis(0), recall.Query{Threshold: 0.5, Most: 8})) || !got[0].At.Equal(day()) {
		t.Fatalf("read again the search finds %s", found(got))
	}
	small, err := recall.Load(path, s.Size()/2)
	if err != nil || small.Len() >= 3 || small.Has("https://gazette.example/ferry") {
		t.Fatalf("read with a smaller cap there are %d pages, %v", small.Len(), err)
	}
	s.Clear()
	if err = recall.Save(path, s); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("with nothing to keep the file is still there: %v", err)
	}
	if err = recall.Save(path, s); err != nil {
		t.Fatalf("with nothing to keep and no file: %v", err)
	}
}

func TestAFileThatIsNotWholeIsRefused(t *testing.T) {
	whole := ferries(t).Encode()
	if s, err := recall.Decode(whole, recall.MaxBytes); err != nil || s.Len() != 3 {
		t.Fatalf("the whole file reads as %d pages, %v", s.Len(), err)
	}
	huge := bytes.Clone(whole)
	copy(huge[len(recall.Magic)+8:], []byte{0xff, 0xff, 0xff, 0x7f})
	version := bytes.Clone(whole)
	version[len(recall.Magic)] = 9
	nan := bytes.Clone(whole)
	copy(nan[len(nan)-4:], []byte{0, 0, 0xc0, 0x7f})
	for name, data := range map[string][]byte{
		"empty":                     {},
		"text":                      []byte(`{"pages": []}`),
		"cut":                       whole[:len(whole)-3],
		"cut in its head":           whole[:len(recall.Magic)+2],
		"longer":                    append(bytes.Clone(whole), 0),
		"of too many pages":         huge,
		"of a version to come":      version,
		"with no number":            nan,
		"of a text that is not one": bytes.Replace(whole, []byte("bowline"), []byte("bow\xffine"), 1),
	} {
		s, err := recall.Decode(data, recall.MaxBytes)
		if err == nil || s == nil || s.Len() != 0 {
			t.Errorf("a file %s reads as %v, %v", name, s, err)
		}
		if err != nil && strings.Contains(err.Error(), "bowline") {
			t.Errorf("the error of a file %s says what a page said: %v", name, err)
		}
	}
	path := filepath.Join(t.TempDir(), "recall.bin")
	if err := os.WriteFile(path, whole[:40], 0o600); err != nil {
		t.Fatal(err)
	}
	if s, err := recall.Load(path, recall.MaxBytes); err == nil || s.Len() != 0 {
		t.Fatalf("a broken file loads as %d pages, %v", s.Len(), err)
	}
}

func FuzzDecode(f *testing.F) {
	whole := ferries(f).Encode()
	f.Add(whole)
	f.Add(whole[:len(whole)/2])
	f.Fuzz(func(t *testing.T, data []byte) {
		if s, err := recall.Decode(data, recall.MaxBytes); err != nil && s.Len() != 0 {
			t.Fatalf("a refused file leaves %d pages", s.Len())
		}
	})
}
