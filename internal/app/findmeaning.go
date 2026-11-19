package app

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/model"
)

// What finding by meaning is cut and judged by. They were set against
// the vectors of the model of the tests alone: a model that embeds for
// real wants them tuned.
const (
	findFeature = "find"
	// passageSize is the most bytes of a passage and passageWords the
	// fewest words of one; maxPassages is the most passages of a page
	// that are embedded.
	passageSize  = 320
	passageWords = 3
	maxPassages  = 400
	// meaningThreshold is how close to the words a passage is, as the
	// cosine of their vectors, to be offered, and maxMeant the most
	// passages offered.
	meaningThreshold = 0.5
	maxMeant         = 5
	// meaningPause is how many frames the words stay as they are before
	// they are embedded.
	meaningPause = 20
)

// meant is a passage of a page and the heading it stands under.
type meant struct {
	passage

	heading string
}

// meaning is what finding by meaning holds: the passages of a page and
// their vectors, made once a page, and the passages close to the words.
type meaning struct {
	// off is whether the robot was turned off: the find is then by the
	// exact words alone.
	off bool
	// page is the page the passages are of, vectors theirs once the
	// model gave them.
	page     *web.Page
	passages []meant
	vectors  [][]float32
	// close are the passages close to the words, the closest first, and
	// sought whether they were looked for; wait is the frames left
	// before they are, and serial counts the words looked for.
	close  []int
	sought bool
	wait   int
	serial int
	// busy is whether the model is embedding off the frame, which read
	// gives the outcome of, again whether other words came meanwhile,
	// and cancel what ends it.
	busy, again bool
	read        chan understood
	cancel      context.CancelFunc
}

// understood is what the model made of a page's passages and of words.
type understood struct {
	page    *web.Page
	serial  int
	vectors [][]float32
	close   []int
	err     error
}

// canMean reports whether the model helps the find and embeds, and
// means whether the find is by meaning too: its robot was not turned
// off.
func (s *Screen) canMean() bool {
	return s.helps(findFeature) && s.opts.Model.Status().Capabilities.Embeddings
}

func (s *Screen) means() bool { return s.canMean() && !s.find.meaning.off }

// meant are the passages that mean the words, the closest first.
func (s *Screen) meant() []passage {
	m := &s.find.meaning
	if !s.means() {
		return nil
	}
	out := make([]passage, len(m.close))
	for i, at := range m.close {
		out[i] = m.passages[at].passage
	}
	return out
}

// unmean forgets what was close to the words; the page's vectors stay.
func (s *Screen) unmean() {
	m := &s.find.meaning
	m.close, m.sought, m.wait = nil, false, 0
	m.serial++
}

// forgetMeaning drops the passages of the page and their vectors, and
// ends what the model was embedding of them.
func (s *Screen) forgetMeaning() {
	m := &s.find.meaning
	if m.cancel != nil {
		m.cancel()
	}
	m.page, m.passages, m.vectors, m.cancel = nil, nil, nil, nil
	s.unmean()
}

// meaningStands says where a find whose words are not on the page
// stands.
func (s *Screen) meaningStands() string {
	m := &s.find.meaning
	switch {
	case m.off:
		return "Not in these words on this page"
	case m.wait > 0 || m.busy || !m.sought:
		return "Not in these words · looking for what means them"
	case len(m.close) == 0:
		return "Not in these words on this page"
	}
	return "Not in these words · " + passagesMean(len(m.close))
}

func passagesMean(n int) string {
	if n == 1 {
		return "1 passage means it"
	}
	return strconv.Itoa(n) + " passages mean it"
}

// showMeaning writes the model's part of the card: its robot, the
// passage the find stands at in the page's own words, and the passages
// as chips named after their headings.
func (s *Screen) showMeaning() {
	f := &s.find
	m := &f.meaning
	bare := len(f.found) > 0 || strings.TrimSpace(s.doc.Value(f.query)) == ""
	s.refill(f.end, func() {
		if bare || !s.canMean() || !m.off && len(m.close) == 0 {
			return
		}
		f.end.Append(robotToggle("find-robot", "These words are not on the page. "+passagesMean(len(m.close))+".",
			!m.off, func() {
				m.off = !m.off
				s.seekWords()
				m.wait = min(m.wait, 1)
			}))
	}, "find-robot")
	s.refill(f.body, func() {
		if bare || !s.means() || len(m.close) == 0 {
			return
		}
		chips := ui.Div(ui.ID("find-passages"))
		for i, at := range m.close {
			name := m.passages[at].heading
			if name == "" {
				name = "Passage " + strconv.Itoa(i+1)
			}
			chips.Append(s.pill("find-passage-"+strconv.Itoa(i), name, name, i == f.at, func() {
				f.at = i
				s.stand()
			}))
		}
		f.body.Append(ui.P(ui.ID("find-quote"), ui.Class("inset"), ui.Text("“"+f.shown[f.at].text+"”")),
			ui.Div(ui.Class("part"),
				ui.Span(ui.ID("find-passage"), ui.Text("Passage "+strconv.Itoa(f.at+1)+" of "+strconv.Itoa(len(m.close)))),
				chips))
	}, "find-passage-"+strconv.Itoa(f.at))
}

