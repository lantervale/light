package web

import (
	"sync"
	"time"

	"github.com/lantervale/ember/ui"
)

// landingGrace is how long a picture that came may take to show before
// the page stops waiting for it: ui decodes it off the frame and takes
// it at the next layout, and keeps none it cannot read.
const landingGrace = 2 * time.Second

// pictureWatch is what became of the pictures ui asked a page's
// fetcher for: when each came, zero while it is on its way. One that
// could not be fetched is forgotten.
type pictureWatch struct {
	mu   sync.Mutex
	came map[string]time.Time
}

func (w *pictureWatch) watched(fetch ui.ImageFetcher) ui.ImageFetcher {
	return func(ref string) ([]byte, error) {
		w.mu.Lock()
		if w.came == nil {
			w.came = map[string]time.Time{}
		}
		w.came[ref] = time.Time{}
		w.mu.Unlock()
		data, err := fetch(ref)
		w.mu.Lock()
		defer w.mu.Unlock()
		if err != nil {
			delete(w.came, ref)
		} else {
			w.came[ref] = time.Now()
		}
		return data, err
	}
}

// loading reports whether a picture of doc is on its way, or came and
// has yet to show; asked on the frame.
func (w *pictureWatch) loading(doc *ui.Document) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for ref, at := range w.came {
		switch {
		case at.IsZero():
			return true
		case landed(doc, ref) || time.Since(at) > landingGrace:
			delete(w.came, ref)
		default:
			return true
		}
	}
	return false
}

func landed(doc *ui.Document, ref string) bool {
	if _, ok := doc.ImageSize(ref); ok {
		return true
	}
	_, ok := doc.ImageIntrinsic(ref)
	return ok
}
