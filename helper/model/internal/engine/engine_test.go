package engine_test

import (
	"bufio"
	"encoding/json"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/helper/model/internal/engine"
	"github.com/lantervale/light/helper/model/internal/serve"
	"github.com/lantervale/light/helper/model/internal/wire"
)

// These tests run a real model. They are skipped unless envDev names a
// llama.cpp and a model already on this machine, as light's
// LIGHT_MODEL_DEV does: "library=DIR;backends=DIR;model=FILE". Nothing
// is downloaded.
const (
	envDev     = "LIGHT_MODEL_TEST_DEV"
	envBatch   = "LIGHT_MODEL_TEST_BATCH"
	envMeasure = "LIGHT_MODEL_TEST_MEASURE"
)

func dev(t *testing.T) wire.Command {
	t.Helper()
	value := os.Getenv(envDev)
	if value == "" {
		t.Skipf("%s names no llama.cpp and no model on this machine", envDev)
	}
	hello := wire.Command{Op: wire.OpHello, Protocol: wire.Protocol, Llama: "1.0.0", GPU: true, Threads: 4,
		Context: 8192, Dev: true}
	if v := os.Getenv("LIGHT_MODEL_TEST_LLAMA"); v != "" {
		hello.Llama = v
	}
	for part := range strings.SplitSeq(value, ";") {
		name, path, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch name {
		case "library":
			hello.Library = path
		case "backends":
			hello.Backends = path
		case "model":
			hello.Model = path
		}
	}
	if _, err := os.Stat(hello.Model); err != nil {
		t.Skipf("the model of %s is not there: %v", envDev, err)
	}
	// The model stands in for an embedding model: what is checked is
	// the way, not the vectors' worth.
	hello.Embedding = hello.Model
	return hello
}

type session struct {
	t      *testing.T
	to     *io.PipeWriter
	events chan wire.Event
	ended  chan error
}

func run(t *testing.T, e *engine.Engine, hello wire.Command) *session {
	t.Helper()
	in, to := io.Pipe()
	from, out := io.Pipe()
	s := &session{t: t, to: to, events: make(chan wire.Event, 4096), ended: make(chan error, 1)}
	go func() {
		err := serve.Run(in, out, e, serve.Options{Build: "test"})
		_ = out.Close()
		s.ended <- err
	}()
	go func() {
		lines := bufio.NewScanner(from)
		lines.Buffer(make([]byte, 1<<20), 1<<26)
		for lines.Scan() {
			var ev wire.Event
			if err := json.Unmarshal(lines.Bytes(), &ev); err != nil {
				t.Errorf("the helper wrote %q", lines.Text())
			}
			s.events <- ev
		}
		close(s.events)
	}()
	s.send(hello)
	return s
}

func (s *session) send(c wire.Command) {
	s.t.Helper()
	line, _ := json.Marshal(c)
	if _, err := s.to.Write(append(line, '\n')); err != nil {
		s.t.Fatal(err)
	}
}

func (s *session) next() wire.Event {
	s.t.Helper()
	select {
	case ev, open := <-s.events:
		if !open {
			s.t.Fatal("the helper ended")
		}
		return ev
	case <-time.After(5 * time.Minute):
		s.t.Fatal("the helper says nothing")
	}
	return wire.Event{}
}

// answer is the text of request id and the event that ended it.
func (s *session) answer(id int, each func(n int)) (string, wire.Event) {
	s.t.Helper()
	var text strings.Builder
	n := 0
	for {
		ev := s.next()
		switch {
		case ev.ID != id:
		case ev.Event == wire.EventToken:
			text.WriteString(ev.Text)
			n++
			if each != nil {
				each(n)
			}
		case ev.Event != wire.EventAlive:
			return text.String(), ev
		}
	}
}

func user(text string) []wire.Message { return []wire.Message{{Role: wire.User, Text: text}} }

func batch() int {
	n, _ := strconv.Atoi(os.Getenv(envBatch))
	return n
}

func TestALibraryOfAnotherVersionIsRefusedBeforeAModelIsLoaded(t *testing.T) {
	hello := dev(t)
	hello.Llama = "9.9.9"
	s := run(t, &engine.Engine{}, hello)
	var last wire.Event
	for last = s.next(); last.Event != wire.EventError; last = s.next() {
		if last.Event == wire.EventStatus && last.Text != "loading llama.cpp" {
			t.Fatalf("with a library of another version the helper went on to %q", last.Text)
		}
	}
	if last.Code != wire.CodeVersion || !strings.Contains(last.Error, "9.9.9") {
		t.Fatalf("the helper said %+v", last)
	}
	if err := <-s.ended; err == nil {
		t.Fatal("the helper went on")
	}
}

func TestARealModelAnswersStopsCountsAndEmbeds(t *testing.T) {
	hello := dev(t)
	e := &engine.Engine{Batch: batch()}

	describes(t, e, hello)
	s := loads(t, e, hello)
	answers(t, s)
	stopsWhenCancelled(t, s)
	keepsToWhatIsAsked(t, s)
	countsAndEmbeds(t, s)

	if os.Getenv(envMeasure) != "" {
		measure(t, s)
	}
	_ = s.to.Close()
	select {
	case err := <-s.ended:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Minute):
		t.Fatal("the helper does not leave")
	}
}

