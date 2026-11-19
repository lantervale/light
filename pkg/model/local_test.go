package model_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/model"
)

func TestNothingRunsAndNothingIsFetchedUntilTheModelIsOn(t *testing.T) {
	opts, b := options(t, "ok")
	b.open(opts, model.Settings{})
	if got := b.Status(); got.State != model.Off {
		t.Fatalf("a model nobody turned on is %v", got.State)
	}
	if _, err := b.Generate(context.Background(), say("hello")); !errors.Is(err, model.ErrOff) {
		t.Fatalf("a model that is off answers: %v", err)
	}
	if _, err := b.Count(context.Background(), "hello"); !errors.Is(err, model.ErrOff) {
		t.Fatalf("a model that is off counts: %v", err)
	}
	if err := b.Install(small); !errors.Is(err, model.ErrOff) {
		t.Fatalf("a model that is off downloads: %v", err)
	}
	if _, err := b.Inspect(context.Background(), "/a/file.gguf"); !errors.Is(err, model.ErrOff) {
		t.Fatalf("a model that is off starts a helper to read a file: %v", err)
	}
	b.Warm()
	time.Sleep(50 * time.Millisecond)
	if helpers, asked := b.helpers(), b.site.asked(); len(helpers) != 0 || len(asked) != 0 {
		t.Fatalf("a model that is off started %d helpers and made %d requests", len(helpers), len(asked))
	}
	if _, err := os.Stat(b.dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a model that is off wrote in its folder: %v", err)
	}
}

func TestALightThatKeepsNothingHasNoModel(t *testing.T) {
	opts, b := options(t, "ok")
	opts.Dir = ""
	b.open(opts, model.Settings{On: true})
	b.Configure(model.Settings{On: true, ID: small})
	if got := b.Status(); got.State != model.Off || got.Reason == "" {
		t.Fatalf("a light with no folder has a model that is %v (%q)", got.State, got.Reason)
	}
	if err := b.Install(small); !errors.Is(err, model.ErrOff) {
		t.Fatalf("a light with no folder downloads: %v", err)
	}
	if _, err := b.Generate(context.Background(), say("hello")); !errors.Is(err, model.ErrOff) {
		t.Fatalf("a light with no folder answers: %v", err)
	}
	if err := b.DeleteAll(); err != nil {
		t.Fatal(err)
	}
}

func TestAModelTurnedOnWithoutItsFilesIsMissing(t *testing.T) {
	opts, b := options(t, "ok")
	b.open(opts, model.Settings{On: true})
	got := b.Status()
	if got.State != model.Missing || got.Chosen.ID != small || got.Reason == "" {
		t.Fatalf("a model with no file is %v, chosen %+v, because %q", got.State, got.Chosen, got.Reason)
	}
	if _, err := b.Generate(context.Background(), say("hello")); !errors.Is(err, model.ErrMissing) {
		t.Fatalf("a model with no file answers: %v", err)
	}
	b.install(small)
	if got = b.Status(); got.State != model.Ready || got.Loaded || !got.Capabilities.Chat ||
		got.Capabilities.Context != 2048 {
		t.Fatalf("an installed model is %v, loaded %v, able of %+v", got.State, got.Loaded, got.Capabilities)
	}
	if len(b.helpers()) != 0 {
		t.Fatal("installing started the helper")
	}
}

func TestAnAnswerComesAsItIsWritten(t *testing.T) {
	b := started(t, "ok")
	before := b.Status().Seq
	stream, err := b.Generate(context.Background(), say("one two three"))
	if err != nil {
		t.Fatal(err)
	}
	var text string
	eventually(t, "the answer ends", func() bool {
		piece, done := stream.Take()
		text += piece
		return done
	})
	if text != "one two three" || stream.Err() != nil {
		t.Fatalf("the answer is %q, %v", text, stream.Err())
	}
	result, err := stream.Wait()
	if err != nil || result.Text != "one two three" || result.Stopped != model.StoppedStop || result.Tokens != 3 {
		t.Fatalf("the answer ended as %+v, %v", result, err)
	}
	eventually(t, "the model is ready again", func() bool { return b.Status().State == model.Ready })
	got := b.Status()
	if !got.Loaded || got.Memory == 0 || got.Seq == before {
		t.Fatalf("after an answer the model is %+v", got)
	}
}

