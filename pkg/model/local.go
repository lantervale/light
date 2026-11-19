package model

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"
)

// Options are what a Local is made with. Only Dir and Helper are needed:
// the durations are for tests.
type Options struct {
	// Dir is the model folder of the profile (<profile>/model). With
	// none, a light that keeps nothing on disk, there is no model: it
	// stays Off whatever the settings say.
	Dir string
	// Helper is the path of light-model (FindHelper), "" for none.
	Helper string
	// Env is more of the helper's environment, which is otherwise only
	// a home of its own in Dir.
	Env []string
	// Build is this light's version, and Release whether it is a
	// released one: a released light takes no Dev, and only a helper of
	// its own version.
	Build   string
	Release bool
	// Dev is a developer's own llama.cpp and model, used in place of
	// what light downloads.
	Dev Dev
	// Manifest is what may be downloaded, nil for light's own (Pinned).
	Manifest *Manifest
	// Transport makes the downloads' requests, nil for the system's.
	Transport http.RoundTripper
	// Memory is this machine's, in bytes, 0 to ask the system.
	Memory int64

	// Idle is how long the helper stays without a request before it is
	// stopped to free its memory (5 minutes).
	Idle time.Duration
	// Handshake is how long the helper has to say hello (10 seconds),
	// Start to load the model (5 minutes: a first start compiles the
	// graphics card's programs), Silence how long it may compute
	// without a sign of life (1 minute), CancelWait how long it has to
	// stop a request it was told to (10 seconds), Grace to leave once
	// its input closed (2 seconds). Past any of them it is killed.
	Handshake, Start, Silence, CancelWait, Grace time.Duration
	// Backoff is the wait before a helper that crashed is started
	// again, doubled at each crash in a row up to MaxBackoff (1 second,
	// 1 minute).
	Backoff, MaxBackoff time.Duration
}

func (o Options) withDefaults() Options {
	for _, d := range []struct {
		value *time.Duration
		by    time.Duration
	}{
		{&o.Idle, 5 * time.Minute}, {&o.Handshake, 10 * time.Second}, {&o.Start, 5 * time.Minute},
		{&o.Silence, time.Minute}, {&o.CancelWait, 10 * time.Second}, {&o.Grace, 2 * time.Second},
		{&o.Backoff, time.Second}, {&o.MaxBackoff, time.Minute},
	} {
		if *d.value <= 0 {
			*d.value = d.by
		}
	}
	if o.Memory == 0 {
		o.Memory = Memory()
	}
	if o.Release {
		o.Dev = Dev{}
	}
	return o
}

// The helper's phases.
const (
	phaseNone = iota
	phaseHandshake
	phaseLoading
	phaseReady
)

// job is one request that waits or runs.
type job struct {
	priority Priority
	command  Command
	// stream ends the job, whatever it asks: a count and vectors are
	// read once it is done.
	stream  *Stream
	vectors [][]float32
	count   int
	epoch   int
	unhook  func() bool
}

// target is the files a helper is started on, and what they can do.
type target struct {
	library, backends, model, embedding string
	chosen                              Chosen
	caps                                Capabilities
	dev                                 bool
	queryPrefix, documentPrefix         string
}

// Local is the model run on this device by light-model, a helper
// program started at the first request, stopped after a while without
// one, started again after a crash, and killed when it stops answering.
// It is a Model and a Library.
type Local struct {
	opts     Options
	manifest Manifest
	files    files
	install  *installer

	wake    chan struct{}
	stopped chan struct{}

	mu          sync.Mutex
	settings    Settings
	seq         uint64
	state       State
	reason      string
	detail      string
	loaded      bool
	memory      int64
	chosen      Chosen
	caps        Capabilities
	prefixes    [2]string
	unusable    error
	broken      error
	interactive []*job
	background  []*job
	warm        bool
	epoch       int
	acks        []chan struct{}
	closed      bool
	ids         int
	// live is the model and the embedding model the helper was started
	// on.
	live [2]string

	// The rest is the loop's alone.
	proc       *process
	phase      int
	procEpoch  int
	job        *job
	cancelling bool
	requeue    bool
	deadline   time.Time
	failures   int
	retryAt    time.Time
}

var (
	_ Model   = (*Local)(nil)
	_ Library = (*Local)(nil)
)

