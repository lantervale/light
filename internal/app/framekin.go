package app

import (
	"context"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/pkg/frames"
)

// kinKey names the documents the one f shows, the page's for nil, is
// kin to: those of its origin that keep their storage where it does
// (frames.Kin), from the origin light holds it to be of; false for a
// document that is kin to none.
func (set *framed) kinKey(f *frame) (string, bool) {
	if f == nil {
		return frames.Kin(set.top.Base, nil, frames.Origin(set.top.Base), frames.Sandbox{})
	}
	if f.page == nil {
		return "", false
	}
	return frames.Kin(set.top.Base, f.chain(), f.origin, f.held)
}

// kinOf are the documents of the page that share an engine with the
// one f shows, the page's for nil; nil for a document that shares its
// engine with none.
func (set *framed) kinOf(f *frame) *dom.Kin {
	key, has := set.kinKey(f)
	if !has {
		return nil
	}
	if set.kin == nil {
		set.kin = map[string]*dom.Kin{}
	}
	if set.kin[key] == nil {
		set.kin[key] = dom.NewKin(key)
	}
	return set.kin[key]
}

// reachable reports whether the document f shows may be given scripts
// for its kin to reach it, though it has none of its own to run.
func (s *Screen) reachable(f *frame) bool {
	return !f.gone && f.windowed() && f.page != nil && (f.load == nil || f.page.Address == blankAddress) &&
		f.page.Err == nil && f.page.Doc != nil && s.scripting() && f.held.Runs() && f.page.Tree != nil &&
		f.page.Tree.Activate == nil
}

// initial gives f the document a frame holds until it shows another:
// about:blank, empty, of the origin of the document its element is in.
// It is made when a script of the kin it would be of reaches for it,
// which may be before the frame was first followed, and reports
// whether f holds a document.
func (s *Screen) initial(f *frame, kin *dom.Kin) bool {
	if f.gone || !f.windowed() || !s.config.Frames || !s.scripting() {
		return false
	}
	if !f.live {
		s.follow1(f, s.asks(f))
	}
	if f.page != nil || !f.admitted {
		return f.page != nil
	}
	chain := f.documents()
	chain = append(chain[:len(chain)-1], f.in.Base)
	if key, has := frames.Kin(f.set.top.Base, chain, f.set.originIn(f.parent), f.sandbox); !has || key != kin.Key() {
		return false
	}
	page := s.frameLoader(f).Inline(context.Background(), f.in.Base, f.in.Space, "", s.frameLinks(f))
	if page.Err != nil || page.Doc == nil || page.Tree == nil {
		return false
	}
	page.Address = blankAddress
	f.page, f.held = page, f.sandbox
	f.origin = f.originOf(page)
	f.in.Doc.SetFrame(f.node, page.Doc)
	return true
}
