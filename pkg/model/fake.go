package model

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"slices"
	"strings"
	"sync"
)

// Fake is a model for tests: it runs nothing, answers what it is told
// to, and is a Model and a Library. NewFake makes one that is on and
// ready, the manifest's entries all installed.
//
// Its answers come at once unless Manual is set, in which case each
// Step gives the oldest request its next word: a test of a screen draws
// between two.
type Fake struct {
	// Reply is the answer to a request, or the error it ends with; nil
	// answers the request's last message back.
	Reply func(Request) (string, error)
	// Vector is the embedding of a text; nil gives one that is close
	// for texts sharing words.
	Vector func(text string) []float32
	// Tokens counts a text; nil counts a token every four bytes.
	Tokens func(text string) int
	// Files is what Inspect reads, by path; a path it does not hold is
	// refused.
	Files map[string]File
	// Manual holds each answer until Step, and each install until
	// Advance.
	Manual bool

	mu       sync.Mutex
	status   Status
	settings Settings
	entries  []Entry
	asked    []Request
	pending  []*fakeJob
	warmed   int
	closed   bool
}

type fakeJob struct {
	stream *Stream
	words  []string
	tokens int
	err    error
}

var (
	_ Model   = (*Fake)(nil)
	_ Library = (*Fake)(nil)
)

// NewFake is a model that is on and ready.
func NewFake() *Fake {
	m := Pinned()
	f := &Fake{settings: Settings{On: true, GPU: true}}
	recommended := m.Recommended(0)
	if archive, ok := m.Archive("darwin", "arm64"); ok {
		f.entries = append(f.entries, m.Runtime.entry(archive))
	}
	for _, d := range m.Models {
		entry := d.entry()
		entry.Recommended = d.ID == recommended.ID
		f.entries = append(f.entries, entry)
	}
	for i := range f.entries {
		f.entries[i].Install.State = Installed
	}
	f.status = Status{
		Seq: 1, State: Ready, Chosen: Chosen{ID: recommended.ID, Name: recommended.Name},
		Capabilities: Capabilities{Context: recommended.Context, Chat: true, Embeddings: true, Dimensions: fakeDimensions},
	}
	return f
}

// SetState puts the model in a state, with the reason it gives.
func (f *Fake) SetState(state State, reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status.State, f.status.Reason = state, reason
	f.status.Seq++
}

// SetCapabilities says what the model can do.
func (f *Fake) SetCapabilities(c Capabilities) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status.Capabilities = c
	f.status.Seq++
}

// SetInstalled puts an entry on the device or takes it off.
func (f *Fake) SetInstalled(id string, there bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.entry(id); e != nil {
		e.Install = Install{}
		if there {
			e.Install.State = Installed
		}
	}
	f.status.Seq++
}

// Asked is every request Generate was given, oldest first.
func (f *Fake) Asked() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

// Warmed is how many times Warm was called.
func (f *Fake) Warmed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.warmed
}

// Settings are the last ones Configure was given.
func (f *Fake) Settings() Settings {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.settings
}

func (f *Fake) Status() Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	status := f.status
	status.Waiting = max(len(f.pending)-1, 0)
	return status
}

func (f *Fake) refusal() error {
	switch f.status.State {
	case Off:
		return ErrOff
	case Missing:
		return fmt.Errorf("%w: %s", ErrMissing, f.status.Reason)
	case Failed:
		return fmt.Errorf("%w: %s", ErrHelper, f.status.Reason)
	case Preparing, Ready, Working:
	}
	if f.closed {
		return ErrClosed
	}
	return nil
}

func (f *Fake) Generate(ctx context.Context, req Request) (*Stream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.refusal(); err != nil {
		return nil, err
	}
	if len(req.Messages) == 0 {
		return nil, errors.New("a request needs a message")
	}
	f.asked = append(f.asked, req)
	text, err := req.Messages[len(req.Messages)-1].Text, error(nil)
	if f.Reply != nil {
		text, err = f.Reply(req)
	}
	job := &fakeJob{stream: newStream(req.Priority == Background), words: strings.SplitAfter(text, " "), err: err}
	if !f.Manual {
		for f.advance(job) {
		}
		return job.stream, nil
	}
	f.pending = append(f.pending, job)
	f.status.State = Working
	f.status.Seq++
	context.AfterFunc(ctx, func() {
		job.stream.finish(Result{}, ctx.Err())
		f.mu.Lock()
		f.drop(job)
		f.mu.Unlock()
	})
	return job.stream, nil
}

// advance gives job its next word, or its end; it reports whether more
// is to come.
func (f *Fake) advance(job *fakeJob) bool {
	if len(job.words) > 0 && job.err == nil {
		job.stream.push(job.words[0])
		job.words = job.words[1:]
		job.tokens++
		if len(job.words) > 0 {
			return true
		}
	}
	job.stream.finish(Result{Stopped: StoppedStop, Tokens: job.tokens}, job.err)
	return false
}

func (f *Fake) drop(job *fakeJob) {
	f.pending = slices.DeleteFunc(f.pending, func(j *fakeJob) bool { return j == job })
	if len(f.pending) == 0 && f.status.State == Working {
		f.status.State = Ready
	}
	f.status.Seq++
}

// Step gives the oldest request that waits its next word, and reports
// whether one waited.
func (f *Fake) Step() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return false
	}
	if job := f.pending[0]; !f.advance(job) {
		f.drop(job)
	}
	f.status.Seq++
	return true
}

// Finish gives every request that waits the rest of its answer.
func (f *Fake) Finish() {
	for f.Step() {
	}
}

