package model_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/model"
)

// tree is every file under dir, by its path from it.
func tree(t *testing.T, dir string) []string {
	t.Helper()
	var names []string
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			names = append(names, filepath.ToSlash(rel))
		}
		return nil
	})
	return names
}

func on(t *testing.T) *bench {
	t.Helper()
	opts, b := options(t, "ok")
	return b.open(opts, model.Settings{On: true})
}

func TestInstallingAModelBringsTheLibraryAndChecksBoth(t *testing.T) {
	b := on(t)
	b.site.gate = make(chan struct{})
	before := b.Status().Seq
	if err := b.Install(small); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the download shows how far it is", func() bool {
		got := b.entry(small).Install
		return got.State == model.Installing && got.Done > 0 && got.Total > got.Done
	})
	if got := b.entry(model.RuntimeID).Install.State; got != model.NotInstalled {
		t.Fatalf("while a model that needs it downloads, the library is %v", got)
	}
	if b.Status().Seq == before {
		t.Fatal("a download does not move the status's Seq")
	}
	close(b.site.gate)
	eventually(t, "the model is installed", func() bool { return b.entry(small).Install.State == model.Installed })
	if got := b.entry(model.RuntimeID).Install.State; got != model.Installed {
		t.Fatalf("the library is %v", got)
	}
	if got := b.Status(); got.State != model.Ready {
		t.Fatalf("once installed the model is %v (%q)", got.State, got.Reason)
	}
	files := tree(t, b.dir)
	system := "b1-" + runtime.GOOS + "-" + runtime.GOARCH
	want := []string{
		"models/small.gguf", "models/small.json",
		"runtime/" + system + ".json",
		"runtime/" + system + "/llama-b1/LICENSE",
		"runtime/" + system + "/llama-b1/libggml-base.dylib",
		"runtime/" + system + "/llama-b1/libggml-metal.dylib",
		"runtime/" + system + "/llama-b1/libggml.dylib",
		"runtime/" + system + "/llama-b1/libllama.0.dylib",
		"runtime/" + system + "/llama-b1/libllama.dylib",
		"runtime/" + system + "/llama-b1/libllama.so",
		"runtime/" + system + "/llama-b1/llama.dll",
	}
	slices.Sort(want)
	slices.Sort(files)
	if !slices.Equal(files, want) {
		t.Fatalf("the model folder holds\n%s\nand not\n%s", strings.Join(files, "\n"), strings.Join(want, "\n"))
	}
	for _, asked := range b.site.asked() {
		if !strings.HasSuffix(asked, " ") {
			t.Fatalf("a first download asked for a part: %q", asked)
		}
	}
	// A second install has nothing to fetch.
	asked := len(b.site.asked())
	b.install(small)
	if len(b.site.asked()) != asked {
		t.Fatal("an installed model was downloaded again")
	}
}

func TestADownloadThatStoppedGoesOnFromWhereItWas(t *testing.T) {
	b := on(t)
	b.install(model.RuntimeID)
	b.site.mu.Lock()
	b.site.cut["/small.gguf"] = 10000
	size := len(b.site.files["/small.gguf"])
	b.site.mu.Unlock()
	if err := b.Install(small); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the download failed", func() bool { return b.entry(small).Install.State == model.InstallFailed })
	if got := b.entry(small).Install; got.Err == "" {
		t.Fatalf("a failed download gives no reason: %+v", got)
	}
	part, err := os.Stat(filepath.Join(b.dir, "downloads", "small.gguf.part"))
	if err != nil || part.Size() == 0 || part.Size() >= int64(size) {
		t.Fatalf("what came of the download: %v, %v", part, err)
	}
	if got := b.Status(); got.State != model.Missing {
		t.Fatalf("with half a model the model is %v", got.State)
	}
	b.install(small)
	asked := b.site.asked()
	if last := asked[len(asked)-1]; !strings.HasPrefix(last, "/small.gguf bytes=") || strings.HasSuffix(last, "=0-") {
		t.Fatalf("the second try asked %q", last)
	}
	data, err := os.ReadFile(filepath.Join(b.dir, "models", "small.gguf"))
	if err != nil || sum(data) != sum(b.site.files["/small.gguf"]) {
		t.Fatalf("the resumed file is not the model: %v", err)
	}
}

