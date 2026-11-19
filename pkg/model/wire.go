package model

// This file is the protocol between light and its helper, light-model.
// The helper is a module of its own and holds a copy of it
// (helper/model/internal/wire/wire.go), which a test keeps the same but
// for its first line.
//
// Each side writes one JSON value a line: light on the helper's standard
// input, the helper on its standard output. The first line light writes
// is a "hello" naming the protocol, the files and the settings; the
// helper answers "hello" with its own protocol before it loads anything,
// then says what it is doing ("status") until "ready", or "error" and
// leaves. After that light sends one request at a time ("generate",
// "embed", "count"), each with an id of its own, and may "cancel" it;
// the helper answers "token" as an answer comes, "alive" while it
// computes without one, and ends each request with "done", "vectors",
// "count" or "error". A hello that names Inspect is answered by "file"
// and nothing else. The helper leaves when its standard input closes.

// Protocol is the version both sides must speak.
const Protocol = 1

// The operations light asks.
const (
	OpHello    = "hello"
	OpGenerate = "generate"
	OpEmbed    = "embed"
	OpCount    = "count"
	OpCancel   = "cancel"
)

// The events the helper answers.
const (
	EventHello   = "hello"
	EventStatus  = "status"
	EventReady   = "ready"
	EventFile    = "file"
	EventToken   = "token"
	EventAlive   = "alive"
	EventDone    = "done"
	EventVectors = "vectors"
	EventCount   = "count"
	EventError   = "error"
)

// What an error is about.
const (
	CodeProtocol = "protocol"
	CodeVersion  = "version"
	CodeLibrary  = "library"
	CodeModel    = "model"
	CodeTemplate = "template"
	CodeTooLong  = "too-long"
	CodeRequest  = "request"
	CodeDev      = "dev"
)

// Why an answer ended.
const (
	StoppedStop      = "stop"
	StoppedLength    = "length"
	StoppedCancelled = "cancelled"
)

// Role is who said a message.
type Role string

const (
	User      Role = "user"
	Assistant Role = "assistant"
)

// Message is one turn of a conversation.
type Message struct {
	Role Role   `json:"role"`
	Text string `json:"text"`
}

// Command is one line light writes.
type Command struct {
	Op string `json:"op"`
	ID int    `json:"id,omitempty"`

	// hello
	Protocol int    `json:"protocol,omitempty"`
	Build    string `json:"build,omitempty"`
	// Llama is the version the library must say it is.
	Llama string `json:"llama,omitempty"`
	// Library and Backends are the folders of the llama.cpp libraries
	// and of its ggml backends; Model and Embedding the GGUF files.
	Library   string `json:"library,omitempty"`
	Backends  string `json:"backends,omitempty"`
	Model     string `json:"model,omitempty"`
	Embedding string `json:"embedding,omitempty"`
	// Inspect is a GGUF file to describe in place of serving.
	Inspect string `json:"inspect,omitempty"`
	GPU     bool   `json:"gpu,omitempty"`
	Threads int    `json:"threads,omitempty"`
	Context int    `json:"context,omitempty"`
	// Dev says the files are a developer's own, not light's downloads.
	Dev bool `json:"dev,omitempty"`

	// generate
	System      string    `json:"system,omitempty"`
	Messages    []Message `json:"messages,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Seed        uint32    `json:"seed,omitempty"`
	Stop        []string  `json:"stop,omitempty"`
	Grammar     string    `json:"grammar,omitempty"`

	// embed, count
	Texts []string `json:"texts,omitempty"`
	Text  string   `json:"text,omitempty"`
}

// Event is one line the helper writes.
type Event struct {
	Event string `json:"event"`
	ID    int    `json:"id,omitempty"`

	// hello
	Protocol int    `json:"protocol,omitempty"`
	Build    string `json:"build,omitempty"`

	// status, token, error
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`

	// ready
	Llama        string   `json:"llama,omitempty"`
	Context      int      `json:"context,omitempty"`
	TrainContext int      `json:"train_context,omitempty"`
	Chat         bool     `json:"chat,omitempty"`
	Embeddings   bool     `json:"embeddings,omitempty"`
	Threads      int      `json:"threads,omitempty"`
	Devices      []string `json:"devices,omitempty"`
	// Memory is the helper's resident memory, in bytes.
	Memory int64 `json:"memory,omitempty"`
	// LoadMS is how long loading took.
	LoadMS float64 `json:"load_ms,omitempty"`

	// file
	Name         string `json:"name,omitempty"`
	Architecture string `json:"architecture,omitempty"`
	Parameters   int64  `json:"parameters,omitempty"`
	Template     bool   `json:"template,omitempty"`
	Embedding    bool   `json:"embedding,omitempty"`

	// done
	Stopped      string  `json:"stopped,omitempty"`
	PromptTokens int     `json:"prompt_tokens,omitempty"`
	Tokens       int     `json:"tokens,omitempty"`
	FirstMS      float64 `json:"first_ms,omitempty"`
	TotalMS      float64 `json:"total_ms,omitempty"`

	// vectors, count
	Vectors    [][]float32 `json:"vectors,omitempty"`
	Dimensions int         `json:"dimensions,omitempty"`
	Count      int         `json:"count,omitempty"`
}
