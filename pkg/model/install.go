package model

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// userAgent is all a download says of who asks.
const userAgent = "light"

// part is one file an install downloads.
type part struct {
	url, sha256 string
	size        int64
	// name is its name while it downloads.
	name string
	// land puts the checked file where it is used.
	land func(checked string) error
}

// download is one install that runs.
type download struct {
	cancel      context.CancelFunc
	done, total atomic.Int64
	ended       chan struct{}
}

// installer downloads what the manifest names into the model folder,
// each install on a goroutine of its own that ends with it. Only the
// manifest's addresses are asked, over https, with no cookie and no
// referrer; a file is used only once its size and SHA-256 are the
// manifest's.
type installer struct {
	files    files
	manifest Manifest
	client   *http.Client
	// changed is called, from any goroutine, when an install moved, and
	// settled when one ended.
	changed, settled func()

	mu      sync.Mutex
	active  map[string]*download
	failed  map[string]string
	closed  bool
	running sync.WaitGroup
}

func newInstaller(f files, m Manifest, transport http.RoundTripper, changed, settled func()) *installer {
	if transport == nil {
		transport = http.DefaultTransport
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != secure || len(via) > 5 {
			return fmt.Errorf("it redirects to %s, and a download follows a few redirects, to https only", req.URL.Host)
		}
		return nil
	}}
	return &installer{
		files: f, manifest: m, client: client, changed: changed, settled: settled,
		active: map[string]*download{}, failed: map[string]string{},
	}
}

// state is how far the entry of that id is from being installed.
func (in *installer) state(id string) Install {
	in.mu.Lock()
	defer in.mu.Unlock()
	if d := in.active[id]; d != nil {
		return Install{State: Installing, Done: d.done.Load(), Total: d.total.Load()}
	}
	if in.installed(id) {
		return Install{State: Installed}
	}
	if reason := in.failed[id]; reason != "" {
		return Install{State: InstallFailed, Err: reason}
	}
	return Install{}
}

func (in *installer) installed(id string) bool {
	if id == RuntimeID {
		_, _, ok := in.files.runtimePaths(in.manifest.Runtime)
		return ok
	}
	d, known := in.manifest.Model(id)
	if !known {
		return false
	}
	_, ok := in.files.modelPath(d)
	return ok
}

// parts is what installing id still has to download: the library when
// it is not there, then the model.
func (in *installer) parts(id string) ([]part, error) {
	var parts []part
	if !in.installed(RuntimeID) {
		archive, ok := in.manifest.Archive(runtime.GOOS, runtime.GOARCH)
		if !ok {
			return nil, fmt.Errorf("light has no llama.cpp for %s on %s", runtime.GOOS, runtime.GOARCH)
		}
		parts = append(parts, part{
			url: archive.URL, sha256: archive.SHA256, size: archive.Size, name: path.Base(archive.URL),
			land: in.landRuntime,
		})
	}
	if id == RuntimeID {
		return parts, nil
	}
	d, known := in.manifest.Model(id)
	if !known {
		return nil, fmt.Errorf("light knows no model %q", id)
	}
	if !in.installed(id) {
		parts = append(parts, part{
			url: d.URL, sha256: d.SHA256, size: d.Size, name: d.File,
			land: func(checked string) error { return in.landModel(d, checked) },
		})
	}
	return parts, nil
}

