package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// HelperName is the helper's file name without its extension.
const HelperName = "light-model"

// The environment a developer points light at files of their own with.
// A released light ignores both.
const (
	// EnvHelper is the path of the helper, in place of the one beside
	// light.
	EnvHelper = "LIGHT_MODEL_HELPER"
	// EnvDev names a llama.cpp and a model that are already on the
	// machine, in place of light's downloads:
	// "library=DIR;backends=DIR;model=FILE;embedding=FILE".
	EnvDev = "LIGHT_MODEL_DEV"
)

// Dev is a developer's own llama.cpp and model. Its zero value is none.
type Dev struct {
	Library, Backends, Model, Embedding string
}

// ParseDev reads the value of EnvDev: the library's folder and the
// model are needed, the backends are the library's folder unless named.
func ParseDev(value string) (Dev, error) {
	var d Dev
	if strings.TrimSpace(value) == "" {
		return d, nil
	}
	for part := range strings.SplitSeq(value, ";") {
		name, path, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found || !filepath.IsAbs(path) {
			return Dev{}, fmt.Errorf("%s: %q is not name=/absolute/path", EnvDev, part)
		}
		switch name {
		case "library":
			d.Library = path
		case "backends":
			d.Backends = path
		case "model":
			d.Model = path
		case "embedding":
			d.Embedding = path
		default:
			return Dev{}, fmt.Errorf("%s: %q is not library, backends, model or embedding", EnvDev, name)
		}
	}
	if d.Library == "" || d.Model == "" {
		return Dev{}, fmt.Errorf("%s names no library or no model", EnvDev)
	}
	if d.Backends == "" {
		d.Backends = d.Library
	}
	return d, nil
}

// FindHelper is the helper beside the program at exe, "" when there is
// none: light-model next to light, or in the Helpers folder of the app
// light is in on macOS.
func FindHelper(exe string) string {
	name := HelperName
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	for _, candidate := range []string{filepath.Join(dir, name), filepath.Join(dir, "..", "Helpers", name)} {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return filepath.Clean(candidate)
		}
	}
	return ""
}

// files is where everything of the model is kept: the model folder of
// the profile.
//
//	runtime/<tag>-<os>-<arch>/   the llama.cpp libraries, unpacked
//	runtime/<tag>-<os>-<arch>.json  where its libraries and backends are
//	models/<file>, models/<id>.json  a model, and its digest once checked
//	downloads/                   what is being downloaded
//	own.json                     the user's own files, as inspected
//	cache/                       the helper's home: what it unpacks to run
//	helper.log                   what the helper wrote on its standard error
type files struct{ dir string }

func (f files) runtimeDir(r Runtime) string {
	return filepath.Join(f.dir, "runtime", r.Tag+"-"+runtime.GOOS+"-"+runtime.GOARCH)
}
func (f files) runtimeRecord(r Runtime) string { return f.runtimeDir(r) + ".json" }
func (f files) model(d Download) string        { return filepath.Join(f.dir, "models", d.File) }
func (f files) modelRecord(d Download) string  { return filepath.Join(f.dir, "models", d.ID+".json") }
func (f files) downloads() string              { return filepath.Join(f.dir, "downloads") }
func (f files) own() string                    { return filepath.Join(f.dir, "own.json") }
func (f files) cache() string                  { return filepath.Join(f.dir, "cache") }
func (f files) log() string                    { return filepath.Join(f.dir, "helper.log") }

// placed is where an unpacked library's parts are, from its folder.
type placed struct {
	Library  string `json:"library"`
	Backends string `json:"backends"`
}

// installed is the note left beside a model once its digest was checked.
type installed struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// runtimePaths is the folders of the installed library and its
// backends, and whether it is installed.
func (f files) runtimePaths(r Runtime) (string, string, bool) {
	var p placed
	if !readJSON(f.runtimeRecord(r), &p) || !filepath.IsLocal(p.Library) || !filepath.IsLocal(p.Backends) {
		return "", "", false
	}
	root := f.runtimeDir(r)
	library, backends := filepath.Join(root, p.Library), filepath.Join(root, p.Backends)
	if info, err := os.Stat(library); err != nil || !info.IsDir() {
		return "", "", false
	}
	return library, backends, true
}

// modelPath is the installed file of d, and whether it is installed:
// its note is there and the file has the size it had when checked.
func (f files) modelPath(d Download) (string, bool) {
	var note installed
	if !readJSON(f.modelRecord(d), &note) || note.SHA256 != d.SHA256 {
		return "", false
	}
	info, err := os.Stat(f.model(d))
	if err != nil || info.Size() != d.Size {
		return "", false
	}
	return f.model(d), true
}

// owned is a file of the user's as it was when inspected.
type owned struct {
	File
	Modified time.Time `json:"modified"`
}

func (f files) ownFiles() map[string]owned {
	known := map[string]owned{}
	readJSON(f.own(), &known)
	return known
}

func (f files) keepOwn(o owned) error {
	known := f.ownFiles()
	known[o.Path] = o
	return writeJSON(f.own(), known)
}

func readJSON(path string, into any) bool {
	data, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(data, into) == nil
}

// writeJSON writes value at path, making its folder, whole or not at
// all.
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	draft := path + ".tmp"
	if err = os.WriteFile(draft, data, 0o600); err != nil {
		return err
	}
	return os.Rename(draft, path)
}

// digest is the SHA-256 of the file at path, and its size.
func digest(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = file.Close() }()
	sum := sha256.New()
	size, err := io.Copy(sum, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(sum.Sum(nil)), size, nil
}

// unchanged is why the user's file is not the one that was inspected,
// nil when it is: its size and date are the same, or its digest is.
func (f files) unchanged(o owned) error {
	info, err := os.Stat(o.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%w: %s is no longer there", ErrMissing, o.Path)
	case err != nil:
		return fmt.Errorf("%s cannot be read: %w", o.Path, err)
	case info.Size() == o.Size && info.ModTime().Equal(o.Modified):
		return nil
	}
	sum, _, err := digest(o.Path)
	if err != nil {
		return fmt.Errorf("%s cannot be read: %w", o.Path, err)
	}
	if sum != o.SHA256 {
		return fmt.Errorf("%s changed since it was chosen: choose it again to use it", o.Path)
	}
	o.Modified = info.ModTime()
	return f.keepOwn(o)
}