// Open is the model of a profile, following s. Nothing runs and nothing
// is downloaded until s.On and a request, or an Install.
func Open(opts Options, s Settings) *Local {
	opts = opts.withDefaults()
	l := &Local{opts: opts, files: files{dir: opts.Dir}, wake: make(chan struct{}, 1), stopped: make(chan struct{})}
	if opts.Manifest != nil {
		l.manifest = *opts.Manifest
	} else {
		l.manifest = Pinned()
	}
	l.install = newInstaller(l.files, l.manifest, opts.Transport, l.bump, l.settled)
	if opts.Dir == "" {
		l.reason = "this light keeps nothing on disk, and so has no model"
		close(l.stopped)
		return l
	}
	l.settings = s
	l.refresh()
	go l.loop()
	return l
}

func (l *Local) poke() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (l *Local) bump() {
	l.mu.Lock()
	l.seq++
	l.mu.Unlock()
}

// settled is an install that ended: what is missing may be there now.
func (l *Local) settled() {
	l.mu.Lock()
	l.refresh()
	l.mu.Unlock()
	l.poke()
}

// set changes the status under the lock.
func (l *Local) set(change func()) {
	l.mu.Lock()
	change()
	l.seq++
	l.mu.Unlock()
}

// refresh works out, with the lock held or before the loop runs, what
// the settings and the files on disk make of the model while no helper
// runs for it.
func (l *Local) refresh() {
	l.seq++
	if l.opts.Dir == "" || !l.settings.On {
		l.state, l.reason, l.unusable = Off, "", nil
		if l.opts.Dir == "" {
			l.reason = "this light keeps nothing on disk, and so has no model"
		}
		return
	}
	t, err := l.target(l.settings, false)
	l.chosen, l.prefixes = t.chosen, [2]string{t.documentPrefix, t.queryPrefix}
	if !l.loaded {
		l.caps = t.caps
	}
	l.caps.Embeddings = t.caps.Embeddings
	if err == nil && l.live != ([2]string{}) && l.live != [2]string{t.model, t.embedding} {
		// What the helper runs on is no longer what is installed.
		l.epoch++
		l.live = [2]string{}
	}
	wasUnusable := l.unusable != nil
	l.unusable = err
	switch {
	case errors.Is(err, ErrMissing):
		l.state, l.reason = Missing, err.Error()
	case err != nil:
		l.state, l.reason = Failed, err.Error()
	case l.broken != nil:
		l.state, l.reason = Failed, l.broken.Error()
	case l.state == Off || l.state == Missing || wasUnusable:
		l.state, l.reason = Ready, ""
	}
}

// target is what the settings s name on this device. Its error says
// what is missing (ErrMissing) or wrong; the choice it names is given
// even then. verify reads a file of the user's whole when it seems to
// have changed.
func (l *Local) target(s Settings, verify bool) (target, error) {
	var t target
	if dev := l.opts.Dev; dev.Model != "" {
		t = target{
			library: dev.Library, backends: dev.Backends, model: dev.Model, embedding: dev.Embedding, dev: true,
			chosen: Chosen{File: dev.Model, Name: "a developer's " + filepath.Base(dev.Model)},
			caps:   Capabilities{Context: DefaultContext, Chat: true, Embeddings: dev.Embedding != ""},
		}
		return t, nil
	}
	var err error
	switch {
	case s.File != "":
		err = l.ownTarget(&t, s.File, verify)
	default:
		err = l.listedTarget(&t, s.ID)
	}
	var found bool
	if t.library, t.backends, found = l.files.runtimePaths(l.manifest.Runtime); !found && err == nil {
		err = fmt.Errorf("%w: the library that runs it (llama.cpp) is not on this device", ErrMissing)
	}
	if e, has := l.manifest.Embedding(); has {
		if t.embedding, found = l.files.modelPath(e); found {
			t.caps.Embeddings = true
			t.queryPrefix, t.documentPrefix = e.QueryPrefix, e.DocumentPrefix
		}
	}
	return t, err
}

func (l *Local) listedTarget(t *target, id string) error {
	if id == "" {
		id = l.manifest.Recommended(l.opts.Memory).ID
	}
	d, known := l.manifest.Model(id)
	if !known || d.Kind != KindChat {
		return fmt.Errorf("light knows no model %q to chat with", id)
	}
	t.chosen, t.caps = Chosen{ID: d.ID, Name: d.Name}, Capabilities{Context: d.Context, Chat: true}
	var found bool
	if t.model, found = l.files.modelPath(d); !found {
		return fmt.Errorf("%w: %s is not on this device", ErrMissing, d.Name)
	}
	return nil
}

