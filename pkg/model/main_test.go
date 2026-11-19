package model_test

import (
	"archive/tar"
	"bufio"
	"bytes"
	"cmp"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/model"
)

// The test program is its own fake helper: run with envMode set, it
// speaks the helper's protocol on its standard input and output in
// place of running the tests.
const (
	envMode  = "LIGHT_MODEL_TEST_HELPER"
	envNotes = "LIGHT_MODEL_TEST_NOTES"
	envPace  = "LIGHT_MODEL_TEST_PACE"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(envMode); mode != "" {
		helper(mode)
		return
	}
	os.Exit(m.Run())
}

// note is what a fake helper leaves for the test that started it.
type note struct {
	PID   int           `json:"pid"`
	At    int64         `json:"at"`
	Hello model.Command `json:"hello"`
	Env   []string      `json:"env"`
	Asked []string      `json:"asked,omitempty"`
}

func leave(n note) {
	dir := os.Getenv(envNotes)
	if dir == "" {
		return
	}
	data, _ := json.Marshal(n)
	_ = os.WriteFile(filepath.Join(dir, strconv.Itoa(n.PID)+".json"), data, 0o600)
}

// helper is the fake helper. What it does is its mode, and the text of
// the last message it is asked to answer: "crash" leaves in the middle
// of the answer, "hang" stops answering, anything else is answered back
// word by word.
func helper(mode string) {
	out := json.NewEncoder(os.Stdout)
	say := func(ev model.Event) { _ = out.Encode(ev) }
	commands := make(chan model.Command)
	go func() {
		lines := bufio.NewScanner(os.Stdin)
		lines.Buffer(make([]byte, 1<<20), 1<<24)
		for lines.Scan() {
			var c model.Command
			if json.Unmarshal(lines.Bytes(), &c) == nil {
				commands <- c
			}
		}
		close(commands)
	}()
	hello, open := <-commands
	if !open {
		return
	}
	me := note{PID: os.Getpid(), At: time.Now().UnixNano(), Hello: hello, Env: os.Environ()}
	leave(me)
	switch mode {
	case "mute":
		for range commands {
		}
		return
	case "dies":
		fmt.Fprintln(os.Stderr, "dyld: the library is not there")
		os.Exit(3)
	case "garbage":
		_, _ = os.Stdout.WriteString("this is no event\n")
		for range commands {
		}
		return
	case "old":
		say(model.Event{Event: model.EventHello, Protocol: model.Protocol - 1, Build: "old"})
		return
	}
	build := hello.Build
	if mode == "another" {
		build = "0.0.1"
	}
	say(model.Event{Event: model.EventHello, Protocol: model.Protocol, Build: build})
	if hello.Inspect != "" {
		describe(say, hello.Inspect)
		return
	}
	say(model.Event{Event: model.EventStatus, Text: "loading the model"})
	switch mode {
	case "slow":
		for range commands {
		}
		return
	case "refuse":
		say(model.Event{Event: model.EventError, Code: model.CodeVersion,
			Error: "llama.cpp says it is 9.9.9 and light-model is built for " + hello.Llama})
		return
	}
	say(model.Event{Event: model.EventReady, Context: hello.Context, Chat: true, Embeddings: hello.Embedding != "",
		Memory: 1 << 20})
	pace := time.Millisecond
	if ms, err := strconv.Atoi(os.Getenv(envPace)); err == nil {
		pace = time.Duration(ms) * time.Millisecond
	}
	for c := range commands {
		switch c.Op {
		case model.OpCount:
			say(model.Event{Event: model.EventCount, ID: c.ID, Count: len(strings.Fields(c.Text))})
		case model.OpEmbed:
			vectors := make([][]float32, len(c.Texts))
			for i, text := range c.Texts {
				vectors[i] = []float32{float32(len(text)), 0}
			}
			say(model.Event{Event: model.EventVectors, ID: c.ID, Vectors: vectors, Dimensions: 2})
		case model.OpGenerate:
			text := c.Messages[len(c.Messages)-1].Text
			me.Asked = append(me.Asked, text)
			leave(me)
			answer(say, commands, c, text, pace)
		}
	}
}

func describe(say func(model.Event), path string) {
	switch filepath.Base(path) {
	case "chat.gguf", "other.gguf":
		say(model.Event{Event: model.EventFile, Name: "A chat model", Architecture: "qwen2", Parameters: 1_500_000_000,
			TrainContext: 32768, Template: true})
	case "bare.gguf":
		say(model.Event{Event: model.EventFile, Name: "A bare model", Architecture: "llama", TrainContext: 2048})
	case "embed.gguf":
		say(model.Event{Event: model.EventFile, Name: "An embedder", Architecture: "bert", TrainContext: 512,
			Embedding: true})
	default:
		say(model.Event{Event: model.EventError, Code: model.CodeModel, Error: "it is not a GGUF model llama.cpp loads"})
	}
}

