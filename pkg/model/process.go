package model

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	// longestLine is the longest line the helper may write.
	longestLine = 32 << 20
	// logLimit is the size the helper's log is started over at.
	logLimit = 1 << 20
	// tailKept is how much of what the helper last wrote on its
	// standard error is kept for a reason.
	tailKept = 600
)

// process is one run of the helper: its pipes, and the events it
// writes, read on a goroutine that ends with it.
type process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	events chan Event
	tail   *tail
	log    *os.File

	// err is why the helper ended, set before events is closed.
	err error
}

// tail keeps the last bytes written to it and passes them all on.
type tail struct {
	mu   sync.Mutex
	last []byte
	to   io.Writer
}

func (t *tail) Write(data []byte) (int, error) {
	t.mu.Lock()
	t.last = append(t.last, data...)
	if len(t.last) > tailKept {
		t.last = t.last[len(t.last)-tailKept:]
	}
	t.mu.Unlock()
	if t.to != nil {
		_, _ = t.to.Write(data)
	}
	return len(data), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.last))
}

// environment is all the helper is given of light's environment: a home
// of its own in the model folder, where the one thing it writes goes
// (the libffi library its binding to llama.cpp unpacks to load), and on
// Windows the folders the system needs to start a program.
func environment(home string, more []string) []string {
	env := []string{
		"HOME=" + home, "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "TMPDIR=" + filepath.Join(home, "tmp"),
		"LocalAppData=" + home, "USERPROFILE=" + home,
	}
	if runtime.GOOS == "windows" {
		for _, name := range []string{"SystemRoot", "SystemDrive", "windir"} {
			if value := os.Getenv(name); value != "" {
				env = append(env, name+"="+value)
			}
		}
	}
	return append(env, more...)
}

// start runs the helper at path with home as its home and folder, its
// standard error going to logPath.
func start(path, home, logPath string, more []string) (*process, error) {
	for _, dir := range []string{home, filepath.Join(home, "tmp")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("making the helper's folder: %w", err)
		}
	}
	if info, err := os.Stat(logPath); err == nil && info.Size() > logLimit {
		_ = os.Remove(logPath)
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening the helper's log: %w", err)
	}
	//nolint:noctx // the helper is light's own program, found beside it; it is stopped by its pipe, not a context
	cmd := exec.Command(path)
	cmd.Env, cmd.Dir = environment(home, more), home
	p := &process{cmd: cmd, events: make(chan Event, 64), tail: &tail{to: log}, log: log}
	cmd.Stderr = p.tail
	if p.stdin, err = cmd.StdinPipe(); err != nil {
		_ = log.Close()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = log.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		_ = log.Close()
		return nil, fmt.Errorf("starting %s: %w", filepath.Base(path), err)
	}
	go p.read(stdout)
	return p, nil
}

// read passes on what the helper writes until it ends or writes what is
// no event, then reaps it.
func (p *process) read(stdout io.Reader) {
	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 64<<10), longestLine)
	var garbled error
	for lines.Scan() {
		var ev Event
		if err := json.Unmarshal(lines.Bytes(), &ev); err != nil || ev.Event == "" {
			garbled = errors.New("it wrote what is no answer")
			break
		}
		p.events <- ev
	}
	if garbled != nil || lines.Err() != nil {
		_ = p.cmd.Process.Kill()
		_, _ = io.Copy(io.Discard, stdout)
	}
	err := p.cmd.Wait()
	_ = p.log.Close()
	switch {
	case garbled != nil:
		p.err = garbled
	case err != nil:
		p.err = err
	default:
		p.err = errors.New("it left")
	}
	if said := p.tail.String(); said != "" {
		p.err = fmt.Errorf("%w: %s", p.err, lastLine(said))
	}
	close(p.events)
}

func lastLine(text string) string {
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		return text[i+1:]
	}
	return text
}

func (p *process) send(c Command) error {
	line, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = p.stdin.Write(append(line, '\n'))
	return err
}

// stop ends the helper: its input is closed, which it leaves at, and it
// is killed when it has not left after grace. It returns once the
// helper is gone and its reader ended.
func (p *process) stop(grace time.Duration) {
	_ = p.stdin.Close()
	timer := time.NewTimer(grace)
	defer timer.Stop()
	for {
		select {
		case _, open := <-p.events:
			if !open {
				return
			}
		case <-timer.C:
			p.kill()
			return
		}
	}
}

// kill ends the helper at once and waits for its reader.
func (p *process) kill() {
	_ = p.stdin.Close()
	_ = p.cmd.Process.Kill()
	for open := true; open; {
		_, open = <-p.events
	}
}

// inspect asks a helper of its own what the GGUF file at path is.
func inspect(ctx context.Context, helper, home, logPath string, more []string, hello Command) (Event, error) {
	p, err := start(helper, home, logPath, more)
	if err != nil {
		return Event{}, err
	}
	defer p.kill()
	if err = p.send(hello); err != nil {
		return Event{}, fmt.Errorf("%w: %w", ErrHelper, err)
	}
	for {
		select {
		case ev, open := <-p.events:
			switch {
			case !open:
				return Event{}, fmt.Errorf("%w: %w", ErrHelper, p.err)
			case ev.Event == EventHello && ev.Protocol != Protocol:
				return Event{}, protocolError(ev.Protocol)
			case ev.Event == EventError:
				return Event{}, errors.New(ev.Error)
			case ev.Event == EventFile:
				return ev, nil
			}
		case <-ctx.Done():
			return Event{}, ctx.Err()
		}
	}
}

func protocolError(theirs int) error {
	return fmt.Errorf("%w: light-model speaks protocol %d and light protocol %d: they are not of the same release",
		ErrHelper, theirs, Protocol)
}
