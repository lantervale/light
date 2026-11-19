package app //nolint:testpackage // the channels of a page are what its frames hold

import (
	"slices"
	"testing"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/internal/web"
)

// piped is a page with two frames whose documents run scripts, and the
// windows of each as its scripts name them.
func piped() (*framed, *windows, *windows, *windows) {
	top := &web.Page{}
	set := &framed{top: top, session: &session{scripts: &running{page: top}}}
	one, other := &frame{set: set, run: &running{}}, &frame{set: set, run: &running{}}
	set.all = []*frame{one, other}
	return set, &windows{set: set}, &windows{set: set, by: one}, &windows{set: set, by: other}
}

func (set *framed) queued(id int) []string {
	var out []string
	if e := set.ports[id]; e != nil {
		for _, m := range e.queue {
			out = append(out, m.Text)
		}
	}
	return out
}

func TestAnEndIsOnlyItsHolders(t *testing.T) {
	set, top, one, other := piped()
	a, b, made := top.Pipe()
	if !made || a == b || len(set.ports) != 2 {
		t.Fatalf("a channel is %d and %d, %d ends", a, b, len(set.ports))
	}
	one.Send(a, dom.Posted{Text: "forged"})
	one.Send(b, dom.Posted{Text: "forged"})
	one.Requeue(b, dom.Posted{Text: "forged"})
	one.Shut(a)
	other.Shut(b)
	if len(set.ports) != 2 || len(set.queued(a))+len(set.queued(b)) != 0 {
		t.Fatalf("scripts that hold no end of a channel used it: %d ends, %v %v", len(set.ports), set.queued(a),
			set.queued(b))
	}
	_ = one.Post(topWindow, dom.Posted{Text: "steals", Ports: []int{a}}, "*")
	if len(set.mail) != 0 || set.ports[a].holder != set.session.scripts {
		t.Errorf("an end went with the message of scripts that do not hold it")
	}
	top.Send(a, dom.Posted{Text: "first"})
	top.Send(a, dom.Posted{Text: "second"})
	if got := set.queued(b); !slices.Equal(got, []string{"first", "second"}) {
		t.Errorf("the other end holds %v", got)
	}
	top.Requeue(b, dom.Posted{Text: "unread"})
	top.Requeue(b, dom.Posted{Text: "unread too"})
	if got := set.queued(b); !slices.Equal(got, []string{"unread", "unread too", "first", "second"}) {
		t.Errorf("after its holder gave back what it never read the end holds %v", got)
	}
}

func TestAnEndTravelsWithItsMessages(t *testing.T) {
	set, top, one, other := piped()
	a, b, _ := top.Pipe()
	top.Send(a, dom.Posted{Text: "waits"})
	if err := top.Post(99, dom.Posted{Text: "yours", Ports: []int{b}}, "*"); err != nil || len(set.mail) != 0 {
		t.Fatalf("a message for no window was kept: %v, %d", err, len(set.mail))
	}
	if set.ports[b] != nil || set.ports[a].peer != 0 {
		t.Fatalf("the end of a message no window gets stays open")
	}
	top.Shut(a)
	a, b, _ = top.Pipe()
	top.Send(a, dom.Posted{Text: "waits"})
	set.take(set.session.scripts, dom.Posted{Ports: []int{b}})
	top.Send(b, dom.Posted{Text: "from one that gave it away"})
	top.Shut(b)
	if set.ports[b] == nil || len(set.queued(a)) != 0 {
		t.Fatalf("an end on its way is still its sender's")
	}
	set.give(set.all[0].run, dom.Posted{Ports: []int{b}})
	one.Send(b, dom.Posted{Text: "answer"})
	if got := set.queued(a); !slices.Equal(got, []string{"answer"}) {
		t.Errorf("the first end holds %v", got)
	}
	c, d, _ := one.Pipe()
	one.Send(b, dom.Posted{Text: "twice", Ports: []int{c, c}})
	if set.ports[c] != nil || len(set.queued(a)) != 1 {
		t.Errorf("an end listed twice went")
	}
	one.Send(b, dom.Posted{Text: "its own other end", Ports: []int{a}})
	one.Send(b, dom.Posted{Text: "onward", Ports: []int{d}})
	if set.ports[d] == nil || set.ports[d].holder != nil {
		t.Errorf("an end sent onward is not on its way")
	}
	set.give(set.all[1].run, dom.Posted{Ports: []int{d}})
	other.Shut(d)
	one.Shut(b)
	top.Shut(a)
	if len(set.ports) != 0 {
		t.Errorf("%d ends are left of channels closed at both ends", len(set.ports))
	}
	for i := 0; ; i++ {
		if _, _, made := top.Pipe(); !made {
			if i != maxPorts/2 {
				t.Errorf("a page made %d channels, want %d", i, maxPorts/2)
			}
			break
		}
	}
	set.all = nil
	set.release()
	if set.ports != nil {
		t.Errorf("a page let go of keeps its channels")
	}
}