func (l *Local) ownTarget(t *target, path string, verify bool) error {
	t.chosen = Chosen{File: path, Name: filepath.Base(path)}
	o, known := l.files.ownFiles()[path]
	if !known {
		return fmt.Errorf("%s was not looked into yet: choose it again to use it", path)
	}
	if o.Name != "" {
		t.chosen.Name = o.Name
	}
	t.model, t.caps = path, Capabilities{Context: min(o.Context, DefaultContext), Chat: true}
	if verify {
		return l.files.unchanged(o)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%w: %s is no longer there", ErrMissing, path)
	}
	return nil
}

// Status is the model now.
func (l *Local) Status() Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Status{
		Seq: l.seq, State: l.state, Reason: l.reason, Detail: l.detail, Loaded: l.loaded, Chosen: l.chosen,
		Capabilities: l.caps, Memory: l.memory, Waiting: len(l.interactive) + len(l.background),
	}
}

// queue adds a job to those that wait, or says why the model takes
// none.
func (l *Local) queue(ctx context.Context, priority Priority, command Command, held bool) (*job, error) {
	j := &job{priority: priority, command: command, stream: newStream(held)}
	j.unhook = context.AfterFunc(ctx, func() {
		if j.stream.finish(Result{}, ctx.Err()) {
			l.poke()
		}
	})
	l.mu.Lock()
	err := l.refusal(command.Op)
	if err == nil {
		l.ids++
		j.command.ID, j.epoch = l.ids, l.epoch
		if priority == Background {
			l.background = append(l.background, j)
		} else {
			l.interactive = append(l.interactive, j)
		}
		l.seq++
	}
	l.mu.Unlock()
	if err != nil {
		j.unhook()
		return nil, err
	}
	l.poke()
	return j, nil
}

// refusal is why no request of that kind is taken now, with the lock
// held.
func (l *Local) refusal(op string) error {
	switch {
	case l.closed:
		return ErrClosed
	case l.opts.Dir == "" || !l.settings.On:
		return ErrOff
	case l.unusable != nil:
		return l.unusable
	case l.broken != nil:
		return l.broken
	case op == OpEmbed && !l.caps.Embeddings:
		return ErrNoEmbeddings
	case op == OpGenerate && !l.caps.Chat:
		return ErrNoChat
	}
	return nil
}

// Generate queues req and returns the stream of its answer.
func (l *Local) Generate(ctx context.Context, req Request) (*Stream, error) {
	if len(req.Messages) == 0 {
		return nil, errors.New("a request needs a message")
	}
	tokens := req.MaxTokens
	if tokens <= 0 {
		tokens = DefaultTokens
	}
	j, err := l.queue(ctx, req.Priority, Command{
		Op: OpGenerate, System: req.System, Messages: req.Messages, MaxTokens: tokens, Temperature: req.Temperature,
		Seed: req.Seed, Stop: req.Stop, Grammar: req.Grammar,
	}, req.Priority == Background)
	if err != nil {
		return nil, err
	}
	return j.stream, nil
}

// Embed returns one vector a text.
func (l *Local) Embed(ctx context.Context, texts Texts) ([][]float32, error) {
	if len(texts.Texts) == 0 {
		return nil, nil
	}
	l.mu.Lock()
	prefix := l.prefixes[min(max(int(texts.Kind), 0), 1)]
	l.mu.Unlock()
	prefixed := make([]string, len(texts.Texts))
	for i, text := range texts.Texts {
		prefixed[i] = prefix + text
	}
	j, err := l.queue(ctx, texts.Priority, Command{Op: OpEmbed, Texts: prefixed}, true)
	if err != nil {
		return nil, err
	}
	if _, err = j.stream.Wait(); err != nil {
		return nil, err
	}
	return j.vectors, nil
}

// Count is how many tokens text is for the chosen model.
func (l *Local) Count(ctx context.Context, text string) (int, error) {
	j, err := l.queue(ctx, Interactive, Command{Op: OpCount, Text: text}, true)
	if err != nil {
		return 0, err
	}
	if _, err = j.stream.Wait(); err != nil {
		return 0, err
	}
	return j.count, nil
}

