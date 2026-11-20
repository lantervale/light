// Package model is the language model light runs on this device, in
// plain Go: what a surface asks of it (Model), what the settings show
// and change of it (Library), the one that runs it in a helper program
// beside light (Local), and one for tests (Fake).
//
// Nothing is downloaded and no helper is started until the user turned
// the model on: a model that is off answers ErrOff and stays Off.
//
// A surface asks, and reads the answer as it comes:
//
//	stream, err := m.Generate(ctx, model.Request{
//		System:   "You answer in one short paragraph.",
//		Messages: []model.Message{{Role: model.User, Text: question}},
//	})
//	// each frame, without waiting:
//	text, done := stream.Take()
//	// or, on a goroutine of its own:
//	result, err := stream.Wait()
//
// Cancelling ctx ends the stream at once with ctx's error; the helper
// stops within a token. Status never waits either: a frame loop reads
// it each frame and compares its Seq with the one it drew.
package model

import (
	"context"
	"errors"
	"fmt"
)

// State is what the model is doing.
type State int

const (
	// Off is a model the user did not turn on: nothing runs, nothing is
	// downloaded.
	Off State = iota
	// Missing is a model turned on whose files are not on this device:
	// the library that runs it, or the model chosen.
	Missing
	// Preparing is the helper starting and loading the model, which a
	// first start may take half a minute over; Status.Detail says what
	// it is doing.
	Preparing
	// Ready is a model that answers: Status.Loaded says whether it is in
	// memory already or loads at the next request.
	Ready
	// Working is a request being answered.
	Working
	// Failed is a model that cannot run, Status.Reason saying why. One
	// that crashed is started again at the next request, after a wait.
	Failed
)

func (s State) String() string {
	switch s {
	case Off:
		return "off"
	case Missing:
		return "missing"
	case Preparing:
		return "preparing"
	case Ready:
		return "ready"
	case Working:
		return "working"
	case Failed:
		return "failed"
	}
	return fmt.Sprintf("state %d", int(s))
}

// Status is the model at one moment.
type Status struct {
	// Seq grows with every change of the status or of the catalogue: a
	// frame loop draws again when it is not the one it drew.
	Seq   uint64
	State State
	// Reason says why the model is Missing or Failed.
	Reason string
	// Detail is what the helper says it is doing while Preparing.
	Detail string
	// Loaded is whether the model is in memory.
	Loaded bool
	// Chosen is the model the settings name: the id of a catalogue
	// entry, or the path of a file of the user's own.
	Chosen Chosen
	// Capabilities are what the chosen model can do, known before it is
	// loaded: from the catalogue, or from the file's inspection.
	Capabilities Capabilities
	// Memory is what the helper holds, in bytes, as it last said; 0 when
	// it does not run.
	Memory int64
	// Waiting is how many requests wait their turn.
	Waiting int
}

// Chosen names the model in use: a catalogue entry by its ID, or a file
// of the user's by its path. Both empty is none.
type Chosen struct {
	ID   string
	File string
	Name string
}

// Capabilities are what a model can do.
type Capabilities struct {
	// Context is how many tokens a request and its answer hold together.
	Context int
	// Chat is whether Generate answers.
	Chat bool
	// Embeddings is whether Embed answers, and Dimensions the length of
	// its vectors (0 until the embedding model was loaded once).
	Embeddings bool
	Dimensions int
}

// Priority orders the requests that wait.
type Priority int

const (
	// Interactive is a request somebody waits for: it goes before every
	// Background one and stops the one being answered.
	Interactive Priority = iota
	// Background is a request nobody waits for (a page's category). One
	// stopped for an Interactive request starts again afterwards, so
	// its stream gives its text only once whole.
	Background
	// Page is a request a page's script made: it waits behind light's
	// own, and one of those stops it as an Interactive request stops a
	// Background one. Asked at Temperature 0 it starts over the same,
	// so its stream gives its text as it comes, and skips, the second
	// time, what it gave the first.
	Page
	priorities
)

// Request is one thing asked of the model. Its text is input only:
// nothing in light or its helper reads it as an instruction of theirs.
type Request struct {
	// System is what the model is told it is, "" for nothing.
	System   string
	Messages []Message
	// MaxTokens is the most tokens the answer has, 0 for DefaultTokens.
	MaxTokens int
	// Temperature is how freely the next token is chosen: 0 is always
	// the likeliest, which gives the same answer each time.
	Temperature float64
	// Seed picks among the answers a Temperature over 0 allows.
	Seed uint32
	// Stop ends the answer before the first of these texts.
	Stop []string
	// Grammar keeps the answer within a GBNF grammar ("" for free
	// text): Choice makes one of a list of words, JSON is any JSON value.
	Grammar  string
	Priority Priority
}

