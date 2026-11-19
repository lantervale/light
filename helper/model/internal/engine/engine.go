// Package engine runs a model with llama.cpp, through yzma, which loads
// its libraries at run time without cgo. Importing it unpacks and loads
// a libffi library as the program starts: only light-model does.
package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/message"
	"github.com/hybridgroup/yzma/pkg/template"

	"github.com/lantervale/light/helper/model/internal/serve"
	"github.com/lantervale/light/helper/model/internal/wire"
)

// PromptBatch is how many tokens of a request are read at a time: a
// cancel is looked at between two batches, and llama.cpp looks at it
// only now and then within one.
const PromptBatch = 256

// embeddingContext is the longest text an embedding is made of, in
// tokens.
const embeddingContext = 2048

// Engine is one llama.cpp, the model it loaded and, once asked for, the
// embedding model.
type Engine struct {
	// Batch is how many tokens of a request are read at a time, 0 for
	// PromptBatch.
	Batch int

	hello   wire.Command
	loaded  bool
	model   llama.Model
	vocab   llama.Vocab
	lctx    llama.Context
	tmpl    string
	threads int32

	embedModel llama.Model
	embedCtx   llama.Context
}

var _ serve.Engine = (*Engine)(nil)

// library loads the llama.cpp a hello names, once, and refuses one that
// is not the version light installed.
func (e *Engine) library(hello wire.Command) error {
	if e.loaded {
		return nil
	}
	for _, dir := range []string{hello.Library, hello.Backends} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return serve.Refuse(wire.CodeLibrary, "llama.cpp is not in %s", dir)
		}
	}
	if err := llama.Load(hello.Library); err != nil {
		return serve.Refuse(wire.CodeLibrary, "llama.cpp does not load from %s: %v", hello.Library, err)
	}
	if err := serve.CheckVersion(llama.Version(), hello.Llama); err != nil {
		return err
	}
	if os.Getenv("LIGHT_MODEL_VERBOSE") == "" {
		llama.LogSet(llama.LogSilent())
	}
	llama.BackendInit()
	if err := llama.GGMLBackendLoadAllFromPath(hello.Backends); err != nil {
		return serve.Refuse(wire.CodeLibrary, "llama.cpp's backends do not load from %s: %v", hello.Backends, err)
	}
	e.loaded = true
	return nil
}

func modelParams(gpu bool) llama.ModelParams {
	params := llama.ModelDefaultParams()
	if gpu {
		params.NGpuLayers = 999
	} else {
		params.SetCPUOnly()
	}
	return params
}

func open(path string, params llama.ModelParams) (llama.Model, error) {
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return 0, serve.Refuse(wire.CodeModel, "%s is not a file that can be read", filepath.Base(path))
	}
	model, err := llama.ModelLoadFromFile(path, params)
	if err != nil || model == 0 {
		return 0, serve.Refuse(wire.CodeModel, "%s is not a GGUF model this llama.cpp loads", filepath.Base(path))
	}
	return model, nil
}

// Load loads the library and the model.
func (e *Engine) Load(hello wire.Command, status func(string)) (wire.Event, error) {
	start := time.Now()
	status("loading llama.cpp")
	if err := e.library(hello); err != nil {
		return wire.Event{}, err
	}
	status("loading the model")
	model, err := open(hello.Model, modelParams(hello.GPU))
	if err != nil {
		return wire.Event{}, err
	}
	e.hello, e.model, e.vocab = hello, model, llama.ModelGetVocab(model)
	e.tmpl = llama.ModelChatTemplate(model, "")
	if e.tmpl == "" {
		return wire.Event{}, serve.Refuse(wire.CodeTemplate, "%s carries no chat template, which answering needs",
			filepath.Base(hello.Model))
	}
	trained := int(llama.ModelNCtxTrain(model))
	size := hello.Context
	if size <= 0 || size > trained {
		size = trained
	}
	params := llama.ContextDefaultParams()
	params.NCtx = uint32(size)        //nolint:gosec // a context's size
	params.NBatch = uint32(e.batch()) //nolint:gosec // a batch's size
	if hello.Threads > 0 {
		params.NThreads, params.NThreadsBatch = int32(hello.Threads), int32(hello.Threads) //nolint:gosec // a few threads
	}
	if e.lctx, err = llama.InitFromModel(model, params); err != nil || e.lctx == 0 {
		return wire.Event{}, serve.Refuse(wire.CodeModel, "llama.cpp makes no context of %d tokens for this model: %v",
			size, err)
	}
	e.threads = llama.NThreads(e.lctx)
	// The first computation is where a graphics card's programs are
	// compiled, which may take half a minute the first time.
	status("warming up")
	if err = llama.Warmup(e.lctx, model); err != nil {
		return wire.Event{}, serve.Refuse(wire.CodeModel, "the model does not run: %v", err)
	}
	return wire.Event{
		Llama: llama.Version(), Context: int(llama.NCtx(e.lctx)), TrainContext: trained, Chat: true,
		Embeddings: hello.Embedding != "", Threads: int(e.threads), Devices: devices(), Memory: peakMemory(),
		LoadMS: ms(time.Since(start)),
	}, nil
}

