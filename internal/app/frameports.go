package app

import (
	"maps"
	"slices"

	"github.com/lantervale/light/internal/dom"
)

// maxPorts is how many ends of channels between its documents a page
// holds at most.
const maxPorts = 1024

// end is one end of a channel between two documents of a page: a
// MessagePort one of them holds. The scripts know it by a number that
// gives them nothing but what is posted through it, and only while
// light knows them as its holder: an end named by other scripts is no
// end. What is posted to an end waits in it until its holder is known,
// and goes with it to the next one.
type end struct {
	// peer is the other end, 0 once it was closed.
	peer int
	// holder is the scripts that hold the end, nil while it travels
	// with a message.
	holder *running
	queue  []dom.Posted
	// back is how many messages its holder put back before giving it
	// away: they are read before what came since.
	back int
}

// held is the end of that number when by holds it, nil otherwise.
func (set *framed) held(by *running, id int) *end {
	if e := set.ports[id]; e != nil && by != nil && e.holder == by {
		return e
	}
	return nil
}

// take has the ends a message carries leave by, its sender: false,
// with those it does hold closed, when one of them is not its own to
// give or is given twice.
func (set *framed) take(by *running, m dom.Posted) bool {
	seen := map[int]bool{}
	for _, id := range m.Ports {
		if set.held(by, id) == nil || seen[id] {
			for _, mine := range m.Ports {
				if set.held(by, mine) != nil {
					set.shut(mine)
				}
			}
			return false
		}
		seen[id] = true
	}
	for _, id := range m.Ports {
		set.ports[id].holder, set.ports[id].back = nil, 0
	}
	return true
}

// give has the ends a message carries held by the scripts that get it.
func (set *framed) give(to *running, m dom.Posted) {
	for _, id := range m.Ports {
		if e := set.ports[id]; e != nil {
			e.holder = to
		}
	}
}

// lose closes the ends of a message no one gets.
func (set *framed) lose(m dom.Posted) {
	for _, id := range m.Ports {
		set.shut(id)
	}
}

// shut closes an end: what waited in it is lost, and its other end
// has no other end from then on, which its holder is told (orphans).
func (set *framed) shut(id int) {
	e := set.ports[id]
	if e == nil {
		return
	}
	delete(set.ports, id)
	for _, m := range e.queue {
		set.lose(m)
	}
	if peer := set.ports[e.peer]; peer != nil {
		peer.peer = 0
	}
}

// numbered are the numbers of the ends, the oldest first.
func (set *framed) numbered() []int { return slices.Sorted(maps.Keys(set.ports)) }

// carries reports whether a message waits in an end whose holder may
// read it.
func (set *framed) carries() bool {
	for _, e := range set.ports {
		if e.holder != nil && len(e.queue) > 0 {
			return true
		}
	}
	return false
}

// sweep closes the ends of the scripts that are no more.
func (set *framed) sweep() {
	for _, id := range set.numbered() {
		if e := set.ports[id]; e != nil && e.holder != nil && e.holder.rt.Closed() {
			set.shut(id)
		}
	}
}

// carry gives each holder what waits in its ends.
func (set *framed) carry() {
	for _, id := range set.numbered() {
		e := set.ports[id]
		if e == nil || e.holder == nil || len(e.queue) == 0 {
			continue
		}
		queue := e.queue
		e.queue, e.back = nil, 0
		for i, m := range queue {
			if set.ports[id] != e || e.holder == nil {
				for _, lost := range queue[i:] {
					set.lose(lost)
				}
				break
			}
			set.give(e.holder, m)
			e.holder.rt.PortMessage(id, m)
		}
	}
}

// orphans tells the holders of the ends whose other end was closed,
// and forgets those ends.
func (set *framed) orphans() {
	for _, id := range set.numbered() {
		if e := set.ports[id]; e != nil && e.peer == 0 && e.holder != nil && len(e.queue) == 0 {
			set.shut(id)
			e.holder.rt.PortClosed(id)
		}
	}
}

func (w *windows) Pipe() (int, int, bool) {
	set := w.set
	by := set.scriptsOf(w.by)
	if by == nil || len(set.ports)+2 > maxPorts {
		return 0, 0, false
	}
	if set.ports == nil {
		set.ports = map[int]*end{}
	}
	one, other := set.lastPort+1, set.lastPort+2
	set.lastPort = other
	set.ports[one], set.ports[other] = &end{peer: other, holder: by}, &end{peer: one, holder: by}
	return one, other, true
}

func (w *windows) Send(port int, m dom.Posted) {
	set := w.set
	by := set.scriptsOf(w.by)
	e := set.held(by, port)
	if !set.take(by, m) {
		return
	}
	if e == nil || set.ports[port] != e {
		set.lose(m)
		return
	}
	peer := set.ports[e.peer]
	if peer == nil || len(peer.queue) >= maxMail || slices.Contains(m.Ports, e.peer) {
		set.lose(m)
		return
	}
	peer.queue = append(peer.queue, m)
}

func (w *windows) Requeue(port int, m dom.Posted) {
	set := w.set
	by := set.scriptsOf(w.by)
	e := set.held(by, port)
	if !set.take(by, m) {
		return
	}
	if e == nil || set.ports[port] != e || len(e.queue) >= maxMail || slices.Contains(m.Ports, port) {
		set.lose(m)
		return
	}
	e.queue = slices.Insert(e.queue, e.back, m)
	e.back++
}

func (w *windows) Shut(port int) {
	if w.set.held(w.set.scriptsOf(w.by), port) != nil {
		w.set.shut(port)
	}
}
