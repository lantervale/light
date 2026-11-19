// Package serve is light-model's side of the protocol it speaks with
// light (wire): it reads light's lines, hands each request to an Engine
// on one goroutine, and writes what comes back. It knows nothing of
// llama.cpp, and reads a request's text as nothing but the model's
// input.
package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lantervale/light/helper/model/internal/wire"
)

// Error is a refusal with what it is about (wire.Code…).
type Error struct {
	Code string
	Text string
}

func (e *Error) Error() string { return e.Text }

// Refuse is an Error of that code.
func Refuse(code, format string, args ...any) error {
	return &Error{Code: code, Text: fmt.Sprintf(format, args...)}
}

// Engine runs the model. Its methods are called from one goroutine,
// locked to its thread, one at a time.
type Engine interface {
	// Load loads the library and the model a hello names, saying what
	// it does through status, and returns the "ready" event.
	Load(hello wire.Command, status func(string)) (wire.Event, error)
	// Inspect describes the GGUF file a hello names: the "file" event.
	Inspect(hello wire.Command) (wire.Event, error)
	// Generate answers a request, each piece through token, and
	// returns the "done" event. It calls alive now and then while it
	// computes without a piece, and stops within a token of ctx's end.
	Generate(ctx context.Context, c wire.Command, token func(string), alive func()) (wire.Event, error)
	// Embed returns one vector of unit length a text.
	Embed(ctx context.Context, texts []string, alive func()) ([][]float32, error)
	// Count is how many tokens text is.
	Count(text string) (int, error)
	Close()
}

// Options are what a helper is built as.
type Options struct {
	// Build is the helper's version, and Release whether it is a
	// released one, which runs only what light installed.
	Build   string
	Release bool
	// Grace is how long the request that runs has to stop once light
	// went away (2 seconds).
	Grace time.Duration
}

// longestLine is the longest line light may write: a page and more.
const longestLine = 64 << 20

type server struct {
	engine Engine
	opts   Options

	out  sync.Mutex
	to   *json.Encoder
	jobs chan wire.Command

	mu        sync.Mutex
	running   int
	cancel    context.CancelFunc
	cancelled map[int]bool
}

func (s *server) say(ev wire.Event) {
	s.out.Lock()
	defer s.out.Unlock()
	_ = s.to.Encode(ev)
}

func (s *server) refuse(id int, err error) {
	ev := wire.Event{Event: wire.EventError, ID: id, Error: err.Error(), Code: wire.CodeRequest}
	var refusal *Error
	if errors.As(err, &refusal) {
		ev.Code = refusal.Code
	}
	s.say(ev)
}

// Run serves light, whose lines come on in, until in ends. The error it
// returns is why the helper would not serve; it was told to light.
func Run(in io.Reader, out io.Writer, engine Engine, opts Options) error {
	if opts.Grace <= 0 {
		opts.Grace = 2 * time.Second
	}
	s := &server{engine: engine, opts: opts, to: json.NewEncoder(out), jobs: make(chan wire.Command, 64),
		cancelled: map[int]bool{}}
	lines := bufio.NewScanner(in)
	lines.Buffer(make([]byte, 64<<10), longestLine)
	var hello wire.Command
	if !lines.Scan() {
		return nil
	}
	if err := json.Unmarshal(lines.Bytes(), &hello); err != nil || hello.Op != wire.OpHello {
		err = Refuse(wire.CodeProtocol, "light-model is started by light, which says hello first")
		s.refuse(0, err)
		return err
	}
	s.say(wire.Event{Event: wire.EventHello, Protocol: wire.Protocol, Build: opts.Build})
	if err := Check(hello, opts.Release); err != nil {
		s.refuse(0, err)
		return err
	}

	// The engine has one goroutine, locked to its thread: what it loads
	// stays where it was loaded.
	failed := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := s.begin(hello); err != nil {
			s.refuse(0, err)
			failed <- err
			return
		}
		if hello.Inspect != "" {
			failed <- nil
			return
		}
		for c := range s.jobs {
			s.work(c)
		}
		s.engine.Close()
	}()
	read := make(chan struct{})
	go func() {
		defer close(read)
		for lines.Scan() {
			s.heard(lines.Bytes())
		}
	}()
	select {
	case err := <-failed:
		return err
	case <-read:
	}
	// Light went away: what runs is stopped, and the helper leaves, a
	// request that would not stop notwithstanding.
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	close(s.jobs)
	select {
	case <-done:
	case <-time.After(opts.Grace):
	}
	return nil
}

func (s *server) begin(hello wire.Command) error {
	if hello.Inspect != "" {
		ev, err := s.engine.Inspect(hello)
		if err != nil {
			return err
		}
		ev.Event = wire.EventFile
		s.say(ev)
		return nil
	}
	ev, err := s.engine.Load(hello, func(text string) { s.say(wire.Event{Event: wire.EventStatus, Text: text}) })
	if err != nil {
		return err
	}
	ev.Event = wire.EventReady
	s.say(ev)
	return nil
}