// Warm loads the model ahead of a request.
func (l *Local) Warm() {
	l.mu.Lock()
	usable := l.refusal("") == nil
	l.warm = l.warm || usable
	l.mu.Unlock()
	if usable {
		l.poke()
	}
}

// Configure applies the settings. The helper is stopped when they
// changed, and started on them at the next request.
func (l *Local) Configure(s Settings) {
	l.mu.Lock()
	if l.opts.Dir == "" || l.closed || s == l.settings {
		l.mu.Unlock()
		return
	}
	l.settings, l.broken = s, nil
	l.epoch++
	l.refresh()
	l.mu.Unlock()
	l.poke()
}

// quiesce stops the helper and ends what waits, and returns once the
// loop did: the model's files can then be removed.
func (l *Local) quiesce() {
	l.mu.Lock()
	if l.opts.Dir == "" || l.closed {
		l.mu.Unlock()
		return
	}
	ack := make(chan struct{})
	l.acks = append(l.acks, ack)
	l.epoch++
	l.mu.Unlock()
	l.poke()
	select {
	case <-ack:
	case <-l.stopped:
	}
}

// Catalogue is the library and every model of the manifest.
func (l *Local) Catalogue() []Entry {
	var entries []Entry
	if archive, ok := l.manifest.Archive(runtime.GOOS, runtime.GOARCH); ok {
		entries = append(entries, l.manifest.Runtime.entry(archive))
	}
	recommended := l.manifest.Recommended(l.opts.Memory).ID
	for _, d := range l.manifest.Models {
		entry := d.entry()
		entry.Recommended = d.ID == recommended
		entries = append(entries, entry)
	}
	for i := range entries {
		if l.opts.Dir != "" {
			entries[i].Install = l.install.state(entries[i].ID)
		}
	}
	return entries
}

// Recommended is the chat model proposed to this device.
func (l *Local) Recommended() (Entry, bool) {
	entries := l.Catalogue()
	i := slices.IndexFunc(entries, func(e Entry) bool { return e.Recommended })
	if i < 0 {
		return Entry{}, false
	}
	return entries[i], true
}

func (l *Local) on() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.closed:
		return ErrClosed
	case l.opts.Dir == "" || !l.settings.On:
		return ErrOff
	}
	return nil
}

// Install downloads an entry, and the library when it is not there. It
// is refused while the model is off.
func (l *Local) Install(id string) error {
	if err := l.on(); err != nil {
		return err
	}
	err := l.install.install(id)
	l.bump()
	return err
}

// CancelInstall stops a download.
func (l *Local) CancelInstall(id string) { l.install.cancel(id) }

// Delete removes one installed entry, the helper stopped first.
func (l *Local) Delete(id string) error {
	l.quiesce()
	err := l.install.remove(id)
	l.settled()
	return err
}

// DeleteAll removes the model folder: the library, every model, what
// was being downloaded, the notes on the user's files, the helper's
// home and its log.
func (l *Local) DeleteAll() error {
	if l.opts.Dir == "" {
		return nil
	}
	l.quiesce()
	l.install.stop()
	err := os.RemoveAll(l.opts.Dir)
	l.install.forget()
	l.settled()
	return err
}