// describes has the file described without its weights.
func describes(t *testing.T, e *engine.Engine, hello wire.Command) {
	t.Helper()
	described := hello
	described.Model, described.Embedding, described.Inspect = "", "", hello.Model
	file := run(t, e, described)
	var info wire.Event
	for info = file.next(); info.Event != wire.EventFile; info = file.next() {
		if info.Event == wire.EventError {
			t.Fatalf("the model was not described: %s", info.Error)
		}
	}
	if info.Architecture == "" || info.Parameters < 1e8 || info.TrainContext < 2048 || !info.Template || info.Embedding {
		t.Fatalf("the model is described as %+v", info)
	}
	t.Logf("the file: %q, %s, %d weights, trained for %d tokens", info.Name, info.Architecture, info.Parameters,
		info.TrainContext)
	if err := <-file.ended; err != nil {
		t.Fatal(err)
	}
}

// loads loads the model, and checks what the helper says meanwhile.
func loads(t *testing.T, e *engine.Engine, hello wire.Command) *session {
	t.Helper()
	at := time.Now()
	s := run(t, e, hello)
	var ready wire.Event
	var said []string
	for ready = s.next(); ready.Event != wire.EventReady; ready = s.next() {
		if ready.Event == wire.EventError {
			t.Fatalf("the model did not load: %s", ready.Error)
		}
		if ready.Event == wire.EventStatus {
			said = append(said, ready.Text)
		}
	}
	if strings.Join(said, ", ") != "loading llama.cpp, loading the model, warming up" {
		t.Fatalf("while loading the helper said %q", said)
	}
	if ready.Llama != hello.Llama || ready.Context != min(8192, ready.TrainContext) || !ready.Chat || !ready.Embeddings ||
		ready.Threads != 4 || ready.Memory == 0 {
		t.Fatalf("the helper is ready as %+v", ready)
	}
	t.Logf("ready in %v: context %d of %d, devices %q, peak memory %d MB", time.Since(at).Round(time.Millisecond),
		ready.Context, ready.TrainContext, ready.Devices, ready.Memory>>20)
	return s
}

// answers asks a question twice: at a temperature of 0 the answer is the same.
func answers(t *testing.T, s *session) {
	t.Helper()
	s.send(wire.Command{Op: wire.OpGenerate, ID: 1, System: "You answer in one short sentence.",
		Messages: user("What is the capital of France?"), MaxTokens: 40})
	text, done := s.answer(1, nil)
	if done.Event != wire.EventDone || done.Stopped != wire.StoppedStop || !strings.Contains(text, "Paris") ||
		done.Tokens == 0 || done.PromptTokens == 0 || done.FirstMS <= 0 {
		t.Fatalf("the answer is %q, ended as %+v", text, done)
	}
	t.Logf("%q: first token after %.0f ms, %d tokens in %.0f ms", text, done.FirstMS, done.Tokens, done.TotalMS)

	// The same request, at a temperature of 0, has the same answer.
	s.send(wire.Command{Op: wire.OpGenerate, ID: 2, System: "You answer in one short sentence.",
		Messages: user("What is the capital of France?"), MaxTokens: 40})
	if again, _ := s.answer(2, nil); again != text {
		t.Fatalf("asked again the answer is %q, not %q", again, text)
	}
}

// stopsWhenCancelled cancels an answer, and asks again.
func stopsWhenCancelled(t *testing.T, s *session) {
	t.Helper()
	s.send(wire.Command{Op: wire.OpGenerate, ID: 3, Messages: user("Count from 1 to 200, one number a line."),
		MaxTokens: 600})
	cancelled := time.Time{}
	_, done := s.answer(3, func(n int) {
		if n == 10 {
			cancelled = time.Now()
			s.send(wire.Command{Op: wire.OpCancel, ID: 3})
		}
	})
	if done.Stopped != wire.StoppedCancelled || done.Tokens > 40 {
		t.Fatalf("a cancelled answer ended as %+v", done)
	}
	t.Logf("cancelled while writing: stopped %v later, %d tokens written", time.Since(cancelled).Round(time.Millisecond),
		done.Tokens)

	s.send(wire.Command{Op: wire.OpGenerate, ID: 4, Messages: user("Reply with the single word: ready"), MaxTokens: 8})
	text, after := s.answer(4, nil)
	if after.Stopped == wire.StoppedCancelled || !strings.Contains(strings.ToLower(text), "ready") {
		t.Fatalf("after a cancel the answer is %q, ended as %+v", text, after)
	}
}

