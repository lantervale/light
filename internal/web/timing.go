package web

import (
	"context"
	"crypto/tls"
	"net/http/httptrace"
	"sync"
	"time"
)

// Mark is one step of a page's load, in the order they come.
type Mark int

// The steps of a load. The loader marks those from Asked to Built; the
// others are the window's to mark, which knows when it showed the page.
const (
	// Started is when the page was asked for: a link, the address bar.
	Started Mark = iota
	// Asked is when the loader gave its client the request.
	Asked
	// Connected is when the request had a connection, a new one or one
	// kept, its TLS handshake done: of the last request of a redirected
	// load, as Sent and Answered are.
	Connected
	// Sent is when the request was written.
	Sent
	// Answered is when the answer's first byte came.
	Answered
	// Received is when the document was read whole.
	Received
	// Parsed is when its tree was made.
	Parsed
	// Gathered is when its sheets and its scripts were all fetched.
	Gathered
	// Built is when its Document was built: the load's end.
	Built
	// Shown is when the tab took the page for the one it shows.
	Shown
	// Drawn is when the first frame that drew the page had ended.
	Drawn
	// Loaded is when the page's scripts were told of its load.
	Loaded
	marks
)

func (m Mark) String() string {
	return [...]string{"started", "asked", "connected", "sent", "answered", "received", "parsed", "gathered", "built",
		"shown", "drawn", "loaded", "nothing"}[m]
}

// Marks are the steps of a load, in their order.
func Marks() []Mark {
	all := make([]Mark, marks)
	for i := range all {
		all[i] = Mark(i)
	}
	return all
}

// Timing is when each step of one page's load ended: marked off the
// frame by its load and on it by its window, and read by either. A nil
// Timing takes no mark.
type Timing struct {
	mu sync.Mutex
	at [marks]time.Time
	// asking is whether the document's own request is on its way: what
	// the page fetches next tells of its connections too.
	asking bool
}

// Mark notes that the step m ended now. Connected, Sent and Answered are
// of the last request, the others of the first time.
func (t *Timing) Mark(m Mark) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch again := m == Connected || m == Sent || m == Answered; {
	case m == Asked:
		t.asking = true
		if t.at[m].IsZero() {
			t.at[m] = time.Now()
		}
	case again && t.asking, !again && t.at[m].IsZero():
		t.at[m] = time.Now()
	}
}

// settle marks the steps of a request that told nothing of them, one a
// test's transport answered: they ended when its answer began.
func (t *Timing) settle() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.asking = false
	for _, m := range []Mark{Connected, Sent, Answered} {
		if t.at[m].Before(t.at[Asked]) {
			t.at[m] = now
		}
	}
}

// At is when the step m ended, the zero time for one that has not.
func (t *Timing) At(m Mark) time.Time {
	if t == nil {
		return time.Time{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.at[m]
}

// Reached is the last step that ended.
func (t *Timing) Reached() Mark {
	reached := Started
	for _, m := range Marks() {
		if !t.At(m).IsZero() {
			reached = m
		}
	}
	return reached
}

// Since is how long after the load started the step m ended, and
// whether it did.
func (t *Timing) Since(m Mark) (time.Duration, bool) {
	at, from := t.At(m), t.At(Started)
	if from.IsZero() {
		from = t.At(Asked)
	}
	if at.IsZero() || from.IsZero() {
		return 0, false
	}
	return at.Sub(from), true
}

type timingKey struct{}

// Timed is ctx for a load whose steps t is told of.
func Timed(ctx context.Context, t *Timing) context.Context {
	if t == nil {
		return ctx
	}
	connected := func() { t.Mark(Connected) }
	return httptrace.WithClientTrace(context.WithValue(ctx, timingKey{}, t), &httptrace.ClientTrace{
		GotConn:              func(httptrace.GotConnInfo) { connected() },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { connected() },
		WroteRequest:         func(httptrace.WroteRequestInfo) { t.Mark(Sent) },
		GotFirstResponseByte: func() { t.Mark(Answered) },
	})
}

func timingOf(ctx context.Context) *Timing {
	t, _ := ctx.Value(timingKey{}).(*Timing)
	return t
}