// heard is one line of light's after its hello.
func (s *server) heard(line []byte) {
	var c wire.Command
	if err := json.Unmarshal(line, &c); err != nil {
		s.refuse(0, Refuse(wire.CodeRequest, "a line of light's does not read: %v", err))
		return
	}
	switch c.Op {
	case wire.OpCancel:
		s.mu.Lock()
		if s.running == c.ID && s.cancel != nil {
			s.cancel()
		} else {
			s.cancelled[c.ID] = true
		}
		s.mu.Unlock()
	case wire.OpGenerate, wire.OpEmbed, wire.OpCount:
		s.jobs <- c
	default:
		s.refuse(c.ID, Refuse(wire.CodeRequest, "light-model knows no %q", c.Op))
	}
}

// work answers one request.
func (s *server) work(c wire.Command) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.mu.Lock()
	if s.cancelled[c.ID] {
		delete(s.cancelled, c.ID)
		s.mu.Unlock()
		s.say(wire.Event{Event: wire.EventDone, ID: c.ID, Stopped: wire.StoppedCancelled})
		return
	}
	s.running, s.cancel = c.ID, cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running, s.cancel = 0, nil
		delete(s.cancelled, c.ID)
		s.mu.Unlock()
	}()

	// alive may be called from the thread llama.cpp computes on.
	var last atomic.Int64
	last.Store(time.Now().UnixNano())
	alive := func() {
		now := time.Now().UnixNano()
		if before := last.Load(); now-before > int64(time.Second) && last.CompareAndSwap(before, now) {
			s.say(wire.Event{Event: wire.EventAlive, ID: c.ID})
		}
	}
	switch c.Op {
	case wire.OpGenerate:
		ev, err := s.engine.Generate(ctx, c, func(text string) {
			last.Store(time.Now().UnixNano())
			s.say(wire.Event{Event: wire.EventToken, ID: c.ID, Text: text})
		}, alive)
		if err != nil {
			s.refuse(c.ID, err)
			return
		}
		ev.Event, ev.ID = wire.EventDone, c.ID
		s.say(ev)
	case wire.OpEmbed:
		vectors, err := s.engine.Embed(ctx, c.Texts, alive)
		switch {
		case ctx.Err() != nil:
			s.say(wire.Event{Event: wire.EventDone, ID: c.ID, Stopped: wire.StoppedCancelled})
		case err != nil:
			s.refuse(c.ID, err)
		default:
			ev := wire.Event{Event: wire.EventVectors, ID: c.ID, Vectors: vectors}
			if len(vectors) > 0 {
				ev.Dimensions = len(vectors[0])
			}
			s.say(ev)
		}
	case wire.OpCount:
		n, err := s.engine.Count(c.Text)
		if err != nil {
			s.refuse(c.ID, err)
			return
		}
		s.say(wire.Event{Event: wire.EventCount, ID: c.ID, Count: n})
	}
}

// Check is why a hello is refused before anything is loaded, nil when
// it is not: another protocol, a file not named by its whole path, and,
// for a released helper, a developer's files or a library of the
// system's (which may be any llama.cpp: only the one light installed in
// its own folder is the one this helper is built for).
func Check(hello wire.Command, release bool) error {
	if hello.Protocol != wire.Protocol {
		return Refuse(wire.CodeProtocol,
			"light speaks protocol %d and light-model protocol %d: they are not of the same release",
			hello.Protocol, wire.Protocol)
	}
	if hello.Llama == "" {
		return Refuse(wire.CodeVersion, "light did not say which llama.cpp it installed")
	}
	paths := map[string]string{"the library's folder": hello.Library, "the backends' folder": hello.Backends}
	if hello.Inspect != "" {
		paths["the file to read"] = hello.Inspect
	} else {
		paths["the model"] = hello.Model
		if hello.Embedding != "" {
			paths["the embedding model"] = hello.Embedding
		}
	}
	for what, path := range paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return Refuse(wire.CodeLibrary, "%s is not named by its whole path: %q", what, path)
		}
	}
	if !release {
		return nil
	}
	if hello.Dev {
		return Refuse(wire.CodeDev, "a released light-model runs only what light installed, not a developer's files")
	}
	for _, path := range []string{hello.Library, hello.Backends} {
		if SystemPath(path) {
			return Refuse(wire.CodeLibrary, "a released light-model loads no llama.cpp of the system's: %s", path)
		}
	}
	return nil
}

// SystemPath reports whether path is in a folder of the system's or of
// a package manager's, where a llama.cpp is whichever was installed
// there.
func SystemPath(path string) bool {
	path = strings.ToLower(strings.ReplaceAll(path, `\`, "/"))
	if len(path) > 1 && path[1] == ':' {
		path = path[2:]
	}
	for _, prefix := range []string{
		"/usr/", "/opt/", "/lib/", "/lib64/", "/bin/", "/sbin/", "/system/", "/library/", "/nix/", "/snap/",
		"/home/linuxbrew/", "/windows/", "/program files/", "/program files (x86)/", "/programdata/",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return strings.Contains(path, "/homebrew/") || strings.Contains(path, "/cellar/") || strings.Contains(path,
		"/.linuxbrew/")
}

// CheckVersion is why a library that says it is version got is refused
// by a helper told to expect want, nil when it is the one: a helper is
// built for one llama.cpp, and another's functions and structures may
// differ in ways nothing else would notice.
func CheckVersion(got, want string) error {
	if got != want {
		return Refuse(wire.CodeVersion, "llama.cpp says it is %q and light installed %q: it is not loaded", got, want)
	}
	return nil
}
