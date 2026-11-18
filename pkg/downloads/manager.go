package downloads

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/lantervale/light/pkg/content"
	"github.com/lantervale/light/pkg/fetch"
)

const (
	// partial ends the name of a file still being written.
	partial = ".lightdownload"
	// sampled is the least time a speed is measured over.
	sampled = 500 * time.Millisecond
	// chunk is how much is read at a time.
	chunk = 32 << 10
)

var errNoFolder = errors.New("light was given no folder to save to")

// Opener asks for an address and gives its answer still to read, as a
// fetch.Client does.
type Opener interface {
	Open(ctx context.Context, address, accept string) (*fetch.Stream, error)
}

// Manager runs the downloads into one folder, each on a goroutine of its
// own that ends with it. It is safe for use from several goroutines.
type Manager struct {
	dir    string
	client Opener

	mu      sync.Mutex
	items   []*Item
	cancels map[int]context.CancelFunc
	next    int
	version uint64
	closed  bool
	running sync.WaitGroup
}

// New is a manager saving into dir what it gets through client.
func New(dir string, client Opener) *Manager {
	return &Manager{dir: dir, client: client, cancels: map[int]context.CancelFunc{}, next: 1}
}

// Restore puts back the items of an earlier run, newest first, before
// any download is started.
func (m *Manager) Restore(items []Item) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range slices.Backward(items) {
		m.items = append(m.items, &item)
		m.next = max(m.next, item.ID+1)
	}
	m.version++
}

// Start downloads address, under name when one is given, else the name
// its answer says. It goes on when ctx ends, which only says where its
// request is recorded: Cancel and Close end it. It gives the download's
// number, 0 from a closed manager.
func (m *Manager) Start(ctx context.Context, address, name string) int {
	return m.add(ctx, &Item{Address: address, Name: name, suggested: name}, m.fetch)
}

// Write saves data, got from address, under name, as a download that
// has nothing left to receive.
func (m *Manager) Write(address, name, media string, data []byte) int {
	item := &Item{Address: address, Name: name, suggested: name, Media: media}
	return m.add(context.Background(), item, func(_ context.Context, id int) error {
		return m.receive(id, name, media, int64(len(data)), bytes.NewReader(data))
	})
}

// Retry starts again a download that failed or was cancelled.
func (m *Manager) Retry(ctx context.Context, id int) {
	m.mu.Lock()
	item := m.find(id)
	if item == nil || item.State == Active || item.State == Done || m.closed {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	m.run(ctx, item, m.fetch)
}

// add lists item and runs it.
func (m *Manager) add(ctx context.Context, item *Item, work func(context.Context, int) error) int {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return 0
	}
	item.ID = m.next
	m.next++
	m.items = append(m.items, item)
	m.mu.Unlock()
	m.run(ctx, item, work)
	return item.ID
}

// run does work for item on a goroutine of its own, and ends item with
// what work ends with.
func (m *Manager) run(ctx context.Context, item *Item, work func(context.Context, int) error) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	m.mu.Lock()
	item.State, item.Err, item.Received, item.Total, item.Speed = Active, "", 0, -1, 0
	item.Started, item.Ended, item.Path = time.Now(), time.Time{}, ""
	m.cancels[item.ID] = cancel
	m.version++
	m.mu.Unlock()
	m.running.Go(func() {
		defer cancel()
		err := work(ctx, item.ID)
		m.change(item.ID, func(i *Item) {
			i.Ended, i.Speed = time.Now(), 0
			switch {
			case err == nil:
				i.State = Done
			case ctx.Err() != nil:
				i.State = Cancelled
			default:
				i.State, i.Err = Failed, err.Error()
			}
		})
		m.mu.Lock()
		delete(m.cancels, item.ID)
		m.mu.Unlock()
	})
}

// SetDir changes the folder, for the downloads that begin after it.
func (m *Manager) SetDir(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dir = dir
}

func (m *Manager) folder() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dir
}

// fetch gets download id's address and receives its answer.
func (m *Manager) fetch(ctx context.Context, id int) error {
	item, _ := m.Item(id)
	if m.folder() == "" {
		return errNoFolder
	}
	stream, err := m.client.Open(ctx, item.Address, "*/*")
	if err != nil {
		return err
	}
	defer stream.Close()
	if !stream.OK() {
		return fmt.Errorf("the server answered %d %s", stream.Status, http.StatusText(stream.Status))
	}
	_, media := content.Of(stream.Header.Get("Content-Type"), stream.Peek(512), stream.Address)
	disposition := stream.Header.Get("Content-Disposition")
	if item.suggested != "" {
		disposition = mime.FormatMediaType("attachment", map[string]string{"filename": item.suggested})
	}
	return m.receive(id, content.SaveName(disposition, stream.Address, media), media, stream.Length, stream)
}

