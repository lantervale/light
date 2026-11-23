package dom

import (
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
)

// unitRanges are the properties whose number a computed style clamps
// to [0, 1].
//
//nolint:gochecknoglobals // a constant set
var unitRanges = map[string]bool{
	"opacity": true, "fill-opacity": true, "stroke-opacity": true, "flood-opacity": true, "stop-opacity": true,
}

// integers are the properties whose number a computed style rounds to
// the nearest integer.
//
//nolint:gochecknoglobals // a constant set
var integers = map[string]bool{
	"z-index": true, "order": true, "orphans": true, "widows": true, "column-count": true,
}

var errUnworked = errors.New("not worked out")

// The canonical units of lengths, angles and times.
const (
	unitPx  = "px"
	unitDeg = "deg"
	unitS   = "s"
)

// fontSizes are the sizes in px em and rem stand for, 0 where unknown.
type fontSizes struct{ em, rem float64 }

// quantity is a number and its unit: "" for a number, px, %, deg or s,
// the canonical unit of its kind.
type quantity struct {
	v    float64
	unit string
}

// worked is value with each math function that comes to a number, or
// to a length, an angle, a time or a percentage of one unit, replaced
// by what it comes to, as the computed style of the property name
// holds it; the rest is left as written.
func worked(name, value string, fonts fontSizes) string {
	if !strings.Contains(value, "(") {
		return value
	}
	var out strings.Builder
	for i := 0; i < len(value); {
		start, fn, open := mathCall(value, i)
		if fn == "" {
			out.WriteString(value[i:])
			break
		}
		out.WriteString(value[i:start])
		end := closing(value, open)
		if end < 0 {
			out.WriteString(value[start:])
			break
		}
		p := &calcParser{src: value[start : end+1], fonts: fonts}
		q, err := p.call()
		if err != nil || p.at != len(p.src) {
			out.WriteString(value[start : end+1])
		} else if text, ok := serialized(name, q, start == 0 && end == len(value)-1); ok {
			out.WriteString(text)
		} else {
			out.WriteString(value[start : end+1])
		}
		i = end + 1
	}
	return out.String()
}

// mathCall finds the next math function in value from i: where its
// name starts, the name, and where its parenthesis opens.
func mathCall(value string, i int) (int, string, int) {
	for j := i; j < len(value); j++ {
		if value[j] != '(' {
			continue
		}
		k := j
		for k > i && (isNameByte(value[k-1])) {
			k--
		}
		if name := strings.ToLower(value[k:j]); mathFunctions[name] != nil && (k == 0 || !isNameByte(value[k-1])) {
			return k, name, j
		}
	}
	return 0, "", 0
}