// Inspect reads a GGUF file of the user's through a helper of its own.
func (l *Local) Inspect(ctx context.Context, path string) (File, error) {
	if err := l.on(); err != nil {
		return File{}, err
	}
	if !filepath.IsAbs(path) || inside(path, l.opts.Dir) {
		return File{}, fmt.Errorf("%s is not a file of your own: give its whole path, outside light's folder", path)
	}
	hello := Command{Op: OpHello, Protocol: Protocol, Build: l.opts.Build, Llama: l.manifest.Runtime.Version,
		Inspect: path}
	if dev := l.opts.Dev; dev.Library != "" {
		hello.Library, hello.Backends, hello.Dev = dev.Library, dev.Backends, true
	} else {
		var found bool
		if hello.Library, hello.Backends, found = l.files.runtimePaths(l.manifest.Runtime); !found {
			return File{}, fmt.Errorf("%w: the library that reads it (llama.cpp) is not on this device", ErrMissing)
		}
	}
	if l.opts.Helper == "" {
		return File{}, errNoHelper
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return File{}, fmt.Errorf("%s is not a file that can be read", path)
	}
	sum, size, err := digest(path)
	if err != nil {
		return File{}, fmt.Errorf("%s cannot be read: %w", path, err)
	}
	ctx, cancel := context.WithTimeout(ctx, l.opts.Start)
	defer cancel()
	ev, err := inspect(ctx, l.opts.Helper, l.files.cache(), l.files.log(), l.opts.Env, hello)
	if err != nil {
		return File{}, fmt.Errorf("%s was refused: %w", filepath.Base(path), err)
	}
	file := File{
		Path: path, Size: size, SHA256: sum, Name: ev.Name, Architecture: ev.Architecture, Parameters: ev.Parameters,
		Context: ev.TrainContext, Template: ev.Template, Embedding: ev.Embedding,
	}
	switch {
	case file.Embedding:
		return file, fmt.Errorf("%s was refused: it is a model that embeds texts, not one that chats", filepath.Base(path))
	case !file.Template:
		return file, fmt.Errorf("%s was refused: it carries no chat template, which answering needs", filepath.Base(path))
	}
	if err = l.files.keepOwn(owned{File: file, Modified: info.ModTime()}); err != nil {
		return file, err
	}
	l.mu.Lock()
	if l.broken != nil {
		// Whatever was wrong with the file chosen, it is this one now.
		l.broken, l.state = nil, Off
	}
	l.refresh()
	l.mu.Unlock()
	return file, nil
}

var errNoHelper = errors.New("light-model, the program that runs the model, is not beside light")

func inside(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && filepath.IsLocal(rel)
}

// Close stops the helper and the downloads, and ends every request.
func (l *Local) Close() error {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	l.poke()
	<-l.stopped
	l.install.close()
	return nil
}

// loop is the one goroutine that owns the helper: it starts it, hands
// it one job at a time, and watches the clock.
func (l *Local) loop() {
	defer close(l.stopped)
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		if !l.step() {
			return
		}
		var events <-chan Event
		if l.proc != nil {
			events = l.proc.events
		}
		var expired <-chan time.Time
		if !l.deadline.IsZero() {
			timer.Reset(max(time.Until(l.deadline), 0))
			expired = timer.C
		}
		select {
		case <-l.wake:
		case ev, open := <-events:
			l.heard(ev, open)
		case <-expired:
			l.expired()
		}
		timer.Stop()
	}
}

// step does what can be done now. It reports false once closed.
func (l *Local) step() bool {
	l.mu.Lock()
	closed, epoch := l.closed, l.epoch
	keep := func(j *job) bool { return !j.stream.over() }
	l.interactive = slices.DeleteFunc(l.interactive, func(j *job) bool { return !keep(j) })
	l.background = slices.DeleteFunc(l.background, func(j *job) bool { return !keep(j) })
	l.mu.Unlock()
	if closed {
		l.halt(ErrClosed, epoch+1)
		return false
	}
	if epoch != l.procEpoch {
		l.halt(ErrChanged, epoch)
		l.procEpoch, l.failures, l.retryAt = epoch, 0, time.Time{}
		l.mu.Lock()
		for _, ack := range l.acks {
			close(ack)
		}
		l.acks = nil
		l.mu.Unlock()
	}
	if l.job != nil {
		l.supervise()
	} else {
		l.dispatch()
	}
	return true
}

// halt stops the helper and ends, with err, the job that runs and
// those queued before epoch.
func (l *Local) halt(err error, epoch int) {
	if l.proc != nil {
		l.proc.stop(l.opts.Grace)
		l.proc, l.phase = nil, phaseNone
	}
	l.deadline = time.Time{}
	if l.job != nil {
		l.end(Result{}, err)
	}
	l.mu.Lock()
	var ended []*job
	old := func(j *job) bool {
		if j.epoch < epoch {
			ended = append(ended, j)
			return true
		}
		return false
	}
	l.interactive = slices.DeleteFunc(l.interactive, old)
	l.background = slices.DeleteFunc(l.background, old)
	l.loaded, l.memory, l.detail, l.warm, l.live = false, 0, "", false, [2]string{}
	l.state = Off
	l.refresh()
	l.mu.Unlock()
	for _, j := range ended {
		j.stream.finish(Result{}, err)
		j.unhook()
	}
}

// end ends the job that runs.
func (l *Local) end(result Result, err error) {
	l.job.stream.finish(result, err)
	l.job.unhook()
	l.job, l.cancelling, l.requeue, l.deadline = nil, false, false, time.Time{}
}

