package web

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// view is the width pictures are chosen for where the loader knows no
// view.
const view = 1280

// The attributes an img chooses its picture by: the pictures there are
// and how wide it shows.
const (
	attrSrcset = "srcset"
	attrSizes  = "sizes"
)

// candidate is the address a srcset gives for a slot as wide as sizes
// says in a view of that width, at one pixel a point: the smallest
// picture that fills it, the largest when none does, "" for a srcset
// that names none.
func candidate(srcset, sizes string, view float32) string {
	slot := slotOf(sizes, view)
	best, over := "", float32(0)
	for _, part := range candidates(srcset) {
		address, descriptor, _ := strings.Cut(part, " ")
		density := float32(1)
		switch descriptor = strings.TrimSpace(descriptor); {
		case strings.HasSuffix(descriptor, "w"):
			w, err := strconv.ParseFloat(strings.TrimSuffix(descriptor, "w"), 32)
			if err != nil || w <= 0 || slot <= 0 {
				continue
			}
			density = float32(w) / slot
		case strings.HasSuffix(descriptor, "x"):
			x, err := strconv.ParseFloat(strings.TrimSuffix(descriptor, "x"), 32)
			if err != nil || x <= 0 {
				continue
			}
			density = float32(x)
		case descriptor != "":
			continue
		}
		if best == "" || (over < 1 && density > over) || (density >= 1 && density < over) {
			best, over = address, density
		}
	}
	return best
}

// candidates are the entries of a srcset, each an address and its
// descriptor after a space: an address ends at a space, so it may hold
// commas, and one that ends with a comma has no descriptor.
func candidates(srcset string) []string {
	var parts []string
	for {
		srcset = strings.TrimLeft(srcset, ", \t\r\n\f")
		if srcset == "" {
			return parts
		}
		end := strings.IndexAny(srcset, " \t\r\n\f")
		if end < 0 {
			end = len(srcset)
		}
		address, rest := srcset[:end], srcset[end:]
		if bare := strings.TrimRight(address, ","); bare != address {
			parts, srcset = append(parts, bare), rest
			continue
		}
		descriptor, after, _ := strings.Cut(rest, ",")
		parts, srcset = append(parts, address+" "+strings.TrimSpace(descriptor)), after
	}
}

// slotOf is the width sizes gives a picture in a view: the length of
// its first entry whose condition holds, the view's without one.
func slotOf(sizes string, view float32) float32 {
	for _, entry := range strings.Split(sizes, ",") {
		entry = strings.TrimSpace(entry)
		condition, length := "", entry
		if at := strings.LastIndex(entry, ")"); at >= 0 && !strings.HasPrefix(entry, "calc(") {
			condition, length = entry[:at+1], strings.TrimSpace(entry[at+1:])
		}
		if w, ok := lengthIn(length, view); ok && holds(condition, view) {
			return w
		}
	}
	return view
}

// lengthIn is a length of a sizes attribute in pixels: px, vw and em.
func lengthIn(length string, view float32) (float32, bool) {
	for unit, by := range map[string]float32{"px": 1, "vw": view / 100, "rem": 16, "em": 16} {
		if number, found := strings.CutSuffix(length, unit); found {
			if v, err := strconv.ParseFloat(strings.TrimSpace(number), 32); err == nil && v >= 0 {
				return float32(v) * by, true
			}
		}
	}
	return 0, false
}

// holds reports whether a media condition holds in a view of that
// width: "", and min-width and max-width joined by and; any other
// does not.
func holds(condition string, view float32) bool {
	for _, part := range strings.Split(strings.ToLower(condition), " and ") {
		part = strings.TrimSpace(part)
		if part == "" || part == "screen" || part == "all" {
			continue
		}
		name, value, found := strings.Cut(strings.Trim(part, "()"), ":")
		w, ok := lengthIn(strings.TrimSpace(value), view)
		switch name = strings.TrimSpace(name); {
		case !found || !ok:
			return false
		case name == "min-width" && view >= w, name == "max-width" && view <= w:
		default:
			return false
		}
	}
	return true
}

// decoded reports whether light draws pictures of a source's type: any
// it doesn't say, and those the loader asks for.
func decoded(kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	return kind == "" || strings.Contains(drawn, kind)
}

// chosen is the picture an img shows: the candidate of the first source
// of its picture that light draws and whose media holds, else of its
// own srcset, else its src.
func (b *builder) chosen(n *html.Node) (string, bool) {
	width := float32(view)
	if b.loader != nil && b.loader.Viewport != nil {
		if w, _ := b.loader.Viewport(); w > 0 {
			width = w
		}
	}
	if p := n.Parent; p != nil && p.DataAtom == atom.Picture {
		for s := p.FirstChild; s != nil && s != n; s = s.NextSibling {
			if s.DataAtom != atom.Source || !decoded(attr(s, "type")) || !holds(attr(s, "media"), width) {
				continue
			}
			if address := candidate(attr(s, "srcset"), attr(s, "sizes"), width); address != "" {
				return address, true
			}
		}
	}
	if address := candidate(attr(n, "srcset"), attr(n, "sizes"), width); address != "" {
		return address, true
	}
	return lookup(n, "src")
}