// keepsToWhatIsAsked checks a grammar, a stop text, the longest answer and the context.
func keepsToWhatIsAsked(t *testing.T, s *session) {
	t.Helper()
	s.send(wire.Command{Op: wire.OpGenerate, ID: 5, Messages: user("Is a page selling shoes news, a shop or a recipe?"),
		Grammar: `root ::= "news" | "shop" | "recipe"`, MaxTokens: 8})
	if text, _ := s.answer(5, nil); text != "shop" {
		t.Errorf("kept to three words the answer is %q", text)
	}
	s.send(wire.Command{Op: wire.OpGenerate, ID: 6, Messages: user("x"), Grammar: `root ::= ( "a"`, MaxTokens: 8})
	if _, done := s.answer(6, nil); done.Event != wire.EventError {
		t.Errorf("a grammar that does not read ended as %+v", done)
	}
	s.send(wire.Command{Op: wire.OpGenerate, ID: 7, Messages: user("Write the letters A B C D E F separated by spaces."),
		Stop: []string{" D"}, MaxTokens: 30})
	if text, done := s.answer(7, nil); strings.Contains(text, "D") || !strings.Contains(text, "C") ||
		done.Stopped != wire.StoppedStop {
		t.Errorf("stopped before D the answer is %q, ended as %+v", text, done)
	}
	s.send(wire.Command{Op: wire.OpGenerate, ID: 8, Messages: user("Tell a long story."), MaxTokens: 5})
	if _, done := s.answer(8, nil); done.Stopped != wire.StoppedLength || done.Tokens != 5 {
		t.Errorf("an answer of five tokens at most ended as %+v", done)
	}
	s.send(wire.Command{Op: wire.OpGenerate, ID: 9, Messages: user(strings.Repeat("far too long ", 4000)), MaxTokens: 100})
	if _, done := s.answer(9, nil); done.Event != wire.EventError || done.Code != wire.CodeTooLong {
		t.Errorf("a request over the context ended as %+v", done)
	}
}

// countsAndEmbeds counts a text and embeds three, then asks again.
func countsAndEmbeds(t *testing.T, s *session) {
	t.Helper()
	s.send(wire.Command{Op: wire.OpCount, ID: 10, Text: "The quick brown fox jumps over the lazy dog."})
	if _, done := s.answer(10, nil); done.Count < 8 || done.Count > 14 {
		t.Errorf("nine words are %d tokens", done.Count)
	}

	s.send(wire.Command{Op: wire.OpEmbed, ID: 11, Texts: []string{
		"How do I bake sourdough bread at home?", "A recipe for baking a loaf of sourdough.",
		"The quarterly earnings of a steel company.",
	}})
	_, done := s.answer(11, nil)
	if done.Event != wire.EventVectors || len(done.Vectors) != 3 || done.Dimensions < 64 ||
		len(done.Vectors[0]) != done.Dimensions {
		t.Fatalf("the vectors came as %+v", done.Event)
	}
	if length := dot(done.Vectors[0], done.Vectors[0]); math.Abs(float64(length)-1) > 1e-3 {
		t.Errorf("a vector's length is %v", length)
	}
	near, far := dot(done.Vectors[0], done.Vectors[1]), dot(done.Vectors[0], done.Vectors[2])
	t.Logf("%d dimensions; bread to bread %.3f, bread to steel %.3f", done.Dimensions, near, far)
	if near <= far {
		t.Errorf("two texts on bread are no nearer (%.3f) than bread and steel (%.3f)", near, far)
	}

	s.send(wire.Command{Op: wire.OpGenerate, ID: 12, Messages: user("Reply with the single word: done"), MaxTokens: 8})
	if text, _ := s.answer(12, nil); !strings.Contains(strings.ToLower(text), "done") {
		t.Errorf("after embedding the answer is %q", text)
	}
}

func dot(a, b []float32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// measure times a long request: its first token, its pace, and how long
// a cancel sent while the request is still being read takes.
func measure(t *testing.T, s *session) {
	t.Helper()
	page := strings.Repeat("The lighthouse keeper logged the weather, the tide and each passing ship in a ledger. ", 160)
	ask := wire.Command{Op: wire.OpGenerate, System: "You summarise pages in two sentences.",
		Messages: user(page + "\n\nSummarise the page above."), MaxTokens: 120}
	for id := 100; id < 103; id++ {
		ask.ID = id
		s.send(ask)
		_, done := s.answer(id, nil)
		pace := float64(done.Tokens-1) / ((done.TotalMS - done.FirstMS) / 1000)
		t.Logf("MEASURE batch=%d prompt=%d tokens first=%.0fms tokens=%d pace=%.1f tok/s total=%.0fms peak=%dMB",
			batch(), done.PromptTokens, done.FirstMS, done.Tokens, pace, done.TotalMS, done.Memory>>20)
	}
	for id, after := range map[int]time.Duration{200: 30 * time.Millisecond, 201: 100 * time.Millisecond,
		202: 200 * time.Millisecond, 203: 60 * time.Millisecond, 204: 150 * time.Millisecond} {
		ask.ID = id
		s.send(ask)
		time.Sleep(after)
		at := time.Now()
		s.send(wire.Command{Op: wire.OpCancel, ID: id})
		_, done := s.answer(id, nil)
		t.Logf("MEASURE batch=%d cancel sent %v into the request: stopped after %.1f ms (%s, %d tokens written)",
			batch(), after, float64(time.Since(at).Microseconds())/1000, done.Stopped, done.Tokens)
	}
}