func answer(say func(model.Event), commands <-chan model.Command, c model.Command, text string, pace time.Duration) {
	words := strings.SplitAfter(text, " ")
	sent := 0
	for i, word := range words {
		select {
		case other, open := <-commands:
			if !open {
				os.Exit(0)
			}
			if other.Op == model.OpCancel && other.ID == c.ID {
				say(model.Event{Event: model.EventDone, ID: c.ID, Stopped: model.StoppedCancelled, Tokens: sent})
				return
			}
		case <-time.After(pace):
		}
		say(model.Event{Event: model.EventToken, ID: c.ID, Text: word})
		sent++
		if i == 1 && strings.HasPrefix(text, "crash") {
			fmt.Fprintln(os.Stderr, "segmentation fault")
			os.Exit(139)
		}
		if i == 0 && strings.HasPrefix(text, "hang") {
			select {}
		}
	}
	say(model.Event{Event: model.EventDone, ID: c.ID, Stopped: model.StoppedStop, Tokens: sent, PromptTokens: len(words),
		Memory: 2 << 20})
}

// site serves the files a test manifest names, over https as the
// installer wants, and notes what it was asked.
type site struct {
	*httptest.Server

	mu     sync.Mutex
	files  map[string][]byte
	ranges []string
	// cut ends the first answer of a path after that many bytes; gate,
	// when not nil, holds every answer until it is closed.
	cut  map[string]int
	gate chan struct{}
}

func newSite(t *testing.T) *site {
	t.Helper()
	s := &site{files: map[string][]byte{}, cut: map[string]int{}}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

func (s *site) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	data, known := s.files[r.URL.Path]
	cut, gate := s.cut[r.URL.Path], s.gate
	delete(s.cut, r.URL.Path)
	s.ranges = append(s.ranges, r.URL.Path+" "+r.Header.Get("Range"))
	s.mu.Unlock()
	switch {
	case !known:
		http.NotFound(w, r)
	case cut > 0:
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data[:cut])
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		panic(http.ErrAbortHandler)
	default:
		if gate != nil {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			_, _ = w.Write(data[:1])
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			select {
			case <-gate:
			case <-r.Context().Done():
				return
			}
			_, _ = w.Write(data[1:])
			return
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	}
}

func (s *site) asked() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ranges...)
}

func sum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// entry is one file of a tar archive: a link when it has a target.
type entry struct {
	name, body, link string
	kind             byte
}

func tarball(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var packed bytes.Buffer
	zipped := gzip.NewWriter(&packed)
	archive := tar.NewWriter(zipped)
	for _, e := range entries {
		header := &tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.body)), Typeflag: tar.TypeReg}
		switch {
		case e.link != "":
			header.Typeflag, header.Linkname, header.Size = tar.TypeSymlink, e.link, 0
		case strings.HasSuffix(e.name, "/"):
			header.Typeflag = tar.TypeDir
		case e.kind != 0:
			header.Typeflag = e.kind
		}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatal(err)
	}
	return packed.Bytes()
}

// library is an archive laid out as llama.cpp's own are thought to be:
// its libraries, their links and its backends together, beside the
// programs light leaves in it.
func library(t *testing.T) []byte {
	t.Helper()
	return tarball(t,
		entry{name: "llama-b1/"},
		entry{name: "llama-b1/LICENSE", body: "MIT"},
		entry{name: "llama-b1/llama-server", body: "a program"},
		entry{name: "llama-b1/libllama.0.dylib", body: "llama"},
		entry{name: "llama-b1/libllama.dylib", link: "libllama.0.dylib"},
		entry{name: "llama-b1/libllama.so", body: "llama"},
		entry{name: "llama-b1/llama.dll", body: "llama"},
		entry{name: "llama-b1/libggml.dylib", body: "ggml"},
		entry{name: "llama-b1/libggml-base.dylib", body: "base"},
		entry{name: "llama-b1/libggml-metal.dylib", body: "metal"},
	)
}

const (
	small    = "small"
	big      = "big"
	embeds   = "embeds"
	runtime1 = "1.0.0"
)