// install starts downloading the entry of that id, unless it is being
// downloaded or is there already.
func (in *installer) install(id string) error {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.closed {
		return ErrClosed
	}
	if in.active[id] != nil {
		return nil
	}
	parts, err := in.parts(id)
	if err != nil || len(parts) == 0 {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &download{cancel: cancel, ended: make(chan struct{})}
	for _, p := range parts {
		d.total.Add(p.size)
	}
	in.active[id] = d
	delete(in.failed, id)
	in.running.Go(func() {
		failure := in.run(ctx, d, parts)
		cancel()
		in.mu.Lock()
		delete(in.active, id)
		if failure != nil && !errors.Is(failure, context.Canceled) {
			in.failed[id] = failure.Error()
		}
		in.mu.Unlock()
		close(d.ended)
		in.settled()
	})
	return nil
}

func (in *installer) run(ctx context.Context, d *download, parts []part) error {
	if err := os.MkdirAll(in.files.downloads(), 0o700); err != nil {
		return err
	}
	var before int64
	for _, p := range parts {
		partial := filepath.Join(in.files.downloads(), p.name+".part")
		err := in.fetch(ctx, p, partial, func(came int64) {
			d.done.Store(before + came)
			in.changed()
		})
		if err != nil {
			return err
		}
		if err = p.land(partial); err != nil {
			return err
		}
		before += p.size
		d.done.Store(before)
		in.changed()
	}
	return nil
}

// fetch downloads p into the file at partial, going on from what an
// earlier try left there, and checks its size and digest: a file that
// is not the manifest's is removed and an error.
func (in *installer) fetch(ctx context.Context, p part, partial string, progress func(int64)) error {
	have := int64(0)
	if info, err := os.Stat(partial); err == nil {
		have = info.Size()
	}
	if have > p.size {
		_ = os.Remove(partial)
		have = 0
	}
	if have < p.size {
		if err := in.get(ctx, p, partial, have, progress); err != nil {
			return err
		}
	}
	sum, size, err := digest(partial)
	if err != nil {
		return err
	}
	if size != p.size || sum != p.sha256 {
		_ = os.Remove(partial)
		return fmt.Errorf("%s is not the file light expects (SHA-256 %s, %d bytes): it was removed", p.name, sum, size)
	}
	return nil
}

func (in *installer) get(ctx context.Context, p part, partial string, have int64, progress func(int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return err
	}
	if req.URL.Scheme != secure {
		return fmt.Errorf("%s is not downloaded over https", p.name)
	}
	req.Header.Set("User-Agent", userAgent)
	if have > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(have, 10)+"-")
	}
	resp, err := in.client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", p.name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	flags := os.O_CREATE | os.O_WRONLY
	switch {
	case resp.StatusCode == http.StatusPartialContent && have > 0 &&
		strings.HasPrefix(resp.Header.Get("Content-Range"), "bytes "+strconv.FormatInt(have, 10)+"-"):
		flags |= os.O_APPEND
	case resp.StatusCode == http.StatusOK:
		// The server sends the whole file again.
		have = 0
		flags |= os.O_TRUNC
	default:
		_ = os.Remove(partial)
		return fmt.Errorf("downloading %s: the server answered %s", p.name, resp.Status)
	}
	if resp.ContentLength >= 0 && have+resp.ContentLength != p.size {
		return fmt.Errorf("%s is %d bytes there, and light expects %d", p.name, have+resp.ContentLength, p.size)
	}
	file, err := os.OpenFile(partial, flags, 0o600)
	if err != nil {
		return err
	}
	err = copyTo(file, resp.Body, have, p.size, progress)
	if closed := file.Close(); err == nil {
		err = closed
	}
	if err != nil {
		return fmt.Errorf("downloading %s: %w", p.name, err)
	}
	return nil
}

// copyTo appends body to file, which holds have bytes, never past size.
func copyTo(file io.Writer, body io.Reader, have, size int64, progress func(int64)) error {
	buffer := make([]byte, 256<<10)
	for {
		n, err := body.Read(buffer)
		if have+int64(n) > size {
			return errors.New("the server sends more than the file is")
		}
		if n > 0 {
			if _, werr := file.Write(buffer[:n]); werr != nil {
				return werr
			}
			have += int64(n)
			progress(have)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (in *installer) landModel(d Download, checked string) error {
	if err := os.MkdirAll(filepath.Dir(in.files.model(d)), 0o700); err != nil {
		return err
	}
	if err := os.Rename(checked, in.files.model(d)); err != nil {
		return err
	}
	return writeJSON(in.files.modelRecord(d), installed{SHA256: d.SHA256, Size: d.Size})
}

func (in *installer) landRuntime(checked string) error {
	final := in.files.runtimeDir(in.manifest.Runtime)
	draft := final + ".unpacking"
	if err := os.RemoveAll(draft); err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(draft) }()
	if err := unpack(checked, draft); err != nil {
		_ = os.Remove(checked)
		return err
	}
	where, err := place(draft)
	if err != nil {
		_ = os.Remove(checked)
		return err
	}
	if err = os.RemoveAll(final); err != nil {
		return err
	}
	if err = os.Rename(draft, final); err != nil {
		return err
	}
	if err = writeJSON(in.files.runtimeRecord(in.manifest.Runtime), where); err != nil {
		return err
	}
	return os.Remove(checked)
}

// cancel stops the install of that id and waits for nothing: what came
// stays for the next.
func (in *installer) cancel(id string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if d := in.active[id]; d != nil {
		d.cancel()
	}
}

// remove deletes the installed entry of that id and what was
// downloaded of it.
func (in *installer) remove(id string) error {
	in.mu.Lock()
	running := in.active[id]
	delete(in.failed, id)
	in.mu.Unlock()
	if running != nil {
		running.cancel()
		<-running.ended
	}
	if id == RuntimeID {
		r := in.manifest.Runtime
		_ = os.Remove(in.files.runtimeRecord(r))
		if archive, ok := in.manifest.Archive(runtime.GOOS, runtime.GOARCH); ok {
			_ = os.Remove(filepath.Join(in.files.downloads(), path.Base(archive.URL)+".part"))
		}
		return os.RemoveAll(in.files.runtimeDir(r))
	}
	d, known := in.manifest.Model(id)
	if !known {
		return fmt.Errorf("light knows no model %q", id)
	}
	_ = os.Remove(in.files.modelRecord(d))
	_ = os.Remove(filepath.Join(in.files.downloads(), d.File+".part"))
	if err := os.Remove(in.files.model(d)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// stop cancels every install and waits for them to end.
func (in *installer) stop() {
	in.mu.Lock()
	for _, d := range in.active {
		d.cancel()
	}
	in.mu.Unlock()
	in.running.Wait()
}

// forget drops what is remembered of failed installs.
func (in *installer) forget() {
	in.mu.Lock()
	clear(in.failed)
	in.mu.Unlock()
}

func (in *installer) close() {
	in.mu.Lock()
	in.closed = true
	in.mu.Unlock()
	in.stop()
}