// receive writes what from gives to a temporary file of the folder, and
// names it name, or the first free name after it, once it is whole. What
// is not whole is removed.
func (m *Manager) receive(id int, name, media string, total int64, from io.Reader) error {
	dir := m.folder()
	if dir == "" {
		return errNoFolder
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("saving %s: %w", name, err)
	}
	file, name, err := create(dir, name)
	if err != nil {
		return fmt.Errorf("saving %s: %w", name, err)
	}
	m.change(id, func(i *Item) { i.Name, i.Media, i.Total = name, media, total })
	err = m.copy(id, file, from)
	if closed := file.Close(); err == nil {
		err = closed
	}
	if err == nil {
		err = m.finish(id, dir, file.Name(), name)
	}
	if err != nil {
		return errors.Join(err, os.Remove(file.Name()))
	}
	return nil
}

// create makes the temporary file of the first free name after name: one
// no file of the folder has, whole or still being written.
func create(dir, name string) (*os.File, string, error) {
	for {
		free := content.FreeName(name, func(name string) bool {
			return exists(dir, name) || exists(dir, name+partial)
		})
		file, err := os.OpenFile(filepath.Join(dir, free+partial), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return file, free, err
	}
}

func exists(dir, name string) bool {
	_, err := os.Lstat(filepath.Join(dir, name))
	return !errors.Is(err, fs.ErrNotExist)
}

// copy writes from into file, counting the bytes and their speed.
func (m *Manager) copy(id int, file io.Writer, from io.Reader) error {
	buffer := make([]byte, chunk)
	mark, since := time.Now(), int64(0)
	for {
		n, err := from.Read(buffer)
		if n > 0 {
			if _, failed := file.Write(buffer[:n]); failed != nil {
				return failed
			}
			since += int64(n)
			elapsed := time.Since(mark)
			m.change(id, func(i *Item) {
				i.Received += int64(n)
				if elapsed >= sampled {
					i.Speed = float64(since) / elapsed.Seconds()
				}
			})
			if elapsed >= sampled {
				mark, since = time.Now(), 0
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// finish gives the whole file at written its name, never over another:
// a file that took the name meanwhile leaves it the next free one.
func (m *Manager) finish(id int, dir, written, name string) error {
	for {
		free := content.FreeName(name, func(name string) bool { return exists(dir, name) })
		path := filepath.Join(dir, free)
		err := os.Link(written, path)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			err = os.Rename(written, path)
		} else {
			err = os.Remove(written)
		}
		if err == nil {
			m.change(id, func(i *Item) { i.Name, i.Path = free, path })
		}
		return err
	}
}

// Cancel ends download id if it is active; its file is removed.
func (m *Manager) Cancel(id int) {
	m.mu.Lock()
	cancel := m.cancels[id]
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Remove takes download id off the list unless it is active. Its file
// stays.
func (m *Manager) Remove(id int) {
	m.drop(func(i *Item) bool { return i.ID == id })
}

// Clear takes every download that is not active off the list. Their
// files stay.
func (m *Manager) Clear() {
	m.drop(func(*Item) bool { return true })
}

func (m *Manager) drop(chosen func(*Item) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = slices.DeleteFunc(m.items, func(i *Item) bool { return i.State != Active && chosen(i) })
	m.version++
}

// Items is a copy of the downloads as they stand, newest first.
func (m *Manager) Items() []Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]Item, 0, len(m.items))
	for _, item := range slices.Backward(m.items) {
		items = append(items, *item)
	}
	return items
}

// Item is download id as it stands.
func (m *Manager) Item(id int) (Item, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if item := m.find(id); item != nil {
		return *item, true
	}
	return Item{}, false
}

// Active is how many downloads are in progress.
func (m *Manager) Active() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := 0
	for _, item := range m.items {
		if item.State == Active {
			active++
		}
	}
	return active
}

// Version changes whenever a download does.
func (m *Manager) Version() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.version
}

// Close cancels the downloads in progress, waits for their files to be
// removed, and starts no other.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	for _, cancel := range m.cancels {
		cancel()
	}
	m.mu.Unlock()
	m.running.Wait()
}

func (m *Manager) find(id int) *Item {
	for _, item := range m.items {
		if item.ID == id {
			return item
		}
	}
	return nil
}

func (m *Manager) change(id int, apply func(*Item)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if item := m.find(id); item != nil {
		apply(item)
		m.version++
	}
}
