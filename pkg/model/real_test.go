package model_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/model"
)

// TestTheHelpersProtocolIsLights keeps the helper's copy of the
// protocol, in its own module, the same as light's.
func TestTheHelpersProtocolIsLights(t *testing.T) {
	mine, err := os.ReadFile("wire.go")
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := os.ReadFile(filepath.Join("..", "..", "helper", "model", "internal", "wire", "wire.go"))
	if err != nil {
		t.Fatal(err)
	}
	_, mineBody, _ := strings.Cut(string(mine), "\n")
	first, theirBody, _ := strings.Cut(string(theirs), "\n")
	if first != "package wire" || mineBody != theirBody {
		t.Fatal("helper/model/internal/wire/wire.go is not pkg/model/wire.go under another package name: copy it again")
	}
}

// TestARealHelperAnswers runs light's side against a built light-model
// and a real model, when LIGHT_MODEL_HELPER and LIGHT_MODEL_DEV name
// them on this machine. Nothing is downloaded.
func TestARealHelperAnswers(t *testing.T) {
	helper, value := os.Getenv(model.EnvHelper), os.Getenv(model.EnvDev)
	if helper == "" || value == "" {
		t.Skipf("%s and %s name no helper and no model on this machine", model.EnvHelper, model.EnvDev)
	}
	dev, err := model.ParseDev(value)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "model")
	l := model.Open(model.Options{Dir: dir, Helper: helper, Build: "dev", Dev: dev}, model.Settings{On: true, GPU: true})
	defer func() {
		if closing := l.Close(); closing != nil {
			t.Error(closing)
		}
	}()
	at := time.Now()
	l.Warm()
	var seen []string
	for !l.Status().Loaded {
		s := l.Status()
		if s.State == model.Failed {
			t.Fatalf("the model failed: %s", s.Reason)
		}
		if s.State == model.Preparing && s.Detail != "" && (len(seen) == 0 || seen[len(seen)-1] != s.Detail) {
			seen = append(seen, s.Detail)
		}
		if time.Since(at) > 2*time.Minute {
			t.Fatalf("the model is still %v", s.State)
		}
		time.Sleep(time.Millisecond)
	}
	status := l.Status()
	t.Logf("ready %v after Warm (said %q): %+v, memory %d MB", time.Since(at).Round(time.Millisecond), seen,
		status.Capabilities, status.Memory>>20)
	if !status.Capabilities.Chat || status.Capabilities.Context != model.DefaultContext {
		t.Fatalf("the model can %+v", status.Capabilities)
	}

	stream, err := l.Generate(t.Context(), model.Request{
		System: "You answer in one short sentence.", Messages: []model.Message{{Role: model.User,
			Text: "What is the capital of France?"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Wait()
	if err != nil || !strings.Contains(result.Text, "Paris") {
		t.Fatalf("the answer is %q (%v)", result.Text, err)
	}
	t.Logf("%q: first token after %v, %d tokens in %v", result.Text, result.FirstToken, result.Tokens, result.Total)

	ctx, cancel := context.WithCancel(t.Context())
	stream, err = l.Generate(ctx, say("Count from 1 to 300, one number a line."))
	if err != nil {
		t.Fatal(err)
	}
	for text := ""; !strings.Contains(text, "5"); {
		piece, _ := stream.Take()
		text += piece
		time.Sleep(time.Millisecond)
	}
	cancelled := time.Now()
	cancel()
	<-stream.Done()
	t.Logf("cancelled while writing: the stream ended %v later", time.Since(cancelled).Round(10*time.Microsecond))
	if stream.Err() == nil {
		t.Fatal("a cancelled stream ended without an error")
	}
	n, err := l.Count(t.Context(), "The quick brown fox jumps over the lazy dog.")
	if err != nil || n < 8 || n > 14 {
		t.Fatalf("nine words are %d tokens (%v)", n, err)
	}
	stream, err = l.Generate(t.Context(), model.Request{
		Messages: []model.Message{{Role: model.User, Text: "Is a page selling shoes news, a shop or a recipe?"}},
		Grammar:  model.Choice("news", "shop", "recipe"), MaxTokens: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result, err = stream.Wait(); err != nil || result.Text != "shop" {
		t.Fatalf("after a cancel, kept to three words, the answer is %q (%v)", result.Text, err)
	}

	stream, err = l.Generate(t.Context(), model.Request{
		Messages: []model.Message{{Role: model.User, Text: "Give the name and the population of Paris as a JSON object."}},
		Grammar:  model.JSON, MaxTokens: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result, err = stream.Wait(); err != nil || result.Stopped != model.StoppedStop || !json.Valid([]byte(result.Text)) {
		t.Fatalf("kept to JSON the answer is %q, ended as %q (%v)", result.Text, result.Stopped, err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var wrote []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			wrote = append(wrote, rel)
		}
		return nil
	})
	t.Logf("%d entries in the model folder; files: %q", len(entries), wrote)
}