// catalogue is a manifest of three small models served by s: one
// proposed to every device, one to those of 16 GB and more, and one
// that embeds.
func catalogue(t *testing.T, s *site) *model.Manifest {
	t.Helper()
	zero, sixteen := 0, 16
	archive := library(t)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files["/llama.tar.gz"] = archive
	m := &model.Manifest{
		Version: 1,
		Runtime: model.Runtime{
			ID: model.RuntimeID, Name: "llama.cpp", Publisher: "ggml-org", Licence: "MIT", Version: runtime1, Tag: "b1",
			Archives: []model.Archive{{
				OS: runtime.GOOS, Arch: runtime.GOARCH, URL: s.URL + "/llama.tar.gz", Size: int64(len(archive)),
				SHA256: sum(archive),
			}},
		},
	}
	for _, d := range []model.Download{
		{ID: small, Name: "Small", Kind: model.KindChat, Context: 2048, MinMemory: 4, RecommendedFrom: &zero, Tried: true},
		{ID: big, Name: "Big", Kind: model.KindChat, Context: 4096, MinMemory: 8, RecommendedFrom: &sixteen},
		{ID: embeds, Name: "Embeds", Kind: model.KindEmbedding, Context: 512, QueryPrefix: "query: ",
			DocumentPrefix: "passage: "},
	} {
		body := bytes.Repeat([]byte(d.ID+" weights "), 4000)
		d.File, d.URL, d.Size, d.SHA256 = d.ID+".gguf", s.URL+"/"+d.ID+".gguf", int64(len(body)), sum(body)
		d.Publisher, d.Licence = "A publisher", "Apache-2.0"
		s.files["/"+d.ID+".gguf"] = body
		m.Models = append(m.Models, d)
	}
	return m
}

// bench is a Local on a folder of its own, its fake helper and the site
// its manifest downloads from.
type bench struct {
	*model.Local

	t     *testing.T
	dir   string
	notes string
	site  *site
}

// options are a test's options: every wait short, the helper this test
// program in the given mode.
func options(t *testing.T, mode string) (model.Options, *bench) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b := &bench{t: t, dir: filepath.Join(t.TempDir(), "model"), notes: t.TempDir(), site: newSite(t)}
	return model.Options{
		Dir: b.dir, Helper: exe, Build: "test", Manifest: catalogue(t, b.site),
		Transport: b.site.Client().Transport, Memory: 8 << 30,
		Env:  []string{envMode + "=" + mode, envNotes + "=" + b.notes},
		Idle: time.Minute, Handshake: 5 * time.Second, Start: 5 * time.Second, Silence: 5 * time.Second,
		CancelWait: 5 * time.Second, Grace: 2 * time.Second, Backoff: 20 * time.Millisecond,
		MaxBackoff: 100 * time.Millisecond,
	}, b
}

func (b *bench) open(opts model.Options, s model.Settings) *bench {
	b.Local = model.Open(opts, s)
	b.t.Cleanup(func() {
		_ = b.Close()
		for _, n := range b.helpers() {
			eventually(b.t, "the helper "+strconv.Itoa(n.PID)+" is gone", func() bool { return !alive(n.PID) })
		}
	})
	return b
}

// started is a Local that is on, with the library and the small model
// installed, on a fake helper in that mode.
func started(t *testing.T, mode string, change ...func(*model.Options)) *bench {
	t.Helper()
	opts, b := options(t, mode)
	for _, c := range change {
		c(&opts)
	}
	b.open(opts, model.Settings{On: true, GPU: true})
	b.install(small)
	return b
}

func (b *bench) install(id string) {
	b.t.Helper()
	if err := b.Install(id); err != nil {
		b.t.Fatal(err)
	}
	eventually(b.t, id+" is installed", func() bool { return b.entry(id).Install.State == model.Installed })
}

func (b *bench) entry(id string) model.Entry {
	b.t.Helper()
	for _, e := range b.Catalogue() {
		if e.ID == id {
			return e
		}
	}
	b.t.Fatalf("the catalogue has no %s", id)
	return model.Entry{}
}

// helpers is what every fake helper started so far left, oldest first.
func (b *bench) helpers() []note {
	names, _ := filepath.Glob(filepath.Join(b.notes, "*.json"))
	var notes []note
	for _, name := range names {
		var n note
		if data, err := os.ReadFile(name); err == nil && json.Unmarshal(data, &n) == nil {
			notes = append(notes, n)
		}
	}
	slices.SortFunc(notes, func(a, b note) int { return cmp.Compare(a.At, b.At) })
	return notes
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("it never happened that %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func say(text string) model.Request {
	return model.Request{Messages: []model.Message{{Role: model.User, Text: text}}}
}