func TestTheHelperIsGivenTheModelFolderAndNoneOfLightsEnvironment(t *testing.T) {
	t.Setenv("LIGHT_TEST_SECRET", "not the helper's")
	b := started(t, "ok")
	stream, err := b.Generate(context.Background(), say("one"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); err != nil {
		t.Fatal(err)
	}
	helpers := b.helpers()
	if len(helpers) != 1 {
		t.Fatalf("%d helpers were started", len(helpers))
	}
	hello := helpers[0].Hello
	if hello.Protocol != model.Protocol || hello.Llama != runtime1 || !hello.GPU || hello.Threads != model.Threads() ||
		hello.Context != 2048 || hello.Dev || hello.Embedding != "" {
		t.Fatalf("the helper was told %+v", hello)
	}
	for _, path := range []string{hello.Library, hello.Backends, filepath.Dir(hello.Model)} {
		if rel, out := filepath.Rel(b.dir, path); out != nil || !filepath.IsLocal(rel) {
			t.Fatalf("the helper was given %s, outside the model folder", path)
		}
	}
	for _, name := range []string{"libllama.dylib", "libggml-metal.dylib"} {
		if _, absent := os.Stat(filepath.Join(hello.Library, name)); absent != nil {
			t.Fatalf("the library's folder lacks %s: %v", name, absent)
		}
	}
	for _, variable := range helpers[0].Env {
		name, value, _ := strings.Cut(variable, "=")
		inside := strings.HasPrefix(value, b.dir)
		switch {
		case name == envMode || name == envNotes:
		case runtime.GOOS == "windows" && (name == "SystemRoot" || name == "SystemDrive" || name == "windir"):
		case !inside:
			t.Errorf("the helper was given %s of light's environment", variable)
		}
	}
}

func TestAHelperOfAnotherProtocolIsRefused(t *testing.T) {
	b := started(t, "old")
	stream, err := b.Generate(context.Background(), say("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); !errors.Is(err, model.ErrHelper) || !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("a helper of another protocol answered: %v", err)
	}
	if got := b.Status(); got.State != model.Failed || !strings.Contains(got.Reason, "protocol") {
		t.Fatalf("the model is %v because %q", got.State, got.Reason)
	}
	if _, err = b.Generate(context.Background(), say("again")); !errors.Is(err, model.ErrHelper) {
		t.Fatalf("a refused helper is asked again: %v", err)
	}
	if n := len(b.helpers()); n != 1 {
		t.Fatalf("a refused helper was started %d times", n)
	}
	// The settings changed: it is tried again.
	b.Configure(model.Settings{On: true, GPU: false})
	if _, err = b.Generate(context.Background(), say("again")); err != nil {
		t.Fatalf("new settings do not try the helper again: %v", err)
	}
}

func TestAHelperThatRefusesItsLibraryFailsTheModel(t *testing.T) {
	b := started(t, "refuse")
	stream, err := b.Generate(context.Background(), say("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); err == nil || !strings.Contains(err.Error(), "9.9.9") {
		t.Fatalf("a helper that refused its library answered: %v", err)
	}
	if got := b.Status(); got.State != model.Failed || !strings.Contains(got.Reason, "9.9.9") {
		t.Fatalf("the model is %v because %q", got.State, got.Reason)
	}
}

func TestCancellingEndsTheAnswerAtOnceAndTheHelperWithinAToken(t *testing.T) {
	b := started(t, "ok", func(o *model.Options) { o.Env = append(o.Env, envPace+"=20") })
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := b.Generate(ctx, say(strings.Repeat("word ", 500)))
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "a first word comes", func() bool { text, _ := stream.Take(); return text != "" })
	at := time.Now()
	cancel()
	if _, err = stream.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled answer ended with %v", err)
	}
	if took := time.Since(at); took > time.Second {
		t.Fatalf("a cancelled answer took %v to end", took)
	}
	second, err := b.Generate(context.Background(), say("still here"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := second.Wait()
	if err != nil || result.Text != "still here" {
		t.Fatalf("after a cancel the helper answers %q, %v", result.Text, err)
	}
	if took := time.Since(at); took > 5*time.Second {
		t.Fatalf("the helper took %v to stop: it wrote its 500 words out", took)
	}
	if n := len(b.helpers()); n != 1 {
		t.Fatalf("a cancel started %d helpers", n)
	}
}

func TestARequestCancelledWhileItWaitsNeverReachesTheHelper(t *testing.T) {
	b := started(t, "ok", func(o *model.Options) { o.Env = append(o.Env, envPace+"=10") })
	first, err := b.Generate(context.Background(), say(strings.Repeat("slow ", 20)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	waiting, err := b.Generate(ctx, say("never asked"))
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err = waiting.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = first.Wait(); err != nil {
		t.Fatal(err)
	}
	last, err := b.Generate(context.Background(), say("last"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = last.Wait(); err != nil {
		t.Fatal(err)
	}
	if asked := b.helpers()[0].Asked; slices.Contains(asked, "never asked") {
		t.Fatalf("the helper was asked %q", asked)
	}
}

func TestARequestSomebodyWaitsForGoesFirstAndStopsABackgroundOne(t *testing.T) {
	b := started(t, "ok", func(o *model.Options) { o.Env = append(o.Env, envPace+"=10") })
	long := strings.TrimSpace(strings.Repeat("category ", 60))
	background, err := b.Generate(context.Background(), model.Request{
		Messages: []model.Message{{Role: model.User, Text: long}}, Priority: model.Background,
	})
	if err != nil {
		t.Fatal(err)
	}
	later, err := b.Generate(context.Background(), model.Request{
		Messages: []model.Message{{Role: model.User, Text: "second in the background"}}, Priority: model.Background,
	})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "the background request is being answered", func() bool {
		helpers := b.helpers()
		return len(helpers) == 1 && len(helpers[0].Asked) == 1
	})
	time.Sleep(50 * time.Millisecond)
	if text, done := background.Take(); text != "" || done {
		t.Fatalf("a background answer shows %q before it is whole", text)
	}
	interactive, err := b.Generate(context.Background(), say("now please"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := interactive.Wait()
	if err != nil || result.Text != "now please" {
		t.Fatalf("the interactive answer is %q, %v", result.Text, err)
	}
	select {
	case <-background.Done():
		t.Fatal("the background answer ended before the interactive one")
	default:
	}
	whole, err := background.Wait()
	if err != nil || whole.Text != long {
		t.Fatalf("the background answer, started over, is %d bytes of %d, %v", len(whole.Text), len(long), err)
	}
	if _, err = later.Wait(); err != nil {
		t.Fatal(err)
	}
	want := []string{long, "now please", long, "second in the background"}
	if asked := b.helpers()[0].Asked; !slices.Equal(asked, want) {
		t.Fatalf("the helper was asked, in order, %d requests: %.40q", len(asked), asked)
	}
}

func TestAHelperThatCrashesFailsItsAnswerAndIsStartedAgain(t *testing.T) {
	b := started(t, "ok")
	stream, err := b.Generate(context.Background(), say("crash in the middle of this"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Wait()
	if !errors.Is(err, model.ErrHelper) || !strings.Contains(err.Error(), "segmentation fault") {
		t.Fatalf("a crash ended the answer with %v", err)
	}
	if result.Text != "crash in " {
		t.Fatalf("what came before the crash is %q", result.Text)
	}
	if got := b.Status(); got.State != model.Failed || got.Loaded || got.Reason == "" {
		t.Fatalf("after a crash the model is %+v", got)
	}
	again, err := b.Generate(context.Background(), say("back again"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err = again.Wait(); err != nil || result.Text != "back again" {
		t.Fatalf("after a crash the helper answers %q, %v", result.Text, err)
	}
	helpers := b.helpers()
	if len(helpers) != 2 || helpers[0].PID == helpers[1].PID {
		t.Fatalf("a crash led to %d helpers", len(helpers))
	}
	if got := b.Status(); got.State != model.Ready || got.Reason != "" {
		t.Fatalf("after its restart the model is %v (%q)", got.State, got.Reason)
	}
	log, err := os.ReadFile(filepath.Join(b.dir, "helper.log"))
	if err != nil || !strings.Contains(string(log), "segmentation fault") {
		t.Fatalf("the helper's log holds %q, %v", log, err)
	}
}

func TestAHelperThatKeepsDyingIsStartedAgainLessAndLessOften(t *testing.T) {
	b := started(t, "dies")
	stream, err := b.Generate(context.Background(), say("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); !errors.Is(err, model.ErrHelper) || !strings.Contains(err.Error(), "dyld") {
		t.Fatalf("a helper that dies at each start answered: %v", err)
	}
	if got := b.Status(); got.State != model.Failed || !strings.Contains(got.Reason, "dyld") {
		t.Fatalf("the model is %v (%q)", got.State, got.Reason)
	}
	at := time.Now()
	for range 3 {
		if stream, err = b.Generate(context.Background(), say("again")); err != nil {
			t.Fatal(err)
		}
		if _, err = stream.Wait(); !errors.Is(err, model.ErrHelper) {
			t.Fatal(err)
		}
	}
	if n := len(b.helpers()); n != 4 {
		t.Fatalf("four requests started it %d times", n)
	}
	// 20, 40 and 80 ms, the last capped at 100, were waited between the starts.
	if took := time.Since(at); took < 140*time.Millisecond {
		t.Fatalf("three more starts took %v: they did not wait", took)
	}
}

func TestAHelperThatWritesNonsenseIsKilled(t *testing.T) {
	b := started(t, "garbage")
	stream, err := b.Generate(context.Background(), say("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); !errors.Is(err, model.ErrHelper) {
		t.Fatalf("a helper that writes nonsense answered: %v", err)
	}
}

func TestAHelperThatStopsAnsweringIsKilled(t *testing.T) {
	for mode, text := range map[string]string{"ok": "hang after this", "mute": "hello", "slow": "hello"} {
		t.Run(mode, func(t *testing.T) {
			b := started(t, mode, func(o *model.Options) {
				o.Handshake, o.Start, o.Silence = 150*time.Millisecond, 150*time.Millisecond, 150*time.Millisecond
				o.Backoff, o.MaxBackoff = time.Hour, time.Hour
			})
			stream, err := b.Generate(context.Background(), say(text))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = stream.Wait(); !errors.Is(err, model.ErrHelper) {
				t.Fatalf("a helper that stopped answering ended the request with %v", err)
			}
			helpers := b.helpers()
			if len(helpers) != 1 {
				t.Fatalf("%d helpers", len(helpers))
			}
			eventually(t, "the helper is killed", func() bool { return !alive(helpers[0].PID) })
			if got := b.Status(); got.State != model.Failed {
				t.Fatalf("the model is %v", got.State)
			}
		})
	}
}

func TestAHelperToldToStopThatGoesOnIsKilled(t *testing.T) {
	b := started(t, "ok", func(o *model.Options) { o.CancelWait = 150 * time.Millisecond })
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := b.Generate(ctx, say("hang and ignore the cancel"))
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "a first word comes", func() bool { text, _ := stream.Take(); return text != "" })
	cancel()
	pid := b.helpers()[0].PID
	eventually(t, "the helper is killed", func() bool { return !alive(pid) })
	next, err := b.Generate(context.Background(), say("a new helper"))
	if err != nil {
		t.Fatal(err)
	}
	if result, failed := next.Wait(); failed != nil || result.Text != "a new helper" {
		t.Fatalf("%q, %v", result.Text, err)
	}
}

func TestAHelperNobodyAsksIsStoppedAndStartsAgainWhenAsked(t *testing.T) {
	b := started(t, "ok", func(o *model.Options) { o.Idle = 100 * time.Millisecond })
	for range 2 {
		stream, err := b.Generate(context.Background(), say("hello"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = stream.Wait(); err != nil {
			t.Fatal(err)
		}
		eventually(t, "the model is loaded", func() bool { return b.Status().Loaded })
		eventually(t, "the model left memory", func() bool {
			got := b.Status()
			return !got.Loaded &&
				got.State == model.Ready
		})
	}
	helpers := b.helpers()
	if len(helpers) != 2 {
		t.Fatalf("%d helpers were started for two requests an idle time apart", len(helpers))
	}
	for _, h := range helpers {
		eventually(t, "the idle helper is gone", func() bool { return !alive(h.PID) })
	}
}

func TestWarmLoadsTheModelAhead(t *testing.T) {
	b := started(t, "ok")
	b.Warm()
	eventually(t, "the model is loaded", func() bool { return b.Status().Loaded })
	if got := b.Status(); got.State != model.Ready {
		t.Fatalf("a warmed model is %v", got.State)
	}
	if n := len(b.helpers()); n != 1 {
		t.Fatalf("%d helpers", n)
	}
}

func TestPreparingSaysWhatTheHelperDoes(t *testing.T) {
	b := started(t, "slow")
	if _, err := b.Generate(context.Background(), say("hello")); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the helper says what it does", func() bool {
		got := b.Status()
		return got.State == model.Preparing && got.Detail == "loading the model"
	})
}

func TestCloseEndsTheAnswersTheHelperAndTheGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()
	b := started(t, "ok", func(o *model.Options) { o.Env = append(o.Env, envPace+"=20") })
	running, err := b.Generate(context.Background(), say(strings.Repeat("word ", 500)))
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := b.Generate(context.Background(), say("behind"))
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "a first word comes", func() bool { text, _ := running.Take(); return text != "" })
	pid := b.helpers()[0].PID
	if err = b.Close(); err != nil {
		t.Fatal(err)
	}
	for _, stream := range []*model.Stream{running, waiting} {
		if _, err = stream.Wait(); !errors.Is(err, model.ErrClosed) {
			t.Fatalf("a closed model ended a request with %v", err)
		}
	}
	if alive(pid) {
		t.Fatal("the helper outlives the model")
	}
	if _, err = b.Generate(context.Background(), say("late")); !errors.Is(err, model.ErrClosed) {
		t.Fatalf("a closed model takes a request: %v", err)
	}
	if err = b.Close(); err != nil {
		t.Fatal(err)
	}
	b.site.CloseClientConnections()
	b.site.Client().CloseIdleConnections()
	eventually(t, "its goroutines ended", func() bool { return runtime.NumGoroutine() <= before+2 })
}

func TestCountAndEmbedGoThroughTheHelper(t *testing.T) {
	b := started(t, "ok")
	n, err := b.Count(context.Background(), "four words of text")
	if err != nil || n != 4 {
		t.Fatalf("the count is %d, %v", n, err)
	}
	if _, err = b.Embed(context.Background(), model.Texts{Texts: []string{"a"}}); !errors.Is(err, model.ErrNoEmbeddings) {
		t.Fatalf("with no embedding model installed, Embed says %v", err)
	}
	if b.Status().Capabilities.Embeddings {
		t.Fatal("the model says it embeds")
	}
	b.install(embeds)
	eventually(t, "the model embeds", func() bool { return b.Status().Capabilities.Embeddings })
	vectors, err := b.Embed(context.Background(), model.Texts{Texts: []string{"abc", "abcdef"}, Kind: model.Query})
	if err != nil || len(vectors) != 2 {
		t.Fatalf("the vectors are %v, %v", vectors, err)
	}
	if prefix := float32(len("query: ")); vectors[0][0] != prefix+3 || vectors[1][0] != prefix+6 {
		t.Fatalf("the texts were not given their query prefix: %v", vectors)
	}
	documents, err := b.Embed(context.Background(), model.Texts{Texts: []string{"abc"}, Priority: model.Background})
	if err != nil || documents[0][0] != float32(len("passage: "))+3 {
		t.Fatalf("a document was not given its prefix: %v, %v", documents, err)
	}
	if got := b.Status().Capabilities; got.Dimensions != 2 {
		t.Fatalf("the vectors' length is said to be %d", got.Dimensions)
	}
	helpers := b.helpers()
	if last := helpers[len(helpers)-1]; filepath.Base(last.Hello.Embedding) != "embeds.gguf" {
		t.Fatalf("the helper that embeds was given %q", last.Hello.Embedding)
	}
	if vectors, err = b.Embed(context.Background(), model.Texts{}); err != nil || vectors != nil {
		t.Fatalf("no text gives %v, %v", vectors, err)
	}
}

func TestSwitchingModelsEndsTheAnswerAndStartsTheHelperOnTheOther(t *testing.T) {
	b := started(t, "ok", func(o *model.Options) { o.Env = append(o.Env, envPace+"=20") })
	b.install(big)
	stream, err := b.Generate(context.Background(), say(strings.Repeat("word ", 500)))
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "a first word comes", func() bool { text, _ := stream.Take(); return text != "" })
	b.Configure(model.Settings{On: true, GPU: true, ID: big, Threads: 3})
	if _, err = stream.Wait(); !errors.Is(err, model.ErrChanged) {
		t.Fatalf("an answer whose model was switched ended with %v", err)
	}
	if got := b.Status(); got.Chosen.ID != big || got.Capabilities.Context != 4096 {
		t.Fatalf("the model chosen is %+v, able of %+v", got.Chosen, got.Capabilities)
	}
	next, err := b.Generate(context.Background(), say("on the big one"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = next.Wait(); err != nil {
		t.Fatal(err)
	}
	helpers := b.helpers()
	if len(helpers) != 2 {
		t.Fatalf("%d helpers", len(helpers))
	}
	if hello := helpers[1].Hello; filepath.Base(hello.Model) != "big.gguf" || hello.Threads != 3 || hello.Context != 4096 {
		t.Fatalf("the second helper was told %+v", hello)
	}
	eventually(t, "the first helper is gone", func() bool { return !alive(helpers[0].PID) })

	b.Configure(model.Settings{})
	if _, err = b.Generate(context.Background(), say("off")); !errors.Is(err, model.ErrOff) {
		t.Fatalf("a model turned off answers: %v", err)
	}
	eventually(t, "the second helper is gone", func() bool { return !alive(helpers[1].PID) })
	if got := b.Status(); got.State != model.Off || got.Loaded {
		t.Fatalf("a model turned off is %+v", got)
	}
}

func TestAModelTheManifestDoesNotKnowFails(t *testing.T) {
	b := started(t, "ok")
	b.Configure(model.Settings{On: true, ID: "unheard-of"})
	if got := b.Status(); got.State != model.Failed || !strings.Contains(got.Reason, "unheard-of") {
		t.Fatalf("an unknown model is %v (%q)", got.State, got.Reason)
	}
	b.Configure(model.Settings{On: true, ID: embeds})
	if got := b.Status(); got.State != model.Failed {
		t.Fatalf("an embedding model chosen to chat is %v", got.State)
	}
}

func TestADevelopersFilesAreUsedButNotByARelease(t *testing.T) {
	dev := model.Dev{Library: "/opt/llama/lib", Backends: "/opt/ggml", Model: "/models/mine.gguf",
		Embedding: "/models/embed.gguf"}
	opts, b := options(t, "ok")
	opts.Dev = dev
	b.open(opts, model.Settings{On: true})
	if got := b.Status(); got.State != model.Ready || got.Chosen.File != dev.Model || !strings.Contains(got.Chosen.Name,
		"developer") {
		t.Fatalf("with a developer's files the model is %v, %+v", got.State, got.Chosen)
	}
	stream, err := b.Generate(context.Background(), say("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); err != nil {
		t.Fatal(err)
	}
	hello := b.helpers()[0].Hello
	if hello.Library != dev.Library || hello.Backends != dev.Backends || hello.Model != dev.Model ||
		hello.Embedding != dev.Embedding || !hello.Dev {
		t.Fatalf("the helper was told %+v", hello)
	}
	if len(b.site.asked()) != 0 {
		t.Fatal("a developer's files were downloaded over")
	}

	opts, released := options(t, "ok")
	opts.Dev, opts.Release = dev, true
	released.open(opts, model.Settings{On: true})
	if got := released.Status(); got.State != model.Missing || got.Chosen.File != "" {
		t.Fatalf("a released light took a developer's files: %v, %+v", got.State, got.Chosen)
	}
}

func TestAReleasedLightTakesOnlyItsOwnHelper(t *testing.T) {
	b := started(t, "ok", func(o *model.Options) { o.Release, o.Build = true, "1.2.3" })
	stream, err := b.Generate(context.Background(), say("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); err != nil {
		t.Fatalf("a helper of the same release is refused: %v", err)
	}

	other := started(t, "another", func(o *model.Options) { o.Release, o.Build = true, "1.2.3" })
	if stream, err = other.Generate(context.Background(), say("hello")); err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); !errors.Is(err, model.ErrHelper) || !strings.Contains(err.Error(), "0.0.1") {
		t.Fatalf("a helper of another release answered: %v", err)
	}
}

func TestWithNoHelperBesideLightTheModelFails(t *testing.T) {
	b := started(t, "ok", func(o *model.Options) { o.Helper = "" })
	stream, err := b.Generate(context.Background(), say("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); err == nil || !strings.Contains(err.Error(), "light-model") {
		t.Fatalf("with no helper the request ended with %v", err)
	}
	if got := b.Status(); got.State != model.Failed || !strings.Contains(got.Reason, "light-model") {
		t.Fatalf("with no helper the model is %v (%q)", got.State, got.Reason)
	}
}