func (e *Engine) batch() int {
	if e.Batch > 0 {
		return e.Batch
	}
	return PromptBatch
}

func devices() []string {
	names := make([]string, 0, llama.GGMLBackendDeviceCount())
	for i := range llama.GGMLBackendDeviceCount() {
		names = append(names, llama.GGMLBackendDeviceName(llama.GGMLBackendDeviceGet(i)))
	}
	return names
}

// Inspect describes a GGUF file without loading its weights.
func (e *Engine) Inspect(hello wire.Command) (wire.Event, error) {
	if err := e.library(hello); err != nil {
		return wire.Event{}, err
	}
	params := llama.ModelDefaultParams()
	params.SetCPUOnly()
	params.VocabOnly = 1
	model, err := open(hello.Inspect, params)
	if err != nil {
		return wire.Event{}, err
	}
	defer func() { _ = llama.ModelFree(model) }()
	name, _ := llama.ModelMetaValStr(model, "general.name")
	if name == "" {
		name = llama.ModelDesc(model)
	}
	architecture, _ := llama.ModelMetaValStr(model, "general.architecture")
	_, pooled := llama.ModelMetaValStr(model, architecture+".pooling_type")
	// A file read without its weights says its context only in its
	// metadata.
	trained := int(llama.ModelNCtxTrain(model))
	if length, ok := llama.ModelMetaValStr(model, architecture+".context_length"); ok && trained == 0 {
		trained, _ = strconv.Atoi(length)
	}
	parameters := int64(llama.ModelNParams(model)) //nolint:gosec // a count of weights
	return wire.Event{
		Name: name, Architecture: architecture, Parameters: parameters,
		TrainContext: trained, Template: llama.ModelChatTemplate(model, "") != "",
		Embedding: pooled || (llama.ModelHasEncoder(model) && !llama.ModelHasDecoder(model)),
	}, nil
}

// Prompt is the text a model is given for a request: its chat template
// applied to the system text and the messages, with the model's own
// reasoning aloud turned off where a template has such a thing.
func Prompt(tmpl string, c wire.Command) (string, error) {
	messages := make([]message.Message, 0, len(c.Messages)+1)
	if c.System != "" {
		messages = append(messages, message.Chat{Role: "system", Content: c.System})
	}
	for _, m := range c.Messages {
		role := string(m.Role)
		if role != string(wire.User) && role != string(wire.Assistant) {
			return "", serve.Refuse(wire.CodeRequest, "a message is said by %q, neither the user nor the assistant", role)
		}
		messages = append(messages, message.Chat{Role: role, Content: m.Text})
	}
	if len(c.Messages) == 0 {
		return "", serve.Refuse(wire.CodeRequest, "a request needs a message")
	}
	text, err := template.ApplyWithOptions(tmpl, messages, true, template.Options{EnableThinking: false})
	if err != nil {
		return "", serve.Refuse(wire.CodeTemplate, "the model's chat template does not apply: %v", err)
	}
	return text, nil
}