// DefaultTokens is the most tokens of an answer whose request names none.
const DefaultTokens = 512

// Result is how an answer ended.
type Result struct {
	Text string
	// Stopped is why: "stop" (the model was done, or met a Stop text) or
	// "length" (MaxTokens).
	Stopped string
	// PromptTokens and Tokens count what was read and what was written.
	PromptTokens, Tokens int
	// FirstToken and Total are how long the first token and the whole
	// answer took, in milliseconds, in the helper.
	FirstToken, Total float64
}

// Kind is what a text is embedded for: some models embed a question and
// the documents it is matched against differently.
type Kind int

const (
	Document Kind = iota
	Query
)

// Texts are texts to embed together.
type Texts struct {
	Texts    []string
	Kind     Kind
	Priority Priority
}

var (
	// ErrOff is a request made of a model the user did not turn on.
	ErrOff = errors.New("the model is off")
	// ErrMissing is a request made of a model whose files are not on
	// this device.
	ErrMissing = errors.New("the model is not installed")
	// ErrNoChat is Generate asked of a model that does not chat.
	ErrNoChat = errors.New("this model does not chat")
	// ErrNoEmbeddings is Embed asked while no embedding model is
	// installed.
	ErrNoEmbeddings = errors.New("no embedding model is installed")
	// ErrChanged ends a request the model was switched or turned off
	// under.
	ErrChanged = errors.New("the model was changed while it answered")
	// ErrClosed is a request made of, or cut by, a model that was closed.
	ErrClosed = errors.New("the model is closed")
	// ErrHelper ends a request whose helper went away or stopped
	// answering.
	ErrHelper = errors.New("the model's helper stopped")
	// ErrTooLong is a request that does not fit the model's context.
	ErrTooLong = errors.New("the request does not fit the model's context")
)

// Model is what a surface asks of a language model. It is safe for use
// from several goroutines, and none of its methods but Count and Embed
// waits for the model.
type Model interface {
	// Status is the model now. It never waits.
	Status() Status
	// Generate queues req and returns the stream of its answer. The
	// stream ends with ctx's error as soon as ctx is done.
	Generate(ctx context.Context, req Request) (*Stream, error)
	// Embed returns one vector a text, of unit length.
	Embed(ctx context.Context, texts Texts) ([][]float32, error)
	// Count is how many tokens text is for the chosen model: a request
	// fits while its texts and MaxTokens stay under
	// Status().Capabilities.Context, less a few tokens a message.
	Count(ctx context.Context, text string) (int, error)
	// Warm loads the model ahead of a request (the address field was
	// focused); it does nothing for one that is off or missing.
	Warm()
}

// Library is what the settings show and change of the models: the
// catalogue, what is installed, the one chosen.
type Library interface {
	// Catalogue is every model light knows and the library that runs
	// them, each with whether it is installed and how far its download
	// is. It never waits.
	Catalogue() []Entry
	// Recommended is the chat model proposed when the model is turned
	// on, by this device's memory.
	Recommended() (Entry, bool)
	// Install downloads an entry, and the library when it is not there
	// yet, in the background: Catalogue shows it advance. It is asked
	// only once the user agreed to it.
	Install(id string) error
	// CancelInstall stops a download; what came is kept for the next.
	CancelInstall(id string)
	// Delete removes one installed entry.
	Delete(id string) error
	// DeleteAll removes everything light downloaded or wrote for the
	// model. A file of the user's own is never touched.
	DeleteAll() error
	// Inspect reads a GGUF file of the user's through the helper and
	// notes its digest, so that a change of it is noticed. A file that
	// does not load, or that cannot chat, is an error saying why.
	Inspect(ctx context.Context, path string) (File, error)
	// Configure applies the settings: the model on or off, the one
	// chosen, its threads. A request being answered by another model
	// ends with ErrChanged.
	Configure(s Settings)
	// Close stops the helper and the downloads.
	Close() error
}

// Settings are the settings the model follows (config.Config holds
// them).
type Settings struct {
	// On is whether the user turned the model on.
	On bool
	// ID is the catalogue entry chosen, File the path of a GGUF file of
	// the user's in its place. Both empty is the recommended entry.
	ID   string
	File string
	// Threads is how many threads compute, 0 for light's own choice
	// (Threads).
	Threads int
	// GPU is whether the graphics card computes where it can.
	GPU bool
}

// File is what the helper read in a GGUF file of the user's.
type File struct {
	Path         string
	Size         int64
	SHA256       string
	Name         string
	Architecture string
	// Parameters is how many weights the model has.
	Parameters int64
	// Context is the longest context it was trained for.
	Context int
	// Template is whether it carries a chat template, which Generate
	// needs; Embedding whether it is made to embed and not to chat.
	Template  bool
	Embedding bool
}
