package model_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/model"
)

func own(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAFileOfTheUsersOwnIsReadThroughTheHelperAndUsedWhereItIs(t *testing.T) {
	b := started(t, "ok")
	path := own(t, "chat.gguf", "my own weights")
	file, err := b.Inspect(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	want := model.File{
		Path: path, Size: 14, SHA256: sum([]byte("my own weights")), Name: "A chat model", Architecture: "qwen2",
		Parameters: 1_500_000_000, Context: 32768, Template: true,
	}
	if file != want {
		t.Fatalf("the file was read as %+v", file)
	}
	inspector := b.helpers()[0].Hello
	if inspector.Inspect != path || inspector.Model != "" || inspector.Library == "" {
		t.Fatalf("the helper that read it was told %+v", inspector)
	}

	b.Configure(model.Settings{On: true, File: path})
	got := b.Status()
	if got.State != model.Ready || got.Chosen != (model.Chosen{File: path, Name: "A chat model"}) ||
		got.Capabilities.Context != model.DefaultContext || !got.Capabilities.Chat {
		t.Fatalf("with a file of the user's the model is %v (%q), %+v, %+v", got.State, got.Reason, got.Chosen,
			got.Capabilities)
	}
	stream, err := b.Generate(context.Background(), say("on my own model"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); err != nil {
		t.Fatal(err)
	}
	helpers := b.helpers()
	if hello := helpers[len(helpers)-1].Hello; hello.Model != path || hello.Inspect != "" {
		t.Fatalf("the helper was told %+v", hello)
	}
	for _, kept := range tree(t, b.dir) {
		if strings.Contains(kept, "chat.gguf") {
			t.Fatalf("the user's file was copied to %s", kept)
		}
	}
}

func TestAFileThatChangedSinceItWasChosenIsRefused(t *testing.T) {
	b := started(t, "ok")
	path := own(t, "chat.gguf", "my own weights")
	if _, err := b.Inspect(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	b.Configure(model.Settings{On: true, File: path})

	// The same bytes written again are the same file.
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	stream, err := b.Generate(context.Background(), say("the same file"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); err != nil {
		t.Fatalf("a file only touched is refused: %v", err)
	}

	if err = os.WriteFile(path, []byte("other weights!"), 0o600); err != nil {
		t.Fatal(err)
	}
	b.Configure(model.Settings{On: true, File: path, Threads: 2})
	if stream, err = b.Generate(context.Background(), say("another file")); err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); err == nil || !strings.Contains(err.Error(), "changed since it was chosen") {
		t.Fatalf("a changed file answers: %v", err)
	}
	if got := b.Status(); got.State != model.Failed || !strings.Contains(got.Reason, "changed") {
		t.Fatalf("with a changed file the model is %v (%q)", got.State, got.Reason)
	}
	started := len(b.helpers())
	if _, err = b.Generate(context.Background(), say("again")); err == nil {
		t.Fatal("a changed file is tried again")
	}
	if len(b.helpers()) != started {
		t.Fatal("a helper was started on a changed file")
	}

	// Chosen again, it is the file as it is now.
	if _, err = b.Inspect(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if got := b.Status(); got.State != model.Ready {
		t.Fatalf("chosen again the model is %v (%q)", got.State, got.Reason)
	}
	if stream, err = b.Generate(context.Background(), say("the new file")); err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); err != nil {
		t.Fatal(err)
	}

	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	b.Configure(model.Settings{On: true, File: path})
	if got := b.Status(); got.State != model.Missing {
		t.Fatalf("with its file gone the model is %v", got.State)
	}
}

func TestAFileThatCannotChatIsRefusedWithItsReason(t *testing.T) {
	b := started(t, "ok")
	for name, reason := range map[string]string{
		"bare.gguf": "no chat template", "embed.gguf": "embeds texts", "broken.gguf": "not a GGUF model",
	} {
		path := own(t, name, "weights")
		_, err := b.Inspect(context.Background(), path)
		if err == nil || !strings.Contains(err.Error(), reason) || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: %v", name, err)
		}
		b.Configure(model.Settings{On: true, File: path})
		if got := b.Status(); got.State != model.Failed || !strings.Contains(got.Reason, "choose it again") {
			t.Errorf("with %s chosen the model is %v (%q)", name, got.State, got.Reason)
		}
		if _, err = b.Generate(context.Background(), say("hello")); err == nil {
			t.Errorf("%s answers", name)
		}
	}
	for _, path := range []string{"relative.gguf", filepath.Join(b.dir, "models", "small.gguf"),
		filepath.Join(t.TempDir(), "absent.gguf"), t.TempDir()} {
		if _, err := b.Inspect(context.Background(), path); err == nil {
			t.Errorf("%s was taken for a file of the user's", path)
		}
	}
}

func TestAFileIsNotReadWithoutTheLibrary(t *testing.T) {
	b := on(t)
	path := own(t, "chat.gguf", "weights")
	if _, err := b.Inspect(context.Background(), path); !errors.Is(err, model.ErrMissing) {
		t.Fatalf("with no library a file was read: %v", err)
	}
	if len(b.helpers()) != 0 {
		t.Fatal("a helper was started with no library")
	}
	b.Configure(model.Settings{On: true, File: path})
	if got := b.Status(); got.State != model.Failed {
		t.Fatalf("a file never looked into makes the model %v", got.State)
	}
	b.install(model.RuntimeID)
	if _, err := b.Inspect(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if got := b.Status(); got.State != model.Ready {
		t.Fatalf("a file looked into, with only the library installed, makes the model %v (%q)", got.State, got.Reason)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.Inspect(ctx, own(t, "other.gguf", "weights")); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled look ended with %v", err)
	}
}