// supervise stops the job that runs when nobody waits for it any more,
// or when somebody waits behind a background one.
func (l *Local) supervise() {
	if l.cancelling {
		return
	}
	over := l.job.stream.over()
	l.mu.Lock()
	preempt := !over && l.job.priority == Background && len(l.interactive) > 0
	l.mu.Unlock()
	if !over && !preempt {
		return
	}
	_ = l.proc.send(Command{Op: OpCancel, ID: l.job.command.ID})
	l.cancelling, l.requeue = true, preempt
	l.deadline = time.Now().Add(l.opts.CancelWait)
}

// dispatch hands the helper the next job, starting it when it does not
// run, and counts down to its stop when there is none.
func (l *Local) dispatch() {
	l.mu.Lock()
	var next *job
	switch {
	case len(l.interactive) > 0:
		next = l.interactive[0]
	case len(l.background) > 0:
		next = l.background[0]
	}
	warm := l.warm
	l.mu.Unlock()
	switch {
	case next == nil && !warm:
		if l.proc != nil && l.phase == phaseReady && l.deadline.IsZero() {
			l.deadline = time.Now().Add(l.opts.Idle)
		}
	case l.proc == nil && time.Now().Before(l.retryAt):
		l.deadline = l.retryAt
	case l.proc == nil:
		l.launch()
	case l.phase != phaseReady:
	case next == nil:
		l.set(func() { l.warm = false })
		l.deadline = time.Now().Add(l.opts.Idle)
	default:
		l.run(next)
	}
}

func (l *Local) run(next *job) {
	l.mu.Lock()
	l.interactive = slices.DeleteFunc(l.interactive, func(j *job) bool { return j == next })
	l.background = slices.DeleteFunc(l.background, func(j *job) bool { return j == next })
	l.state, l.warm = Working, false
	l.seq++
	l.mu.Unlock()
	l.job, l.deadline = next, time.Now().Add(l.opts.Silence)
	if err := l.proc.send(next.command); err != nil {
		l.crashed(err, true)
	}
}

// launch starts the helper on what the settings name.
func (l *Local) launch() {
	l.mu.Lock()
	settings := l.settings
	l.mu.Unlock()
	t, err := l.target(settings, true)
	if err == nil && l.opts.Helper == "" {
		err = errNoHelper
	}
	if err != nil {
		if !errors.Is(err, ErrMissing) {
			l.set(func() { l.broken = err })
		}
		l.halt(err, l.procEpoch+1)
		return
	}
	threads := settings.Threads
	if threads <= 0 {
		threads = Threads()
	}
	l.set(func() {
		l.state, l.reason, l.detail = Preparing, "", "starting"
		l.live = [2]string{t.model, t.embedding}
	})
	p, err := start(l.opts.Helper, l.files.cache(), l.files.log(), l.opts.Env)
	if err != nil {
		l.crashed(err, false)
		return
	}
	l.proc, l.phase, l.deadline = p, phaseHandshake, time.Now().Add(l.opts.Handshake)
	err = p.send(Command{
		Op: OpHello, Protocol: Protocol, Build: l.opts.Build, Llama: l.manifest.Runtime.Version,
		Library: t.library, Backends: t.backends, Model: t.model, Embedding: t.embedding,
		GPU: settings.GPU, Threads: threads, Context: t.caps.Context, Dev: t.dev,
	})
	if err != nil {
		l.crashed(err, true)
	}
}

// crashed is a helper that went away, or is killed for not answering:
// the job it ran ends with the cause, as do those that wait when it
// never got ready, and the next start waits.
func (l *Local) crashed(cause error, kill bool) {
	if l.proc != nil && kill {
		l.proc.kill()
	}
	starting := l.phase != phaseReady
	l.proc, l.phase = nil, phaseNone
	err := fmt.Errorf("%w: %w", ErrHelper, cause)
	if l.job != nil {
		l.end(Result{}, err)
	}
	l.failures++
	wait := min(l.opts.Backoff<<min(l.failures-1, 16), l.opts.MaxBackoff)
	l.retryAt, l.deadline = time.Now().Add(wait), time.Time{}
	l.set(func() { l.state, l.reason, l.detail, l.loaded, l.memory = Failed, err.Error(), "", false, 0 })
	if starting {
		// Those that wait asked for this start: they are told it failed,
		// and the next request tries again.
		l.halt(err, l.procEpoch+1)
		l.retryAt = time.Now().Add(wait)
		l.set(func() { l.state, l.reason = Failed, err.Error() })
	}
}

