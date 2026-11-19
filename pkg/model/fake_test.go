package model_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/model"
)

func TestAFakeAnswersAtOnceWhatItIsToldTo(t *testing.T) {
	f := model.NewFake()
	if got := f.Status(); got.State != model.Ready || !got.Capabilities.Chat || !got.Capabilities.Embeddings ||
		got.Chosen.ID == "" {
		t.Fatalf("a new fake is %+v", got)
	}
	stream, err := f.Generate(context.Background(), say("said back"))
	if err != nil {
		t.Fatal(err)
	}
	if text, done := stream.Take(); text != "said back" || !done {
		t.Fatalf("a fake's answer is %q, done %v", text, done)
	}
	f.Reply = func(req model.Request) (string, error) {
		if req.System == "fail" {
			return "", errors.New("no")
		}
		return "a summary of " + req.Messages[0].Text, nil
	}
	stream, _ = f.Generate(context.Background(), say("the page"))
	if result, failed := stream.Wait(); failed != nil || result.Text != "a summary of the page" || result.Tokens != 5 {
		t.Fatalf("%+v, %v", result, err)
	}
	stream, _ = f.Generate(context.Background(), model.Request{System: "fail", Messages: say("x").Messages})
	if _, err = stream.Wait(); err == nil {
		t.Fatal("a reply's error is lost")
	}
	if asked := f.Asked(); len(asked) != 3 || asked[2].System != "fail" {
		t.Fatalf("the fake was asked %+v", asked)
	}
	if _, err = f.Generate(context.Background(), model.Request{}); err == nil {
		t.Fatal("a request with no message is taken")
	}
	f.Warm()
	if f.Warmed() != 1 {
		t.Fatal("Warm is not counted")
	}
}

func TestAManualFakeGivesAWordAStep(t *testing.T) {
	f := model.NewFake()
	f.Manual = true
	seq := f.Status().Seq
	stream, err := f.Generate(context.Background(), say("one two three"))
	if err != nil {
		t.Fatal(err)
	}
	background, _ := f.Generate(context.Background(), model.Request{Messages: say("a b").Messages,
		Priority: model.Background})
	if text, done := stream.Take(); text != "" || done {
		t.Fatalf("before a step the answer is %q", text)
	}
	if got := f.Status(); got.State != model.Working || got.Seq == seq || got.Waiting != 1 {
		t.Fatalf("with an answer held the fake is %+v", got)
	}
	var text []string
	for range 3 {
		if !f.Step() {
			t.Fatal("nothing waited for a step")
		}
		piece, _ := stream.Take()
		text = append(text, piece)
	}
	if strings.Join(text, "|") != "one |two |three" || stream.Err() != nil {
		t.Fatalf("the steps gave %q", text)
	}
	if _, done := stream.Take(); !done {
		t.Fatal("the answer did not end with its last word")
	}
	f.Step()
	if text, done := background.Take(); text != "" || done {
		t.Fatalf("a background answer shows %q before it is whole", text)
	}
	f.Finish()
	if text, done := background.Take(); text != "a b" || !done {
		t.Fatalf("a background answer is %q", text)
	}
	if f.Step() || f.Status().State != model.Ready {
		t.Fatal("the fake is not ready once all is answered")
	}

	ctx, cancel := context.WithCancel(context.Background())
	stream, _ = f.Generate(ctx, say("never ends"))
	cancel()
	if _, err = stream.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled answer ended with %v", err)
	}
	eventually(t, "the fake is ready again", func() bool { return f.Status().State == model.Ready })
}