func isNameByte(c byte) bool {
	return c == '-' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// closing is where the parenthesis opened at open closes, -1 for none.
func closing(value string, open int) int {
	depth := 0
	for i := open; i < len(value); i++ {
		switch value[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// serialized is q as name's computed style writes it: a number clamped
// or rounded as the property asks, alone when it is the whole value.
func serialized(name string, q quantity, alone bool) (string, bool) {
	v := q.v
	if q.unit == "" && alone {
		switch {
		case unitRanges[name]:
			if math.IsNaN(v) {
				v = 0
			}
			v = min(max(v, 0), 1)
		case integers[name]:
			v = math.Floor(v + 0.5)
		}
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "", false
	}
	return computedNumber(v) + q.unit, true
}

// computedNumber is v as a browser writes a computed number: six significant
// digits at most, without an exponent.
func computedNumber(v float64) string {
	if v == 0 {
		return "0"
	}
	rounded, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'g', 6, 64), 64)
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

// calcParser reads one math function of src from at.
type calcParser struct {
	src   string
	at    int
	fonts fontSizes
}

func (p *calcParser) space() {
	for p.at < len(p.src) && strings.IndexByte(" \t\n\r\f", p.src[p.at]) >= 0 {
		p.at++
	}
}

func (p *calcParser) peek() byte {
	p.space()
	if p.at < len(p.src) {
		return p.src[p.at]
	}
	return 0
}

func (p *calcParser) eat(c byte) bool {
	if p.peek() == c {
		p.at++
		return true
	}
	return false
}

func (p *calcParser) name() string {
	p.space()
	start := p.at
	for p.at < len(p.src) && isNameByte(p.src[p.at]) && (p.at > start || p.src[p.at] < '0' || p.src[p.at] > '9') {
		p.at++
	}
	return strings.ToLower(p.src[start:p.at])
}

// call reads a math function by its name and its arguments.
func (p *calcParser) call() (quantity, error) {
	fn := p.name()
	if mathFunctions[fn] == nil || !p.eat('(') {
		return quantity{}, errUnworked
	}
	strategy := ""
	if fn == "round" {
		save := p.at
		switch s := p.name(); s {
		case "nearest", "up", "down", "to-zero":
			if !p.eat(',') {
				return quantity{}, errUnworked
			}
			strategy = s
		default:
			p.at = save
		}
	}
	var args []quantity
	for {
		q, err := p.sum()
		if err != nil {
			return quantity{}, err
		}
		args = append(args, q)
		if p.eat(')') {
			break
		}
		if !p.eat(',') {
			return quantity{}, errUnworked
		}
	}
	return apply(fn, strategy, args)
}

func (p *calcParser) sum() (quantity, error) {
	q, err := p.product()
	for err == nil {
		c := p.peek()
		if c != '+' && c != '-' {
			break
		}
		p.at++
		var r quantity
		if r, err = p.product(); err != nil {
			break
		}
		if q.unit != r.unit {
			return quantity{}, errUnworked
		}
		if c == '-' {
			r.v = -r.v
		}
		q.v += r.v
	}
	return q, err
}

func (p *calcParser) product() (quantity, error) {
	q, err := p.value()
	for err == nil {
		c := p.peek()
		if c != '*' && c != '/' {
			break
		}
		p.at++
		var r quantity
		if r, err = p.value(); err != nil {
			break
		}
		switch {
		case c == '*' && q.unit == "":
			q = quantity{q.v * r.v, r.unit}
		case c == '*' && r.unit == "":
			q.v *= r.v
		case c == '/' && r.unit == "":
			q.v /= r.v
		case c == '/' && r.unit == q.unit:
			q = quantity{q.v / r.v, ""}
		default:
			return quantity{}, errUnworked
		}
	}
	return q, err
}

//nolint:gochecknoglobals // a constant table
var constants = map[string]float64{
	"e": math.E, "pi": math.Pi, "infinity": math.Inf(1), "-infinity": math.Inf(-1), "nan": math.NaN(),
}

func (p *calcParser) value() (quantity, error) {
	switch c := p.peek(); {
	case c == '(':
		p.at++
		q, err := p.sum()
		if err != nil || !p.eat(')') {
			return quantity{}, errUnworked
		}
		return q, nil
	case c >= '0' && c <= '9' || c == '.' || c == '+' || (c == '-' && p.at+1 < len(p.src) &&
		(p.src[p.at+1] >= '0' && p.src[p.at+1] <= '9' || p.src[p.at+1] == '.')):
		return p.dimension()
	}
	save := p.at
	word := p.name()
	if p.peek() == '(' {
		p.at = save
		return p.call()
	}
	if v, ok := constants[word]; ok {
		return quantity{v: v}, nil
	}
	return quantity{}, errUnworked
}

func (p *calcParser) dimension() (quantity, error) {
	start := p.at
	if c := p.src[p.at]; c == '+' || c == '-' {
		p.at++
	}
	digits := func() {
		for p.at < len(p.src) && p.src[p.at] >= '0' && p.src[p.at] <= '9' {
			p.at++
		}
	}
	digits()
	if p.at < len(p.src) && p.src[p.at] == '.' {
		p.at++
		digits()
	}
	if p.at+1 < len(p.src) && (p.src[p.at] == 'e' || p.src[p.at] == 'E') {
		next := p.src[p.at+1]
		if next >= '0' && next <= '9' || (next == '+' || next == '-') && p.at+2 < len(p.src) &&
			p.src[p.at+2] >= '0' && p.src[p.at+2] <= '9' {
			p.at += 2
			digits()
		}
	}
	v, err := strconv.ParseFloat(p.src[start:p.at], 64)
	if err != nil {
		return quantity{}, errUnworked
	}
	if p.at < len(p.src) && p.src[p.at] == '%' {
		p.at++
		return quantity{v, "%"}, nil
	}
	unitStart := p.at
	for p.at < len(p.src) && isNameByte(p.src[p.at]) && p.src[p.at] != '-' {
		p.at++
	}
	return p.canonical(v, strings.ToLower(p.src[unitStart:p.at]))
}

// canonical is v of unit in the canonical unit of its kind.
func (p *calcParser) canonical(v float64, unit string) (quantity, error) {
	switch {
	case unit == "em" && p.fonts.em > 0:
		return quantity{v * p.fonts.em, unitPx}, nil
	case unit == "rem" && p.fonts.rem > 0:
		return quantity{v * p.fonts.rem, unitPx}, nil
	}
	switch unit {
	case "", unitPx, unitDeg, unitS:
		return quantity{v, unit}, nil
	case "rad":
		return degrees(v), nil
	case "grad":
		return quantity{v * 0.9, unitDeg}, nil
	case "turn":
		return quantity{v * 360, unitDeg}, nil
	case "ms":
		return quantity{v / 1000, unitS}, nil
	}
	return quantity{}, errUnworked
}

func radians(q quantity) (float64, error) {
	switch q.unit {
	case "":
		return q.v, nil
	case unitDeg:
		return q.v * math.Pi / 180, nil
	}
	return 0, errUnworked
}

func degrees(rad float64) quantity { return quantity{rad * 180 / math.Pi, unitDeg} }

func sameUnit(args []quantity) bool {
	for _, a := range args[1:] {
		if a.unit != args[0].unit {
			return false
		}
	}
	return true
}

// mathFunc works a math function out of its arguments, of one unit
// unless it takes angles or numbers, and the rounding strategy round
// names.
type mathFunc func(strategy string, args []quantity) (quantity, error)

// numeric is a function of n numbers.
func numeric(n int, do func(args []float64) float64) mathFunc {
	return func(_ string, args []quantity) (quantity, error) {
		if len(args) != n || args[0].unit != "" {
			return quantity{}, errUnworked
		}
		v := make([]float64, n)
		for i, a := range args {
			v[i] = a.v
		}
		return quantity{v: do(v)}, nil
	}
}

// ofOneUnit is a function of n quantities of one unit, which it keeps.
func ofOneUnit(n int, do func(args []float64) float64) mathFunc {
	return func(_ string, args []quantity) (quantity, error) {
		if n > 0 && len(args) != n {
			return quantity{}, errUnworked
		}
		v := make([]float64, len(args))
		for i, a := range args {
			v[i] = a.v
		}
		return quantity{do(v), args[0].unit}, nil
	}
}

// trigonometric is sin, cos or tan of a number of radians or an angle.
func trigonometric(do func(float64) float64) mathFunc {
	return func(_ string, args []quantity) (quantity, error) {
		if len(args) != 1 {
			return quantity{}, errUnworked
		}
		rad, err := radians(args[0])
		return quantity{v: do(rad)}, err
	}
}

// inverse is asin, acos or atan of a number, an angle.
func inverse(do func(float64) float64) mathFunc {
	return func(_ string, args []quantity) (quantity, error) {
		if len(args) != 1 || args[0].unit != "" {
			return quantity{}, errUnworked
		}
		return degrees(do(args[0].v)), nil
	}
}

// mathFunctions are the functions of CSS whose value a computed style
// gives worked out, by their names.
//
//nolint:gochecknoglobals // a constant table
var mathFunctions = map[string]mathFunc{
	"calc":  ofOneUnit(1, func(v []float64) float64 { return v[0] }),
	"min":   ofOneUnit(0, slices.Min),
	"max":   ofOneUnit(0, slices.Max),
	"clamp": ofOneUnit(3, func(v []float64) float64 { return math.Max(v[0], math.Min(v[1], v[2])) }),
	"round": rounded,
	"mod":   ofOneUnit(2, func(v []float64) float64 { return v[0] - v[1]*math.Floor(v[0]/v[1]) }),
	"rem":   ofOneUnit(2, func(v []float64) float64 { return math.Mod(v[0], v[1]) }),
	"abs":   ofOneUnit(1, func(v []float64) float64 { return math.Abs(v[0]) }),
	"sign": func(_ string, args []quantity) (quantity, error) {
		if len(args) != 1 {
			return quantity{}, errUnworked
		}
		switch v := args[0].v; {
		case v > 0:
			return quantity{v: 1}, nil
		case v < 0:
			return quantity{v: -1}, nil
		}
		return quantity{}, nil
	},
	"exp":  numeric(1, func(v []float64) float64 { return math.Exp(v[0]) }),
	"log":  logarithm,
	"pow":  numeric(2, func(v []float64) float64 { return math.Pow(v[0], v[1]) }),
	"sqrt": numeric(1, func(v []float64) float64 { return math.Sqrt(v[0]) }),
	"hypot": ofOneUnit(0, func(v []float64) float64 {
		sum := 0.0
		for _, x := range v {
			sum += x * x
		}
		return math.Sqrt(sum)
	}),
	"sin":  trigonometric(math.Sin),
	"cos":  trigonometric(math.Cos),
	"tan":  trigonometric(math.Tan),
	"asin": inverse(math.Asin),
	"acos": inverse(math.Acos),
	"atan": inverse(math.Atan),
	"atan2": func(_ string, args []quantity) (quantity, error) {
		if len(args) != 2 {
			return quantity{}, errUnworked
		}
		return degrees(math.Atan2(args[0].v, args[1].v)), nil
	},
	"progress": func(_ string, args []quantity) (quantity, error) {
		if len(args) != 3 {
			return quantity{}, errUnworked
		}
		return quantity{v: (args[0].v - args[1].v) / (args[2].v - args[1].v)}, nil
	},
}

// logarithm is log(a), natural, or log(a, base).
func logarithm(strategy string, args []quantity) (quantity, error) {
	if len(args) == 2 {
		return numeric(2, func(v []float64) float64 { return math.Log(v[0]) / math.Log(v[1]) })(strategy, args)
	}
	return numeric(1, func(v []float64) float64 { return math.Log(v[0]) })(strategy, args)
}

// apply works out the math function fn of args.
func apply(fn, strategy string, args []quantity) (quantity, error) {
	do, ok := mathFunctions[fn]
	if !ok || !sameUnit(args) {
		return quantity{}, errUnworked
	}
	return do(strategy, args)
}

// rounded is round(strategy, a, b): a to a multiple of b, 1 for a
// number when b is left out.
func rounded(strategy string, args []quantity) (quantity, error) {
	a := args[0]
	step := quantity{v: 1}
	switch {
	case len(args) == 2:
		step = args[1]
	case len(args) != 1 || a.unit != "":
		return quantity{}, errUnworked
	}
	if step.v == 0 {
		return quantity{math.NaN(), a.unit}, nil
	}
	n := a.v / step.v
	switch strategy {
	case "up":
		n = math.Ceil(n)
	case "down":
		n = math.Floor(n)
	case "to-zero":
		n = math.Trunc(n)
	default:
		n = math.Floor(n + 0.5)
	}
	return quantity{n * step.v, a.unit}, nil
}