func TestAFileThatIsNotTheManifestsIsRefused(t *testing.T) {
	for name, spoil := range map[string]func(s *site){
		"another file of the same size": func(s *site) {
			s.files["/small.gguf"] = bytes.Repeat([]byte("x"), len(s.files["/small.gguf"]))
		},
		"a longer file":  func(s *site) { s.files["/small.gguf"] = append(s.files["/small.gguf"], "more"...) },
		"a shorter file": func(s *site) { s.files["/small.gguf"] = s.files["/small.gguf"][:100] },
		"no file":        func(s *site) { delete(s.files, "/small.gguf") },
	} {
		t.Run(name, func(t *testing.T) {
			b := on(t)
			b.site.mu.Lock()
			spoil(b.site)
			b.site.mu.Unlock()
			if err := b.Install(small); err != nil {
				t.Fatal(err)
			}
			eventually(t, "the download failed", func() bool { return b.entry(small).Install.State == model.InstallFailed })
			for _, file := range tree(t, b.dir) {
				if strings.Contains(file, "small") {
					t.Errorf("a refused file left %s", file)
				}
			}
			if got := b.Status(); got.State != model.Missing {
				t.Fatalf("the model is %v", got.State)
			}
			if _, err := b.Generate(context.Background(), say("hello")); !errors.Is(err, model.ErrMissing) {
				t.Fatalf("a refused model answers: %v", err)
			}
		})
	}
}

func zipped(t *testing.T, names ...string) []byte {
	t.Helper()
	var packed bytes.Buffer
	archive := zip.NewWriter(&packed)
	for _, name := range names {
		w, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte("llama")); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return packed.Bytes()
}

func TestAnArchiveThatWritesOutsideItsFolderIsRefused(t *testing.T) {
	good := []entry{{name: "bin/libllama.dylib", body: "llama"}, {name: "bin/libllama.so", body: "llama"},
		{name: "bin/llama.dll", body: "llama"}}
	archives := map[string][]byte{
		"a path that climbs out": tarball(t, append(good, entry{name: "../../escaped.dylib", body: "out"})...),
		"a path from the root":   tarball(t, append(good, entry{name: "/tmp/escaped.dylib", body: "out"})...),
		"a link out": tarball(t, append(good, entry{name: "bin/libggml.dylib",
			link: "../../../escaped.dylib"})...),
		"a link to the root": tarball(t, append(good, entry{name: "bin/libggml.dylib", link: "/etc/passwd"})...),
		"a file through a link": tarball(t, append(good, entry{name: "bin/out.dylib", link: ".."},
			entry{name: "bin/out.dylib/escaped.dylib", body: "out"})...),
		"a device":                   tarball(t, append(good, entry{name: "bin/libdev.dylib", kind: tar.TypeChar})...),
		"a hard link":                tarball(t, append(good, entry{name: "bin/libhard.dylib", kind: tar.TypeLink})...),
		"no library":                 tarball(t, entry{name: "bin/llama-server", body: "a program"}),
		"not an archive":             []byte("this is no archive"),
		"a zip that climbs out.zip":  zipped(t, "llama.dll", "../escaped.dll"),
		"a zip with backslashes.zip": zipped(t, "llama.dll", `..\escaped.dll`),
	}
	reasons := map[string]string{
		"a path that climbs out": "outside it", "a path from the root": "outside it", "a link out": "points outside",
		"a link to the root": "points outside", "a file through a link": "points outside",
		"a device": "neither a file", "a hard link": "neither a file", "no library": "no llama.cpp library",
		"not an archive": "gzip", "a zip that climbs out.zip": "outside it", "a zip with backslashes.zip": "outside it",
	}
	for name, archive := range archives {
		t.Run(name, func(t *testing.T) {
			opts, b := options(t, "ok")
			served := "/llama.tar.gz"
			if strings.HasSuffix(name, ".zip") {
				served = "/llama.zip"
			}
			b.site.mu.Lock()
			b.site.files[served] = archive
			a := &opts.Manifest.Runtime.Archives[0]
			a.URL, a.Size, a.SHA256 = b.site.URL+served, int64(len(archive)), sum(archive)
			b.site.mu.Unlock()
			b.open(opts, model.Settings{On: true})
			if err := b.Install(model.RuntimeID); err != nil {
				t.Fatal(err)
			}
			eventually(t, "the install failed", func() bool {
				return b.entry(model.RuntimeID).Install.State == model.InstallFailed
			})
			if reason := b.entry(model.RuntimeID).Install.Err; !strings.Contains(reason, reasons[name]) {
				t.Fatalf("it was refused because %q, not because of %q", reason, reasons[name])
			}
			if files := tree(t, filepath.Dir(b.dir)); len(files) != 0 {
				t.Fatalf("a refused archive left %q (%s)", files, b.entry(model.RuntimeID).Install.Err)
			}
		})
	}
}

