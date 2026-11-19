package app

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/model"
)

// opens and ends are the marks a page's text is given to the model
// between.
const opens, ends = "<<<", ">>>"

// asData is a text of a page as a request holds it: between marks it
// cannot hold itself, so that nothing of it reads as the request's own.
func asData(text string) string {
	return opens + "\n" + strings.NewReplacer(opens, "", ends, "").Replace(text) + "\n" + ends
}

// fitted is data cut to the tokens room leaves once fixed is counted, and
// whether it was cut.
func fitted(ctx context.Context, m model.Model, room int, fixed, data string) (string, bool, error) {
	used, err := m.Count(ctx, fixed)
	if err != nil {
		return "", false, err
	}
	room -= used
	capped := false
	for range 8 {
		if room <= 0 {
			break
		}
		count, failed := m.Count(ctx, data)
		if failed != nil {
			return "", false, failed
		}
		if count <= room {
			return data, capped, nil
		}
		data, capped = shorten(data, len(data)*room/count*9/10), true
	}
	return "", true, model.ErrTooLong
}

// shorten is the start of text, at most size bytes of it, ended between
// two words.
func shorten(text string, size int) string {
	size = min(max(size, 0), len(text))
	for size > 0 && !utf8.RuneStart(text[size-1]) {
		size--
	}
	text = text[:max(size-1, 0)]
	if space := strings.LastIndexFunc(text, unicode.IsSpace); space > 0 {
		text = text[:space]
	}
	return text
}

// passage is a run of a page's text: where it begins and ends in the
// Document, and what it reads.
type passage struct {
	from, to ui.Position
	text     string
}

// run is a text node of a page and where its text begins in the text
// of its block.
type run struct {
	node *ui.Node
	at   int
}

// blocks are the texts of a Document's blocks, in order, each with the
// text nodes it is made of: what a reader reads, not its scripts or
// its sheets.
func blocks(doc *ui.Document) ([]string, [][]run) {
	var texts []string
	var runs [][]run
	var last *ui.Node
	doc.Body().Walk(func(n *ui.Node) bool {
		if !n.IsText() {
			switch n.Tag().String() {
			case "script", "style", "noscript", "template", "head", "title", "svg", "textarea", "select":
				return false
			}
			return true
		}
		block := blockOf(n)
		if block != last || len(texts) == 0 {
			texts, runs, last = append(texts, ""), append(runs, nil), block
		}
		i := len(texts) - 1
		runs[i] = append(runs[i], run{n, len(texts[i])})
		texts[i] += n.Text()
		return true
	})
	return texts, runs
}

// blockOf is the nearest element round n that is not a part of a line.
func blockOf(n *ui.Node) *ui.Node {
	for n = n.Parent(); n != nil; n = n.Parent() {
		switch n.Tag().String() {
		case "a", "b", "i", "em", "strong", "span", "code", "small", "sub", "sup", "u", "s", "mark", "abbr", "cite",
			"q", "time", "var", "kbd", "samp", "font", "big", "label":
			continue
		}
		return n
	}
	return nil
}

// pageText is what a page says, a line a block, its white space single.
func pageText(doc *ui.Document) string {
	texts, _ := blocks(doc)
	var lines []string
	for _, text := range texts {
		if line := strings.Join(strings.Fields(text), " "); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// sentences are the sentences of a page, each where it is in the
// Document.
func sentences(doc *ui.Document) []passage {
	var out []passage
	texts, runs := blocks(doc)
	for i, text := range texts {
		start := 0
		for at, r := range text {
			end := at + utf8.RuneLen(r)
			if !strings.ContainsRune(".!?", r) || end < len(text) && !unicode.IsSpace(rune(text[end])) {
				continue
			}
			out = sentence(out, text, runs[i], start, end)
			start = end
		}
		out = sentence(out, text, runs[i], start, len(text))
	}
	return out
}

// sentence adds the text of a block from start to end, without the
// white space round it, to the passages.
func sentence(out []passage, text string, runs []run, start, end int) []passage {
	trimmed := strings.TrimSpace(text[start:end])
	if trimmed == "" {
		return out
	}
	start += strings.Index(text[start:end], trimmed)
	return append(out, passage{place(runs, start, false), place(runs, start+len(trimmed), true),
		strings.Join(strings.Fields(trimmed), " ")})
}

// place is the place of the byte at of a block's text in its text
// nodes: at the end of a node rather than the start of the next for
// closing.
func place(runs []run, at int, closing bool) ui.Position {
	for i := len(runs) - 1; i >= 0; i-- {
		if r := runs[i]; at > r.at || at == r.at && (!closing || i == 0) {
			return ui.Position{Node: r.node, Offset: utf8.RuneCountInString(r.node.Text()[:at-r.at])}
		}
	}
	return ui.Position{}
}
