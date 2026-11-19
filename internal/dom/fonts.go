package dom

import (
	"errors"
	"strconv"
	"strings"
)

// fontWatch is what scripts were last told of the fonts the page's
// text waits for: how many were on their way, and whether a script
// still waits for one.
type fontWatch struct {
	loading int
	waited  bool
}

// fontOps are the operations document.fonts is made of.
func (r *Runtime) fontOps() {
	r.ops["fontsLoading"] = func(arguments) (any, error) { return r.doc.FontsLoading(), nil }
	r.ops["fontsCheck"] = func(a arguments) (any, error) {
		f, err := fontOf(a.text(0))
		if err != nil {
			return nil, err
		}
		return r.doc.FontsReady(f.families, f.weight, f.italic, a.text(1)), nil
	}
	r.ops["fontsLoad"] = func(a arguments) (any, error) {
		f, err := fontOf(a.text(0))
		if err != nil {
			return nil, err
		}
		return r.doc.LoadFonts(f.families, f.weight, f.italic, a.text(1)), nil
	}
}

// watchFonts tells scripts, once a frame, of the fonts on their way:
// that some are, and that the last has landed.
func (r *Runtime) watchFonts() {
	loading := r.doc.FontsLoading()
	if loading == 0 && r.fonts.loading == 0 && !r.fonts.waited {
		return
	}
	left, _ := r.enter("fonts", loading, r.fonts.loading).(float64)
	r.fonts.loading, r.fonts.waited = loading, loading > 0 || left > 0
}

// font is a value of the font property as document.fonts is given it.
type font struct {
	families []string
	weight   int
	italic   bool
}

var errFont = errors.New("SyntaxError: the font is not a value of the font property")

// fontOf reads a value of the font shorthand, "italic 700 16px Lato,
// serif": its style and weight, then a size, then its families.
func fontOf(value string) (font, error) {
	f := font{weight: 400}
	words := strings.Fields(value)
	for i, word := range words {
		lower := strings.ToLower(word)
		if sized(lower) {
			f.families = families(strings.Join(words[i+1:], " "))
			if len(f.families) == 0 {
				return font{}, errFont
			}
			return f, nil
		}
		switch n, err := strconv.Atoi(lower); {
		case err == nil && n >= 1 && n <= 1000:
			f.weight = n
		case lower == "bold":
			f.weight = 700
		case lower == "italic" || lower == "oblique":
			f.italic = true
		}
	}
	return font{}, errFont
}

// sized reports whether word is a font size, with its line height or
// not: a length, a percentage or a size by name.
func sized(word string) bool {
	word, _, _ = strings.Cut(word, "/")
	switch word {
	case "xx-small", "x-small", "small", "medium", "large", "x-large", "xx-large", "smaller", "larger":
		return true
	}
	digits := strings.TrimLeft(word, "0123456789.")
	return digits != word && digits != ""
}

func families(list string) []string {
	var out []string
	for _, name := range strings.Split(list, ",") {
		if name = strings.Trim(strings.TrimSpace(name), `"'`); name != "" {
			out = append(out, name)
		}
	}
	return out
}