func TestAFakeFollowsItsStateAndItsSettings(t *testing.T) {
	f := model.NewFake()
	refused := []struct {
		state model.State
		want  error
	}{{model.Off, model.ErrOff}, {model.Missing, model.ErrMissing}, {model.Failed, model.ErrHelper}}
	for _, c := range refused {
		state, want := c.state, c.want
		f.SetState(state, "because")
		if _, err := f.Generate(context.Background(), say("x")); !errors.Is(err, want) {
			t.Errorf("a fake that is %v answers %v", state, err)
		}
		if _, err := f.Count(context.Background(), "x"); !errors.Is(err, want) {
			t.Errorf("a fake that is %v counts: %v", state, err)
		}
		if _, err := f.Embed(context.Background(), model.Texts{Texts: []string{"x"}}); !errors.Is(err, want) {
			t.Errorf("a fake that is %v embeds: %v", state, err)
		}
	}
	f.SetState(model.Ready, "")
	if n, _ := f.Count(context.Background(), "12345678"); n != 2 {
		t.Fatalf("eight bytes are %d tokens", n)
	}
	vectors, err := f.Embed(context.Background(), model.Texts{Texts: []string{"red apples", "green apples", "a bicycle"}})
	if err != nil || len(vectors) != 3 || len(vectors[0]) != f.Status().Capabilities.Dimensions {
		t.Fatalf("%v, %v", vectors, err)
	}
	if near, far := dot(vectors[0], vectors[1]), dot(vectors[0], vectors[2]); near <= far || dot(vectors[0],
		vectors[0]) < 0.99 {
		t.Fatalf("texts sharing a word are %v apart, others %v", near, far)
	}
	f.SetCapabilities(model.Capabilities{Context: 100, Chat: true})
	if _, err = f.Embed(context.Background(), model.Texts{Texts: []string{"x"}}); !errors.Is(err, model.ErrNoEmbeddings) {
		t.Fatalf("a fake that does not embed embeds: %v", err)
	}

	f.Manual = true
	stream, _ := f.Generate(context.Background(), say("held"))
	f.Configure(model.Settings{On: true, ID: "qwen3-4b-q4"})
	if _, err = stream.Wait(); !errors.Is(err, model.ErrChanged) {
		t.Fatalf("switching the model ended the answer with %v", err)
	}
	if got := f.Status(); got.Chosen.ID != "qwen3-4b-q4" || got.State != model.Ready || f.Settings().ID != "qwen3-4b-q4" {
		t.Fatalf("after a switch the fake is %+v", got)
	}
	f.Files = map[string]model.File{
		"/m/chat.gguf": {Name: "Mine", Template: true}, "/m/bare.gguf": {}, "/m/embed.gguf": {Template: true,
			Embedding: true},
	}
	if file, failed := f.Inspect(context.Background(), "/m/chat.gguf"); failed != nil || file.Path != "/m/chat.gguf" {
		t.Fatalf("%+v, %v", file, err)
	}
	for _, path := range []string{"/m/bare.gguf", "/m/embed.gguf", "/m/absent.gguf"} {
		if _, err = f.Inspect(context.Background(), path); err == nil {
			t.Errorf("%s was not refused", path)
		}
	}
	f.Configure(model.Settings{On: true, File: "/m/chat.gguf"})
	if got := f.Status(); got.Chosen != (model.Chosen{File: "/m/chat.gguf", Name: "Mine"}) || got.State != model.Ready {
		t.Fatalf("with a file chosen the fake is %+v", got)
	}
	f.Configure(model.Settings{})
	if got := f.Status(); got.State != model.Off {
		t.Fatalf("turned off the fake is %v", got.State)
	}
	if err = f.Install("qwen3-4b-q4"); !errors.Is(err, model.ErrOff) {
		t.Fatalf("a fake that is off installs: %v", err)
	}
}

func dot(a, b []float32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func TestAFakeInstallsAndDeletes(t *testing.T) {
	f := model.NewFake()
	recommended, ok := f.Recommended()
	if !ok || recommended.Install.State != model.Installed {
		t.Fatalf("a new fake proposes %+v", recommended)
	}
	if err := f.DeleteAll(); err != nil {
		t.Fatal(err)
	}
	if got := f.Status(); got.State != model.Missing {
		t.Fatalf("with nothing installed the fake is %v", got.State)
	}
	f.Manual = true
	if err := f.Install(recommended.ID); err != nil {
		t.Fatal(err)
	}
	f.Advance(recommended.ID, 1000, nil)
	entry := func(id string) model.Entry {
		for _, e := range f.Catalogue() {
			if e.ID == id {
				return e
			}
		}
		t.Fatalf("no %s", id)
		return model.Entry{}
	}
	if got := entry(recommended.ID).Install; got.State != model.Installing || got.Done != 1000 ||
		got.Total != recommended.Size {
		t.Fatalf("a held install is %+v", got)
	}
	f.Advance(recommended.ID, 0, errors.New("the network went away"))
	if got := entry(recommended.ID).Install; got.State != model.InstallFailed || got.Err == "" {
		t.Fatalf("a failed install is %+v", got)
	}
	_ = f.Install(recommended.ID)
	f.CancelInstall(recommended.ID)
	if got := entry(recommended.ID).Install; got.State != model.NotInstalled {
		t.Fatalf("a cancelled install is %+v", got)
	}
	_ = f.Install(recommended.ID)
	f.Advance(recommended.ID, recommended.Size, nil)
	f.SetInstalled(model.RuntimeID, true)
	f.Configure(model.Settings{On: true, GPU: true, ID: recommended.ID})
	if got := f.Status(); got.State != model.Ready {
		t.Fatalf("once installed the fake is %v (%q)", got.State, got.Reason)
	}
	if err := f.Delete(recommended.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.Status(); got.State != model.Missing {
		t.Fatalf("its model deleted, the fake is %v", got.State)
	}
	if f.Delete("unheard-of") == nil || f.Install("unheard-of") == nil {
		t.Fatal("an unknown entry is installed or deleted")
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Generate(context.Background(), say("x")); err == nil {
		t.Fatal("a closed fake answers")
	}
}
