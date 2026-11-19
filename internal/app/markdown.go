package app

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lantervale/ember/ui"
)

// markdownOf is a selection of a page written as Markdown: its
// headings, its list items and its paragraphs each on a line of their
// own, what is bold or slanted marked so. It is what the model is
// given, so that what it writes back can keep the passage's shape.
func markdownOf(doc *ui.Document, sel ui.Selection) string {
	if sel.Empty() || doc == nil {
		return ""
	}
	var runs []*ui.Node
	doc.Body().Walk(func(n *ui.Node) bool {
		if n.IsText() {
			runs = append(runs, n)
		}
		return true
	})
	from, to := sel.Anchor, sel.Focus
	first, last := -1, -1
	for i, n := range runs {
		if n == from.Node {
			first = i
		}
		if n == to.Node {
			last = i
		}
	}
	if first < 0 || last < 0 {
		return ""
	}
	if first > last || first == last && from.Offset > to.Offset {
		first, last, from, to = last, first, to, from
	}
	var out strings.Builder
	var block, written *ui.Node
	line := ""
	flush := func() {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			switch {
			case out.Len() == 0:
			case isItem(block) && isItem(written) && block.Parent() == written.Parent():
				out.WriteString("\n")
			default:
				out.WriteString("\n\n")
			}
			out.WriteString(mdOpening(block) + line)
			written = block
		}
		line = ""
	}
	for i := first; i <= last; i++ {
		n := runs[i]
		text := []rune(n.Text())
		start, end := 0, len(text)
		if i == first {
			start = min(max(from.Offset, 0), len(text))
		}
		if i == last {
			end = min(max(to.Offset, start), len(text))
		}
		if of := blockOf(n); of != block || i == first {
			flush()
			block = of
		}
		line += inline(n, block, string(text[start:end]))
	}
	flush()
	return out.String()
}

func isItem(block *ui.Node) bool { return block != nil && block.Tag().String() == "li" }

// mdOpening is what a block's line begins with in Markdown.
func mdOpening(block *ui.Node) string {
	if block == nil {
		return ""
	}
	switch tag := block.Tag().String(); tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return strings.Repeat("#", int(tag[1]-'0')) + " "
	case "li":
		list := block.Parent()
		if list == nil || list.Tag().String() != "ol" {
			return "- "
		}
		at := 0
		for _, k := range list.Children() {
			if !k.IsText() && k.Tag().String() == "li" {
				at++
			}
			if k == block {
				break
			}
		}
		return strconv.Itoa(at) + ". "
	}
	return ""
}

// inline is the text of a text node of block with the marks of what it
// is in: bold or slanted.
func inline(n, block *ui.Node, text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	mark := ""
	for p := n.Parent(); p != nil && p != block; p = p.Parent() {
		switch p.Tag().String() {
		case "strong", "b":
			mark = "**"
		case "em", "i":
			if mark == "" {
				mark = "*"
			}
		}
	}
	if mark == "" {
		return text
	}
	core := strings.TrimSpace(text)
	lead, tail, _ := strings.Cut(text, core)
	return lead + mark + core + mark + tail
}

// markdown is a text of the model's written as Markdown, as elements:
// headings, list items and paragraphs, with what is bold, slanted or
// code. Nothing else of Markdown is read, and nothing of the text is
// ever markup: a link or a tag shows as it was written.
func markdown(text string) []*ui.Node {
	var out []*ui.Node
	var para []string
	flush := func() {
		if len(para) > 0 {
			out = append(out, ui.P(mdSpans(strings.Join(para, " "))...))
			para = nil
		}
	}
	for row := range strings.SplitSeq(text, "\n") {
		row = strings.TrimSpace(row)
		level := len(row) - len(strings.TrimLeft(row, "#"))
		bullet, item, listed := listItem(row)
		switch {
		case row == "":
			flush()
		case level > 0 && level <= 6 && strings.HasPrefix(row[level:], " "):
			flush()
			out = append(out, ui.H4(mdSpans(strings.TrimSpace(row[level:]))...))
		case listed:
			flush()
			out = append(out, ui.Div(ui.Class("item"), ui.Span(ui.Class("bullet"), ui.Text(bullet)),
				ui.Span(mdSpans(item)...)))
		default:
			para = append(para, row)
		}
	}
	flush()
	return out
}

// listItem reads a row that is a list's item: its bullet as shown and
// its text.
func listItem(row string) (string, string, bool) {
	for _, dash := range []string{"- ", "* ", "+ "} {
		if rest, ok := strings.CutPrefix(row, dash); ok && strings.TrimSpace(rest) != "" {
			return "•", strings.TrimSpace(rest), true
		}
	}
	digits := len(row) - len(strings.TrimLeft(row, "0123456789"))
	if digits == 0 || digits > 3 || len(row) < digits+2 || (row[digits] != '.' && row[digits] != ')') ||
		row[digits+1] != ' ' {
		return "", "", false
	}
	return row[:digits] + ".", strings.TrimSpace(row[digits+2:]), true
}

// mdSpans are a line's parts: plain text, and what stands between two
// marks of bold, of slant or of code. A mark that is not closed, as
// while the text still comes, is text.
func mdSpans(line string) []ui.Arg {
	var out []ui.Arg
	plain := ""
	text := func() {
		if plain != "" {
			out = append(out, ui.Text(plain))
			plain = ""
		}
	}
	for line != "" {
		mark, wrap := "", ui.Em
		switch {
		case strings.HasPrefix(line, "**"):
			mark, wrap = "**", ui.Strong
		case strings.HasPrefix(line, "__"):
			mark, wrap = "__", ui.Strong
		case line[0] == '`':
			mark, wrap = "`", ui.Code
		case line[0] == '*':
			mark = "*"
		}
		rest := line[len(mark):]
		end := strings.Index(rest, mark)
		if mark == "" || end <= 0 || strings.TrimSpace(rest[:end]) != rest[:end] {
			_, size := utf8.DecodeRuneInString(line)
			plain += line[:size]
			line = line[size:]
			continue
		}
		text()
		out = append(out, wrap(ui.Text(rest[:end])))
		line = rest[end+len(mark):]
	}
	text()
	return out
}