// understand has the model embed the words once they stayed as they are
// for a while, and takes what it made of them.
func (s *Screen) understand() {
	m := &s.find.meaning
	select {
	case u := <-m.read:
		m.busy = false
		if u.page == m.page && u.err == nil {
			m.vectors = u.vectors
		}
		if u.page == m.page && u.serial == m.serial && !m.again {
			m.close, m.sought = u.close, true
			s.find.at = 0
			s.stand()
		}
		if m.again {
			m.again = false
			s.mean()
		}
	default:
	}
	if m.wait > 0 {
		if m.wait--; m.wait == 0 {
			s.mean()
		}
	}
}

// mean asks the model, off the frame, for the passages of the page
// close to the words: the page's passages are cut and embedded the
// first time, the words each time.
func (s *Screen) mean() {
	f := &s.find
	m := &f.meaning
	if !s.means() || f.page == nil {
		return
	}
	if m.busy {
		m.again = true
		return
	}
	if m.page != f.page {
		s.forgetMeaning()
		m.page, m.passages = f.page, passagesOf(f.page.Doc)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.busy = cancel, true
	texts := make([]string, len(m.passages))
	for i, p := range m.passages {
		texts[i] = p.text
	}
	asked := understood{page: m.page, serial: m.serial, vectors: m.vectors}
	embeds, words := s.opts.Model, strings.Join(strings.Fields(s.doc.Value(f.query)), " ")
	go func() {
		defer cancel()
		m.read <- asked.of(ctx, embeds, texts, words)
	}()
}

// of embeds the passages, unless their vectors are there, and the
// words, and finds the passages close to them.
func (u understood) of(ctx context.Context, embeds model.Model, texts []string, words string) understood {
	if u.vectors == nil && len(texts) > 0 {
		if u.vectors, u.err = embeds.Embed(ctx, model.Texts{Texts: texts, Priority: model.Interactive}); u.err != nil {
			u.vectors = nil
			return u
		}
	}
	if len(u.vectors) != len(texts) {
		return u
	}
	asked, err := embeds.Embed(ctx, model.Texts{Texts: []string{words}, Kind: model.Query, Priority: model.Interactive})
	if u.err = err; err != nil || len(asked) != 1 {
		return u
	}
	u.close = closest(asked[0], u.vectors)
	return u
}

// closest are the vectors close enough to one, the closest first.
func closest(to []float32, vectors [][]float32) []int {
	near := make([]float32, len(vectors))
	var out []int
	for i, v := range vectors {
		if len(v) != len(to) {
			continue
		}
		for j := range v {
			near[i] += v[j] * to[j]
		}
		if near[i] >= meaningThreshold {
			out = append(out, i)
		}
	}
	slices.SortStableFunc(out, func(a, b int) int {
		switch {
		case near[a] > near[b]:
			return -1
		case near[a] < near[b]:
			return 1
		}
		return 0
	})
	return out[:min(len(out), maxMeant)]
}

// passagesOf are the passages of a page the model embeds: its sentences
// as the page draws them, a long one cut between words, each with the
// heading it stands under. The headings themselves are none.
func passagesOf(doc *ui.Document) []meant {
	var out []meant
	heading := ""
	texts, runs := blocks(doc)
	for i, text := range texts {
		if block := blockOf(runs[i][0].node); block != nil && headed(block) {
			if line := strings.Join(strings.Fields(text), " "); line != "" {
				heading = line
			}
			continue
		}
		for _, span := range spans(text) {
			for _, p := range sentence(nil, text, runs[i], span[0], span[1]) {
				if len(strings.Fields(p.text)) >= passageWords {
					out = append(out, meant{p, heading})
				}
			}
		}
	}
	plain := make([]passage, len(out))
	for i, p := range out {
		plain[i] = p.passage
	}
	rects := rectsOf(doc, plain)
	drawn := out[:0]
	for i, p := range out {
		if len(rects[i]) > 0 && len(drawn) < maxPassages {
			drawn = append(drawn, p)
		}
	}
	return drawn
}

func headed(block *ui.Node) bool {
	switch block.Tag().String() {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	}
	return false
}

// spans are where the passages of a block's text begin and end: at the
// end of each sentence, and between two words of one longer than
// passageSize.
func spans(text string) [][2]int {
	var out [][2]int
	start, space := 0, -1
	for at, r := range text {
		end := at + utf8.RuneLen(r)
		switch {
		case unicode.IsSpace(r):
			space = at
		case strings.ContainsRune(".!?", r) && (end == len(text) || unicode.IsSpace(rune(text[end]))):
			out = append(out, [2]int{start, end})
			start, space = end, -1
			continue
		}
		if end-start > passageSize && space > start {
			out = append(out, [2]int{start, space})
			start, space = space, -1
		}
	}
	return append(out, [2]int{start, len(text)})
}