// refuse is a helper that will not run on these files or with this
// light: nothing is tried again until the settings change.
func (l *Local) refuse(err error) {
	err = fmt.Errorf("%w: %w", ErrHelper, err)
	l.set(func() { l.broken = err })
	l.halt(err, l.procEpoch+1)
	l.set(func() { l.state, l.reason = Failed, err.Error() })
}

// expired is the deadline passed.
func (l *Local) expired() {
	if l.deadline.IsZero() || time.Now().Before(l.deadline) {
		return
	}
	l.deadline = time.Time{}
	switch {
	case l.proc == nil:
	case l.phase != phaseReady:
		l.crashed(errors.New("it did not start in time"), true)
	case l.job != nil:
		l.crashed(errors.New("it stopped answering"), true)
	default:
		l.proc.stop(l.opts.Grace)
		l.proc, l.phase = nil, phaseNone
		l.set(func() { l.loaded, l.memory = false, 0 })
	}
}

// heard is one event of the helper, or its end.
func (l *Local) heard(ev Event, open bool) {
	if !open {
		l.crashed(l.proc.err, false)
		return
	}
	mine := l.job != nil && ev.ID == l.job.command.ID
	switch ev.Event {
	case EventHello:
		switch {
		case ev.Protocol != Protocol:
			l.refuse(protocolError(ev.Protocol))
		case l.opts.Release && ev.Build != l.opts.Build:
			l.refuse(fmt.Errorf("light-model is of light %s and this is light %s", ev.Build, l.opts.Build))
		default:
			l.phase, l.deadline = phaseLoading, time.Now().Add(l.opts.Start)
		}
	case EventStatus:
		l.set(func() { l.detail = ev.Text })
	case EventReady:
		l.phase, l.deadline = phaseReady, time.Time{}
		l.set(func() {
			l.state, l.detail, l.loaded, l.memory = Ready, "", true, ev.Memory
			l.caps.Context, l.caps.Chat = ev.Context, ev.Chat
		})
	case EventError:
		switch {
		case ev.ID == 0:
			l.refuse(errors.New(ev.Error))
		case mine:
			l.finished(Result{}, wireError(ev))
		}
	case EventToken:
		if mine && !l.cancelling {
			l.job.stream.push(ev.Text)
			l.deadline = time.Now().Add(l.opts.Silence)
		}
	case EventAlive:
		if mine && !l.cancelling {
			l.deadline = time.Now().Add(l.opts.Silence)
		}
	case EventDone:
		if mine {
			l.done(ev)
		}
	case EventVectors:
		if mine {
			l.job.vectors = ev.Vectors
			l.set(func() { l.caps.Dimensions = ev.Dimensions })
			l.finished(Result{}, nil)
		}
	case EventCount:
		if mine {
			l.job.count = ev.Count
			l.finished(Result{}, nil)
		}
	}
}

func wireError(ev Event) error {
	if ev.Code == CodeTooLong {
		return fmt.Errorf("%w: %s", ErrTooLong, ev.Error)
	}
	return errors.New(ev.Error)
}

// done is the helper ending the job that runs: one stopped for a
// request somebody waits for goes back first among those of its kind.
func (l *Local) done(ev Event) {
	if l.requeue && ev.Stopped == StoppedCancelled && !l.job.stream.over() {
		again := l.job
		again.stream.restart()
		l.job, l.cancelling, l.requeue, l.deadline = nil, false, false, time.Time{}
		l.set(func() {
			l.background = slices.Insert(l.background, 0, again)
			l.state, l.memory = Ready, ev.Memory
		})
		return
	}
	l.set(func() { l.memory = ev.Memory })
	l.finished(Result{
		Stopped: ev.Stopped, PromptTokens: ev.PromptTokens, Tokens: ev.Tokens, FirstToken: ev.FirstMS, Total: ev.TotalMS,
	}, nil)
}

// finished ends the job that runs: the helper answered, so it works.
func (l *Local) finished(result Result, err error) {
	l.end(result, err)
	l.failures = 0
	l.set(func() { l.state = Ready })
}
