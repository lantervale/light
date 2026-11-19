package model

import (
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// RuntimeID is the catalogue entry of the library that runs the models
// (llama.cpp). Installing a model installs it too.
const RuntimeID = "llama.cpp"

// DefaultContext is the most tokens a context is given, whatever the
// model was trained for: memory grows with it.
const DefaultContext = 8192

// What an entry is.
const (
	KindRuntime   = "runtime"
	KindChat      = "chat"
	KindEmbedding = "embedding"
)

//go:embed manifest.json
var pinned []byte

// Manifest is what light may download for the model: nothing else is
// ever fetched, and each file is checked against its digest here before
// it is used.
type Manifest struct {
	Version int        `json:"version"`
	Runtime Runtime    `json:"runtime"`
	Models  []Download `json:"models"`
}

// Runtime is the llama.cpp release the helper is built for.
type Runtime struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Publisher string `json:"publisher"`
	Licence   string `json:"licence"`
	// Version is what the library must say it is (llama_version): the
	// helper refuses any other. Tag is the release its archives are of.
	Version  string    `json:"version"`
	Tag      string    `json:"tag"`
	Archives []Archive `json:"archives"`
}

// Archive is the library for one system.
type Archive struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Download is one model of the manifest.
type Download struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Publisher string `json:"publisher"`
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	File      string `json:"file"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Licence   string `json:"licence"`
	// Context is the context it is run with.
	Context int `json:"context"`
	// MinMemory is the least memory a device needs for it, in GB.
	MinMemory int `json:"min_memory_gb"`
	// RecommendedFrom makes it the model proposed to devices of that
	// much memory, in GB, and more, up to the next one's; nil for a
	// model never proposed.
	RecommendedFrom *int `json:"recommended_from_gb,omitempty"`
	// RecommendedOn keeps that proposal to these systems, as "os/arch"
	// with spaces between: a model that is only quick enough where a
	// graphics card computes it. Empty is every system.
	RecommendedOn string `json:"recommended_on,omitempty"`
	// Tried is whether light's makers ran it.
	Tried bool `json:"tried"`
	// Why is one line on what it is good for.
	Why string `json:"why,omitempty"`
	// QueryPrefix and DocumentPrefix start the texts an embedding model
	// is given.
	QueryPrefix    string `json:"query_prefix,omitempty"`
	DocumentPrefix string `json:"document_prefix,omitempty"`
}

//nolint:gochecknoglobals // the manifest light is built with, read once
var readPinned = sync.OnceValue(func() Manifest {
	m, err := ParseManifest(pinned)
	if err != nil {
		panic("light's own model manifest does not read: " + err.Error())
	}
	return m
})

// Pinned is the manifest light is built with.
func Pinned() Manifest {
	m := readPinned()
	m.Runtime.Archives, m.Models = slices.Clone(m.Runtime.Archives), slices.Clone(m.Models)
	return m
}

// ParseManifest reads a manifest and checks it.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("reading the manifest: %w", err)
	}
	if err := m.Check(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// The only places a manifest downloads from: llama.cpp's own releases
// and the publishers' repositories on Hugging Face.
const (
	runtimeSource = "https://github.com/ggml-org/llama.cpp/releases/download/"
	modelSource   = "https://huggingface.co/"
)

// Check is why m cannot be used, nil when it can: every file has a
// size, a digest and an https address at its publisher's, ids are
// their own, and one model is proposed for each amount of memory.
func (m Manifest) Check() error {
	if m.Runtime.ID != RuntimeID || m.Runtime.Version == "" || m.Runtime.Tag == "" {
		return errors.New("the manifest names no llama.cpp release")
	}
	systems := map[string]bool{}
	for _, a := range m.Runtime.Archives {
		system := a.OS + "/" + a.Arch
		if systems[system] {
			return fmt.Errorf("the manifest has two archives for %s", system)
		}
		systems[system] = true
		if err := checkFile(system, a.URL, runtimeSource+m.Runtime.Tag+"/", a.Size, a.SHA256); err != nil {
			return err
		}
	}
	ids, from := map[string]bool{RuntimeID: true}, map[int]string{}
	for _, d := range m.Models {
		if d.ID == "" || ids[d.ID] {
			return fmt.Errorf("the manifest's model %q has no id of its own", d.ID)
		}
		ids[d.ID] = true
		if d.Kind != KindChat && d.Kind != KindEmbedding {
			return fmt.Errorf("the manifest's %s is of no known kind (%q)", d.ID, d.Kind)
		}
		if d.File == "" || d.File != filepath.Base(d.File) || !filepath.IsLocal(d.File) || d.Context <= 0 || d.Licence == "" {
			return fmt.Errorf("the manifest's %s lacks a file name, a context or a licence", d.ID)
		}
		if err := checkFile(d.ID, d.URL, modelSource, d.Size, d.SHA256); err != nil {
			return err
		}
		if d.RecommendedFrom == nil {
			continue
		}
		if d.Kind != KindChat || *d.RecommendedFrom < 0 || from[*d.RecommendedFrom] != "" {
			return fmt.Errorf("the manifest proposes %s and %s for the same memory, or a model that does not chat",
				d.ID, from[*d.RecommendedFrom])
		}
		from[*d.RecommendedFrom] = d.ID
	}
	if least, _ := m.Model(from[0]); from[0] == "" || least.RecommendedOn != "" {
		return errors.New("the manifest proposes no model for a device of little memory, whatever its system")
	}
	return nil
}

// secure is the one scheme anything is downloaded over.
const secure = "https"

func checkFile(name, address, source string, size int64, digest string) error {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != secure || !strings.HasPrefix(address, source) || path.Clean(u.Path) != u.Path {
		return fmt.Errorf("the manifest's %s is not downloaded from %s", name, source)
	}
	if sum, bad := hex.DecodeString(digest); bad != nil || len(sum) != 32 || size <= 0 {
		return fmt.Errorf("the manifest's %s has no size or no SHA-256 digest", name)
	}
	return nil
}

// Archive is the library's archive for a system, and whether the
// manifest has one.
func (m Manifest) Archive(goos, goarch string) (Archive, bool) {
	i := slices.IndexFunc(m.Runtime.Archives, func(a Archive) bool { return a.OS == goos && a.Arch == goarch })
	if i < 0 {
		return Archive{}, false
	}
	return m.Runtime.Archives[i], true
}

// Model is the model of that id, and whether the manifest has it.
func (m Manifest) Model(id string) (Download, bool) {
	i := slices.IndexFunc(m.Models, func(d Download) bool { return d.ID == id })
	if i < 0 {
		return Download{}, false
	}
	return m.Models[i], true
}

// Embedding is the model texts are embedded with: light's choice, the
// first of the manifest.
func (m Manifest) Embedding() (Download, bool) {
	i := slices.IndexFunc(m.Models, func(d Download) bool { return d.Kind == KindEmbedding })
	if i < 0 {
		return Download{}, false
	}
	return m.Models[i], true
}

// Recommended is the chat model proposed to this system with that
// much memory, in bytes (RecommendedFor).
func (m Manifest) Recommended(memory int64) Download {
	return m.RecommendedFor(memory, runtime.GOOS, runtime.GOARCH)
}

// RecommendedFor is the chat model proposed to a device of that system
// and that much memory, in bytes: the one whose RecommendedFrom is the
// highest the memory reaches, among those proposed to the system; with
// an unknown memory (0), the one for the least.
func (m Manifest) RecommendedFor(memory int64, goos, goarch string) Download {
	var best Download
	for _, d := range m.Models {
		if d.RecommendedFrom == nil || int64(*d.RecommendedFrom)<<30 > memory {
			continue
		}
		if d.RecommendedOn != "" && !slices.Contains(strings.Fields(d.RecommendedOn), goos+"/"+goarch) {
			continue
		}
		if best.RecommendedFrom == nil || *d.RecommendedFrom > *best.RecommendedFrom {
			best = d
		}
	}
	return best
}

// Known reports whether id is a chat model of light's manifest: what
// the settings may name.
func Known(id string) bool {
	d, ok := Pinned().Model(id)
	return ok && d.Kind == KindChat
}

// InstallState is how far an entry is from being on this device.
type InstallState int

const (
	NotInstalled InstallState = iota
	Installing
	Installed
	// InstallFailed is a download that stopped, Install.Err saying why;
	// what came is kept, and Install goes on from it.
	InstallFailed
)

// Install is an entry's state on this device.
type Install struct {
	State InstallState
	// Done of Total bytes came, while Installing.
	Done, Total int64
	Err         string
}

// Entry is one row of the catalogue: the library, or a model.
type Entry struct {
	ID        string
	Name      string
	Publisher string
	Kind      string
	// Size is its download, in bytes; Source where it comes from.
	Size    int64
	Source  string
	Licence string
	Context int
	// MinMemory is the least memory it needs, in bytes.
	MinMemory int64
	// Recommended is whether it is the model proposed to this device;
	// Why says what it is good for.
	Recommended bool
	Why         string
	// Tried is whether light's makers ran it.
	Tried   bool
	Install Install
}

func (d Download) entry() Entry {
	return Entry{
		ID: d.ID, Name: d.Name, Publisher: d.Publisher, Kind: d.Kind, Size: d.Size, Source: d.URL,
		Licence: d.Licence, Context: d.Context, MinMemory: int64(d.MinMemory) << 30, Why: d.Why, Tried: d.Tried,
	}
}

func (r Runtime) entry(a Archive) Entry {
	return Entry{
		ID: r.ID, Name: r.Name + " " + r.Version, Publisher: r.Publisher, Kind: KindRuntime, Size: a.Size,
		Source: a.URL, Licence: r.Licence, Tried: true,
	}
}

// Choice is a grammar that keeps an answer to one of words, for
// Request.Grammar: a page's category among a few.
func Choice(words ...string) string {
	quoted := make([]string, len(words))
	for i, word := range words {
		quoted[i] = strconv.Quote(word)
	}
	return "root ::= " + strings.Join(quoted, " | ")
}

// JSON is a grammar that keeps an answer to one JSON value, for
// Request.Grammar.
const JSON = `root ::= ws value ws
value ::= object | array | string | number | "true" | "false" | "null"
object ::= "{" ws ( string ws ":" ws value ( ws "," ws string ws ":" ws value )* )? ws "}"
array ::= "[" ws ( value ( ws "," ws value )* )? ws "]"
string ::= "\"" ( [^"\\\x7F\x00-\x1F] | "\\" ( ["\\/bfnrt] | "u" hex hex hex hex ) )* "\""
hex ::= [0-9a-fA-F]
number ::= "-"? ( "0" | [1-9] [0-9]* ) ( "." [0-9]+ )? ( [eE] [-+]? [0-9]+ )?
ws ::= [ \t\n]*
`