func TestAZipOfLibrariesIsUnpacked(t *testing.T) {
	opts, b := options(t, "ok")
	archive := zipped(t, "llama.dll", "ggml.dll", "ggml-base.dll", "ggml-cpu-haswell.dll", "llama-server.exe",
		"libllama.dylib", "libllama.so")
	b.site.files["/llama.zip"] = archive
	a := &opts.Manifest.Runtime.Archives[0]
	a.URL, a.Size, a.SHA256 = b.site.URL+"/llama.zip", int64(len(archive)), sum(archive)
	b.open(opts, model.Settings{On: true})
	if err := b.Install(model.RuntimeID); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the library is installed", func() bool {
		got := b.entry(model.RuntimeID).Install
		if got.State == model.InstallFailed {
			t.Fatal(got.Err)
		}
		return got.State == model.Installed
	})
	for _, file := range tree(t, b.dir) {
		if strings.HasSuffix(file, ".exe") {
			t.Fatalf("a program of the archive was unpacked: %s", file)
		}
	}
	if n := len(tree(t, b.dir)); n != 7 {
		t.Fatalf("%d files were kept: %q", n, tree(t, b.dir))
	}
}

func TestADownloadIsCancelledAndWhatCameIsKept(t *testing.T) {
	before := runtime.NumGoroutine()
	b := on(t)
	b.install(model.RuntimeID)
	b.site.gate = make(chan struct{})
	if err := b.Install(small); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the download began", func() bool { return b.entry(small).Install.Done > 0 })
	b.CancelInstall(small)
	eventually(t, "the download stopped", func() bool { return b.entry(small).Install.State == model.NotInstalled })
	if got := b.entry(small).Install; got.Err != "" {
		t.Fatalf("a cancelled download is a failure: %q", got.Err)
	}
	if _, err := os.Stat(filepath.Join(b.dir, "downloads", "small.gguf.part")); err != nil {
		t.Fatalf("what came was not kept: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Install(small); !errors.Is(err, model.ErrClosed) {
		t.Fatalf("a closed model downloads: %v", err)
	}
	close(b.site.gate)
	b.site.CloseClientConnections()
	b.site.Client().CloseIdleConnections()
	eventually(t, "its goroutines ended", func() bool { return runtime.NumGoroutine() <= before+2 })
}

func TestClosingStopsADownload(t *testing.T) {
	b := on(t)
	b.site.gate = make(chan struct{})
	defer close(b.site.gate)
	if err := b.Install(small); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the download began", func() bool { return b.entry(small).Install.Done > 0 })
	closed := make(chan error, 1)
	go func() { closed <- b.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close waits for a download to end")
	}
}

func TestEachInstalledEntryIsDeletedAloneAndEverythingAtOnce(t *testing.T) {
	b := started(t, "ok")
	b.install(big)
	own := filepath.Join(t.TempDir(), "chat.gguf")
	if err := os.WriteFile(own, []byte("my own model"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Inspect(context.Background(), own); err != nil {
		t.Fatal(err)
	}
	stream, err := b.Generate(context.Background(), say("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Wait(); err != nil {
		t.Fatal(err)
	}
	pid := b.helpers()[len(b.helpers())-1].PID

	if err = b.Delete(big); err != nil {
		t.Fatal(err)
	}
	if b.entry(big).Install.State != model.NotInstalled || b.entry(small).Install.State != model.Installed {
		t.Fatal("deleting one model touched another")
	}
	if alive(pid) {
		t.Fatal("the helper runs while a model is deleted")
	}
	if got := b.Status(); got.State != model.Ready {
		t.Fatalf("with its model still there the model is %v", got.State)
	}
	if err = b.Delete(small); err != nil {
		t.Fatal(err)
	}
	if got := b.Status(); got.State != model.Missing {
		t.Fatalf("with its model deleted the model is %v", got.State)
	}
	if err = b.Delete("unheard-of"); err == nil {
		t.Fatal("an unknown model was deleted")
	}

	b.install(small)
	if err = b.DeleteAll(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(b.dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the model folder is still there: %v, %q", err, tree(t, b.dir))
	}
	if data, failed := os.ReadFile(own); failed != nil || string(data) != "my own model" {
		t.Fatalf("the user's own file was touched: %q, %v", data, err)
	}
	for _, e := range b.Catalogue() {
		if e.Install.State != model.NotInstalled {
			t.Fatalf("%s is %v after everything was deleted", e.ID, e.Install.State)
		}
	}
	if got := b.Status(); got.State != model.Missing {
		t.Fatalf("with everything deleted the model is %v", got.State)
	}
}

func TestDownloadsGoOverHTTPSOnly(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("weights"))
	}))
	defer plain.Close()
	for name, address := range map[string]string{"an http address": plain.URL + "/small.gguf",
		"a redirect to http": "/moved"} {
		t.Run(name, func(t *testing.T) {
			opts, b := options(t, "ok")
			moved := httptest.NewTLSServer(http.RedirectHandler(plain.URL+"/small.gguf", http.StatusFound))
			defer moved.Close()
			if address == "/moved" {
				address = moved.URL + address
				opts.Transport = moved.Client().Transport
			}
			opts.Manifest.Models[0].URL = address
			opts.Manifest.Models[0].Size, opts.Manifest.Models[0].SHA256 = 7, sum([]byte("weights"))
			opts.Manifest.Runtime.Archives = nil
			b.open(opts, model.Settings{On: true})
			if err := b.Install(small); err == nil {
				t.Fatal("a model was installed on a system the manifest has no library for")
			}
			opts2, c := options(t, "ok")
			opts2.Manifest.Models[0] = opts.Manifest.Models[0]
			opts2.Transport = opts.Transport
			c.open(opts2, model.Settings{On: true})
			if address != plain.URL+"/small.gguf" {
				c.site.mu.Lock()
				c.site.files = map[string][]byte{}
				c.site.mu.Unlock()
				opts2.Manifest.Runtime.Archives[0].URL = address
			}
			if err := c.Install(small); err != nil {
				t.Fatal(err)
			}
			eventually(t, "the download failed", func() bool { return c.entry(small).Install.State == model.InstallFailed })
			if reason := c.entry(small).Install.Err; !strings.Contains(reason, "https") && !strings.Contains(reason, "404") {
				t.Fatalf("it failed because %q", reason)
			}
		})
	}
}