// sampler chooses each token as a request asks: within its grammar,
// the likeliest at a temperature of 0, otherwise among the likely ones.
func (e *Engine) sampler(c wire.Command) (llama.Sampler, error) {
	chain := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	if c.Grammar != "" {
		grammar := llama.SamplerInitGrammar(e.vocab, c.Grammar, "root")
		if grammar == 0 {
			llama.SamplerFree(chain)
			return 0, serve.Refuse(wire.CodeRequest, "the request's grammar does not read")
		}
		llama.SamplerChainAdd(chain, grammar)
	}
	if c.Temperature <= 0 {
		llama.SamplerChainAdd(chain, llama.SamplerInitGreedy())
		return chain, nil
	}
	llama.SamplerChainAdd(chain, llama.SamplerInitTopK(40))
	llama.SamplerChainAdd(chain, llama.SamplerInitTopP(0.9, 1))
	llama.SamplerChainAdd(chain, llama.SamplerInitTemp(float32(c.Temperature)))
	llama.SamplerChainAdd(chain, llama.SamplerInitDist(c.Seed))
	return chain, nil
}

func (e *Engine) piece(token llama.Token, buf *[]byte) string {
	n := llama.TokenToPiece(e.vocab, token, *buf, 0, false)
	if n < 0 {
		*buf = make([]byte, -n)
		n = llama.TokenToPiece(e.vocab, token, *buf, 0, false)
	}
	if n <= 0 {
		return ""
	}
	return string((*buf)[:n])
}

// watch has llama.cpp stop computing for lctx once ctx is done, and
// say it is alive while it computes; the function returned ends it.
func watch(ctx context.Context, lctx llama.Context, alive func()) func() {
	llama.SetAbortCallback(lctx, func() bool {
		alive()
		return ctx.Err() != nil
	})
	return func() { llama.SetAbortCallback(lctx, nil) }
}

func forget(lctx llama.Context) error {
	memory, err := llama.GetMemory(lctx)
	if err != nil {
		return err
	}
	return llama.MemoryClear(memory, true)
}

// Generate answers a request.
func (e *Engine) Generate(ctx context.Context, c wire.Command, token func(string), alive func()) (wire.Event, error) {
	start := time.Now()
	text, err := Prompt(e.tmpl, c)
	if err != nil {
		return wire.Event{}, err
	}
	tokens := llama.Tokenize(e.vocab, text, true, true)
	room := int(llama.NCtx(e.lctx))
	if len(tokens)+c.MaxTokens > room {
		return wire.Event{}, serve.Refuse(wire.CodeTooLong,
			"the request is %d tokens and its answer up to %d, and the context holds %d",
			len(tokens), c.MaxTokens, room)
	}
	if err = forget(e.lctx); err != nil {
		return wire.Event{}, err
	}
	sampler, err := e.sampler(c)
	if err != nil {
		return wire.Event{}, err
	}
	defer llama.SamplerFree(sampler)
	defer watch(ctx, e.lctx, alive)()

	done := wire.Event{PromptTokens: len(tokens)}
	end := func(stopped string) (wire.Event, error) {
		done.Stopped, done.TotalMS, done.Memory = stopped, ms(time.Since(start)), peakMemory()
		return done, nil
	}
	for from := 0; from < len(tokens); from += e.batch() {
		if ctx.Err() != nil {
			return end(wire.StoppedCancelled)
		}
		if _, err = llama.Decode(e.lctx, llama.BatchGetOne(tokens[from:min(from+e.batch(), len(tokens))])); err != nil {
			if ctx.Err() != nil {
				return end(wire.StoppedCancelled)
			}
			return wire.Event{}, fmt.Errorf("reading the request: %w", err)
		}
	}
	out := serve.NewStopper(c.Stop, token)
	buf := make([]byte, 256)
	for {
		if ctx.Err() != nil {
			return end(wire.StoppedCancelled)
		}
		next := llama.SamplerSample(sampler, e.lctx, -1)
		if done.Tokens == 0 {
			done.FirstMS = ms(time.Since(start))
		}
		if llama.VocabIsEOG(e.vocab, next) {
			out.Flush()
			return end(wire.StoppedStop)
		}
		done.Tokens++
		if !out.Write(e.piece(next, &buf)) {
			return end(wire.StoppedStop)
		}
		if done.Tokens >= c.MaxTokens {
			out.Flush()
			return end(wire.StoppedLength)
		}
		if _, err = llama.Decode(e.lctx, llama.BatchGetOne([]llama.Token{next})); err != nil {
			if ctx.Err() != nil {
				return end(wire.StoppedCancelled)
			}
			return wire.Event{}, fmt.Errorf("writing the answer: %w", err)
		}
	}
}