func (f *Fake) Embed(_ context.Context, texts Texts) ([][]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.refusal(); err != nil {
		return nil, err
	}
	if !f.status.Capabilities.Embeddings {
		return nil, ErrNoEmbeddings
	}
	vectors := make([][]float32, len(texts.Texts))
	for i, text := range texts.Texts {
		if f.Vector != nil {
			vectors[i] = f.Vector(text)
		} else {
			vectors[i] = fakeVector(text)
		}
	}
	return vectors, nil
}

const fakeDimensions = 64

// fakeVector is a vector of unit length that texts sharing words share
// directions of.
func fakeVector(text string) []float32 {
	vector := make([]float32, fakeDimensions)
	for word := range strings.FieldsSeq(strings.ToLower(text)) {
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(strings.Trim(word, ".,;:!?\"'()")))
		vector[hash.Sum32()%fakeDimensions]++
	}
	var length float64
	for _, v := range vector {
		length += float64(v) * float64(v)
	}
	if length == 0 {
		vector[0] = 1
		return vector
	}
	for i := range vector {
		vector[i] = float32(float64(vector[i]) / math.Sqrt(length))
	}
	return vector
}

func (f *Fake) Count(_ context.Context, text string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.refusal(); err != nil {
		return 0, err
	}
	if f.Tokens != nil {
		return f.Tokens(text), nil
	}
	return (len(text) + 3) / 4, nil
}

func (f *Fake) Warm() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.warmed++
}

func (f *Fake) Catalogue() []Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.entries)
}

func (f *Fake) Recommended() (Entry, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.entries, func(e Entry) bool { return e.Recommended })
	if i < 0 {
		return Entry{}, false
	}
	return f.entries[i], true
}

func (f *Fake) entry(id string) *Entry {
	i := slices.IndexFunc(f.entries, func(e Entry) bool { return e.ID == id })
	if i < 0 {
		return nil
	}
	return &f.entries[i]
}

// Install installs an entry and the library: at once, or as far as
// Advance says when Manual.
func (f *Fake) Install(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.settings.On {
		return ErrOff
	}
	e := f.entry(id)
	if e == nil {
		return fmt.Errorf("light knows no model %q", id)
	}
	f.status.Seq++
	if f.Manual {
		if e.Install.State != Installed {
			e.Install = Install{State: Installing, Total: e.Size}
		}
		return nil
	}
	e.Install = Install{State: Installed}
	if runtime := f.entry(RuntimeID); runtime != nil {
		runtime.Install = Install{State: Installed}
	}
	return nil
}

// Advance moves the install of an entry to done bytes: all of them
// install it, and err, when not nil, fails it.
func (f *Fake) Advance(id string, done int64, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.entry(id)
	if e == nil || e.Install.State != Installing {
		return
	}
	f.status.Seq++
	switch {
	case err != nil:
		e.Install = Install{State: InstallFailed, Err: err.Error()}
	case done >= e.Size:
		e.Install = Install{State: Installed}
	default:
		e.Install.Done = done
	}
}

func (f *Fake) CancelInstall(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.entry(id); e != nil && e.Install.State == Installing {
		e.Install = Install{}
		f.status.Seq++
	}
}

func (f *Fake) Delete(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.entry(id)
	if e == nil {
		return fmt.Errorf("light knows no model %q", id)
	}
	e.Install = Install{}
	if f.status.Chosen.ID == id || id == RuntimeID {
		f.status.State, f.status.Reason = Missing, e.Name+" is not on this device"
	}
	f.status.Seq++
	return nil
}

func (f *Fake) DeleteAll() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.entries {
		f.entries[i].Install = Install{}
	}
	if f.settings.On {
		f.status.State, f.status.Reason = Missing, "nothing is on this device"
	}
	f.status.Seq++
	return nil
}

func (f *Fake) Inspect(_ context.Context, path string) (File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.settings.On {
		return File{}, ErrOff
	}
	file, known := f.Files[path]
	switch {
	case !known:
		return File{}, fmt.Errorf("%s was refused: it is not a model", path)
	case file.Embedding:
		return file, fmt.Errorf("%s was refused: it is a model that embeds texts, not one that chats", path)
	case !file.Template:
		return file, fmt.Errorf("%s was refused: it carries no chat template, which answering needs", path)
	}
	file.Path = path
	return file, nil
}

// Configure follows the settings as a Local does: off, the entry or
// the file chosen, missing when it is not installed. Requests that
// wait end with ErrChanged.
func (f *Fake) Configure(s Settings) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s == f.settings {
		return
	}
	f.settings = s
	for _, job := range f.pending {
		job.stream.finish(Result{}, ErrChanged)
	}
	f.pending = nil
	f.status.Seq++
	f.status.State, f.status.Reason = Ready, ""
	switch {
	case !s.On:
		f.status.State = Off
	case s.File != "":
		f.status.Chosen = Chosen{File: s.File, Name: f.Files[s.File].Name}
		if _, known := f.Files[s.File]; !known {
			f.status.State, f.status.Reason = Failed, s.File+" was not looked into yet: choose it again to use it"
		}
	default:
		id := s.ID
		if id == "" {
			if i := slices.IndexFunc(f.entries, func(e Entry) bool { return e.Recommended }); i >= 0 {
				id = f.entries[i].ID
			}
		}
		e := f.entry(id)
		if e == nil {
			f.status.State, f.status.Reason = Failed, fmt.Sprintf("light knows no model %q to chat with", id)
			return
		}
		f.status.Chosen = Chosen{ID: e.ID, Name: e.Name}
		f.status.Capabilities.Context = e.Context
		if e.Install.State != Installed {
			f.status.State, f.status.Reason = Missing, e.Name+" is not on this device"
		}
	}
}

func (f *Fake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	for _, job := range f.pending {
		job.stream.finish(Result{}, ErrClosed)
	}
	f.pending = nil
	return nil
}
