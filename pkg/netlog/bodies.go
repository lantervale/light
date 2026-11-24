package netlog

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
	"slices"
	"time"

	"github.com/lantervale/light/pkg/instant"
)

// KeepBodies has the log keep what its requests carry and the bodies of
// their answers: up to each bytes of one, and all bytes of them together,
// the oldest dropped first. A log keeps none until told, and none of a
// body already on its way.
func (l *Log) KeepBodies(each, all int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.each, l.all = each, all
	l.shed()
}

// Held is how many bytes of bodies the log keeps.
func (l *Log) Held() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.held
}

// shed drops the oldest bodies while they are over the budget: a row
// keeps its preview.
func (l *Log) shed() {
	for l.held > l.all {
		at := slices.IndexFunc(l.rows, func(r *Row) bool { return len(r.Body)+len(r.Sent) > 0 })
		if at < 0 {
			return
		}
		r := l.rows[at]
		l.held -= int64(len(r.Body) + len(r.Sent))
		r.Preview = slices.Clone(r.Preview)
		r.BodyDropped, r.SentCut = r.Size > 0, r.SentSize > 0
		r.Body, r.Sent = nil, nil
	}
}

// read counts p, read of row's body, and keeps what the caps allow.
func (l *Log) read(row *Row, p []byte, transferred int64, took time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	whole := int64(len(row.Body)) == row.Size
	row.Size += int64(len(p))
	row.Transferred, row.Took = transferred, took
	if whole && l.each > 0 && !row.gone && !row.BodyDropped {
		keep := min(int64(len(p)), l.each-int64(len(row.Body)))
		row.Body = append(row.Body, p[:keep]...)
		row.BodyCut = keep < int64(len(p))
		shown := min(len(row.Body), PreviewCap)
		row.Preview = row.Body[:shown:shown]
		l.held += keep
		l.shed()
		p = p[keep:]
	}
	row.Preview = append(row.Preview, p[:min(len(p), PreviewCap-len(row.Preview))]...)
	row.Truncated = row.Size > int64(len(row.Preview))
	l.version++
}

// carried counts p, sent of row's request, and keeps what the caps allow.
func (l *Log) carried(row *Row, p []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	whole := int64(len(row.Sent)) == row.SentSize
	row.SentSize += int64(len(p))
	if whole && l.each > 0 && !row.gone {
		keep := min(int64(len(p)), l.each-int64(len(row.Sent)))
		row.Sent = append(row.Sent, p[:keep]...)
		l.held += keep
		l.shed()
	}
	row.SentCut = row.SentSize > int64(len(row.Sent))
	l.version++
}

// carrying is req with what it carries recorded in row as it is sent,
// from the start again when the transport sends it again.
func (l *Log) carrying(req *http.Request, row *Row) {
	if req.Body == nil || req.Body == http.NoBody {
		return
	}
	req.Body = &sent{ReadCloser: req.Body, log: l, row: row}
	if again := req.GetBody; again != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			body, err := again()
			if err != nil {
				return nil, err
			}
			l.mu.Lock()
			if !row.gone {
				l.held -= int64(len(row.Sent))
			}
			row.Sent, row.SentSize, row.SentCut = nil, 0, false
			l.mu.Unlock()
			return &sent{ReadCloser: body, log: l, row: row}, nil
		}
	}
}

// sent is what a request carries, recorded as the transport reads it.
type sent struct {
	io.ReadCloser
	log *Log
	row *Row
}

func (s *sent) Read(p []byte) (int, error) {
	n, err := s.ReadCloser.Read(p)
	if n > 0 {
		s.log.carried(s.row, p[:n])
	}
	return n, err
}

// marks are when the steps of a request were taken, zero for one that
// was not: a connection used again is neither looked up nor dialled.
type marks struct {
	dns, dnsDone, dial, dialDone, tls, tlsDone time.Time
	// conn is when the request had its connection, wrote when it was
	// sent, first when the answer's first byte came and answered when
	// its headers were read.
	conn, wrote, first, answered time.Time
}

// Timing is how long each step of a request took, -1 for a step that was
// not taken or that the transport does not tell: Blocked waiting for a
// connection, DNS looking the host up, Connect dialling it (TLS, its
// handshake, included), Send writing the request, Wait until the
// answer's first byte and Receive reading it to its end.
type Timing struct {
	Blocked, DNS, Connect, TLS, Send, Wait, Receive time.Duration
}

// Timing is the row's steps so far. They add up to Took.
func (r Row) Timing() Timing {
	t := Timing{Blocked: -1, DNS: -1, Connect: -1, TLS: -1}
	m, end := r.marks, r.Started.Add(r.Took)
	span := func(from, to time.Time) time.Duration {
		if from.IsZero() || to.IsZero() {
			return -1
		}
		return max(0, to.Sub(from))
	}
	first := m.first
	if first.IsZero() {
		first = m.answered
	}
	if m.conn.IsZero() {
		if t.Wait = r.Took; !first.IsZero() {
			t.Wait, t.Receive = span(r.Started, first), span(first, end)
		}
		return t
	}
	t.DNS, t.TLS = span(m.dns, m.dnsDone), span(m.tls, m.tlsDone)
	dialled := m.dialDone
	if m.tlsDone.After(dialled) {
		dialled = m.tlsDone
	}
	t.Connect = span(m.dial, dialled)
	t.Blocked = max(0, m.conn.Sub(r.Started)-max(0, t.DNS)-max(0, t.Connect))
	wrote := m.wrote
	if wrote.IsZero() {
		wrote = m.conn
	}
	if first.IsZero() {
		first = end
	}
	t.Send, t.Wait, t.Receive = span(m.conn, wrote), span(wrote, first), span(first, end)
	return t
}

// traced is the trace that marks row's steps as the transport takes them.
func (l *Log) traced(row *Row) *httptrace.ClientTrace {
	mark := func(which func(m *marks) *time.Time) {
		now := instant.Now()
		l.change(row, func(r *Row) {
			if at := which(&r.marks); at.IsZero() {
				*at = now
			}
		})
	}
	return &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			remote := info.Conn.RemoteAddr().String()
			l.change(row, func(r *Row) { r.Remote, r.Reused, r.Idle = remote, info.Reused, info.WasIdle })
			mark(func(m *marks) *time.Time { return &m.conn })
		},
		DNSStart: func(httptrace.DNSStartInfo) { mark(func(m *marks) *time.Time { return &m.dns }) },
		DNSDone:  func(httptrace.DNSDoneInfo) { mark(func(m *marks) *time.Time { return &m.dnsDone }) },
		ConnectStart: func(string, string) {
			mark(func(m *marks) *time.Time { return &m.dial })
		},
		ConnectDone: func(_, _ string, err error) {
			if err == nil {
				mark(func(m *marks) *time.Time { return &m.dialDone })
			}
		},
		TLSHandshakeStart: func() { mark(func(m *marks) *time.Time { return &m.tls }) },
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			mark(func(m *marks) *time.Time { return &m.tlsDone })
		},
		WroteRequest:         func(httptrace.WroteRequestInfo) { mark(func(m *marks) *time.Time { return &m.wrote }) },
		GotFirstResponseByte: func() { mark(func(m *marks) *time.Time { return &m.first }) },
	}
}