// Count is how many tokens text is.
func (e *Engine) Count(text string) (int, error) {
	return len(llama.Tokenize(e.vocab, text, false, false)), nil
}

// embedder loads the embedding model the first time it is asked for: a
// context that gives one vector a text, the mean of its tokens' when
// the model names no way of its own.
func (e *Engine) embedder() error {
	if e.embedCtx != 0 {
		return nil
	}
	if e.hello.Embedding == "" {
		return serve.Refuse(wire.CodeRequest, "light named no embedding model")
	}
	if e.embedModel == 0 {
		model, err := open(e.hello.Embedding, modelParams(e.hello.GPU))
		if err != nil {
			return err
		}
		e.embedModel = model
	}
	size := min(int(llama.ModelNCtxTrain(e.embedModel)), embeddingContext)
	params := llama.ContextDefaultParams()
	tokens := uint32(size) //nolint:gosec // a context's size
	params.NCtx, params.NBatch, params.NUbatch = tokens, tokens, tokens
	params.Embeddings = 1
	params.NThreads, params.NThreadsBatch = e.threads, e.threads
	for _, pooling := range []llama.PoolingType{llama.PoolingTypeUnspecified, llama.PoolingTypeMean} {
		params.PoolingType = pooling
		lctx, err := llama.InitFromModel(e.embedModel, params)
		if err != nil || lctx == 0 {
			return serve.Refuse(wire.CodeModel, "llama.cpp makes no context for the embedding model: %v", err)
		}
		if llama.GetPoolingType(lctx) != llama.PoolingTypeNone {
			e.embedCtx = lctx
			return nil
		}
		_ = llama.Free(lctx)
	}
	return serve.Refuse(wire.CodeModel, "the embedding model gives no vector for a text")
}

// Embed returns one vector of unit length a text; a text longer than
// the embedding model reads is cut at its end.
func (e *Engine) Embed(ctx context.Context, texts []string, alive func()) ([][]float32, error) {
	if err := e.embedder(); err != nil {
		return nil, err
	}
	defer watch(ctx, e.embedCtx, alive)()
	vocab := llama.ModelGetVocab(e.embedModel)
	size, dimensions := int(llama.NCtx(e.embedCtx)), llama.ModelNEmbd(e.embedModel)
	encoder := llama.ModelHasEncoder(e.embedModel) && !llama.ModelHasDecoder(e.embedModel)
	vectors := make([][]float32, 0, len(texts))
	for _, text := range texts {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		tokens := llama.Tokenize(vocab, text, true, true)
		if len(tokens) == 0 {
			vectors = append(vectors, make([]float32, dimensions))
			continue
		}
		tokens = tokens[:min(len(tokens), size)]
		if err := forget(e.embedCtx); err != nil {
			return nil, err
		}
		var err error
		if encoder {
			_, err = llama.Encode(e.embedCtx, llama.BatchGetOne(tokens))
		} else {
			_, err = llama.Decode(e.embedCtx, llama.BatchGetOne(tokens))
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("embedding a text: %w", err)
		}
		raw, err := llama.GetEmbeddingsSeq(e.embedCtx, 0, dimensions)
		if err != nil || raw == nil {
			return nil, errors.Join(errors.New("llama.cpp gave no vector for a text"), err)
		}
		vectors = append(vectors, unit(raw))
	}
	return vectors, nil
}

// unit is a copy of v of length 1.
func unit(v []float32) []float32 {
	var length float64
	for _, x := range v {
		length += float64(x) * float64(x)
	}
	length = math.Sqrt(length)
	out := make([]float32, len(v))
	if length == 0 {
		return out
	}
	for i, x := range v {
		out[i] = float32(float64(x) / length)
	}
	return out
}

// Close frees what was loaded.
func (e *Engine) Close() {
	if e.embedCtx != 0 {
		_ = llama.Free(e.embedCtx)
	}
	if e.embedModel != 0 {
		_ = llama.ModelFree(e.embedModel)
	}
	if e.lctx != 0 {
		_ = llama.Free(e.lctx)
	}
	if e.model != 0 {
		_ = llama.ModelFree(e.model)
	}
	if e.loaded {
		llama.Close()
	}
	*e = Engine{Batch: e.Batch}
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
