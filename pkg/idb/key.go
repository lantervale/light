package idb

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Key is a key of a record as the store holds it: written so that two
// keys compare as their texts do, byte by byte, in the order the
// standard gives them (a number, then a date, a string, a binary, an
// array). "" is no key.
type Key string

// The first byte of a written key is its kind.
const (
	kindNumber = 0x10
	kindDate   = 0x20
	kindString = 0x30
	kindBinary = 0x40
	kindArray  = 0x50
	// end closes a string, a binary and an array: below any byte that
	// goes on.
	end = 0x00
	// maxDepth is how deep the arrays of a key may be in one another.
	maxDepth = 512
)

// Number is the key of a number, "" for NaN.
func Number(n float64) Key { return scalar(kindNumber, n) }

// Date is the key of a date, by its milliseconds since 1970, "" for
// one that is no date.
func Date(ms float64) Key { return scalar(kindDate, ms) }

func scalar(kind byte, n float64) Key {
	if math.IsNaN(n) {
		return ""
	}
	if n == 0 {
		n = 0
	}
	bits := math.Float64bits(n)
	if bits>>63 == 0 {
		bits |= 1 << 63
	} else {
		bits = ^bits
	}
	out := make([]byte, 9)
	out[0] = kind
	binary.BigEndian.PutUint64(out[1:], bits)
	return Key(out)
}

// Text is the key of a string.
func Text(text string) Key { return Units(utf16.Encode([]rune(text))) }

// Units is the key of a string by its UTF-16 code units, which it is
// ordered by.
func Units(units []uint16) Key {
	out := make([]byte, 0, len(units)+2)
	out = append(out, kindString)
	for _, u := range units {
		switch {
		case u < 0x7f:
			out = append(out, byte(u)+1)
		case u < 0x7f+0x4000:
			u -= 0x7f
			out = append(out, 0x80|byte(u>>8), byte(u&0xff))
		default:
			out = append(out, 0xc0, byte(u>>8), byte(u&0xff))
		}
	}
	return Key(append(out, end))
}

// Binary is the key of bytes.
func Binary(bytes []byte) Key {
	out := make([]byte, 0, len(bytes)+2)
	out = append(out, kindBinary)
	for _, b := range bytes {
		if b < 2 {
			out = append(out, 1, b+1)
		} else {
			out = append(out, b)
		}
	}
	return Key(append(out, end))
}

// Array is the key of a list of keys, "" when one of them is none.
func Array(keys ...Key) Key {
	var out strings.Builder
	out.WriteByte(kindArray)
	for _, k := range keys {
		if k == "" {
			return ""
		}
		out.WriteString(string(k))
	}
	out.WriteByte(end)
	return Key(out.String())
}

// Valid reports whether k is a key written as this package writes
// them, whole.
func (k Key) Valid() bool {
	rest, ok := skip(string(k), 0)
	return ok && rest == ""
}

// skip is what follows the key s begins with.
func skip(s string, depth int) (string, bool) {
	if s == "" || depth > maxDepth {
		return "", false
	}
	switch s[0] {
	case kindNumber, kindDate:
		if len(s) < 9 || s[1:9] > "\xff\xf0\x00\x00\x00\x00\x00\x00" || s[1:9] < "\x00\x0f\xff\xff\xff\xff\xff\xff" {
			return "", false
		}
		return s[9:], true
	case kindString:
		return skipUnits(s[1:])
	case kindBinary:
		return skipBytes(s[1:])
	case kindArray:
		s = s[1:]
		for s != "" && s[0] != end {
			var ok bool
			if s, ok = skip(s, depth+1); !ok {
				return "", false
			}
		}
		if s == "" {
			return "", false
		}
		return s[1:], true
	}
	return "", false
}

func skipUnits(s string) (string, bool) {
	for i := 0; i < len(s); {
		switch b := s[i]; {
		case b == end:
			return s[i+1:], true
		case b < 0x80:
			i++
		case b < 0xc0:
			i += 2
		case b == 0xc0:
			if i+2 >= len(s) || s[i+1] < 0x40 || (s[i+1] == 0x40 && s[i+2] < 0x7f) {
				return "", false
			}
			i += 3
		default:
			return "", false
		}
	}
	return "", false
}

func skipBytes(s string) (string, bool) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case end:
			return s[i+1:], true
		case 1:
			if i++; i >= len(s) || (s[i] != 1 && s[i] != 2) {
				return "", false
			}
		}
	}
	return "", false
}

// number is the number of a key that is one.
func (k Key) number() (float64, bool) {
	if len(k) != 9 || k[0] != kindNumber {
		return 0, false
	}
	bits := binary.BigEndian.Uint64([]byte(k[1:]))
	if bits>>63 == 1 {
		bits &^= 1 << 63
	} else {
		bits = ^bits
	}
	return math.Float64frombits(bits), true
}

// String is the key as a developer reads it: 3, "text", a date's
// milliseconds after @, a binary's bytes in hexadecimal after 0x, an
// array between brackets.
func (k Key) String() string {
	var out strings.Builder
	if rest, ok := show(&out, string(k)); !ok || rest != "" {
		return "?"
	}
	return out.String()
}

func show(out *strings.Builder, s string) (string, bool) {
	rest, ok := skip(s, 0)
	if !ok {
		return "", false
	}
	body := s[1 : len(s)-len(rest)]
	switch s[0] {
	case kindNumber, kindDate:
		n, _ := (Key(string(rune(kindNumber))) + Key(body)).number()
		if s[0] == kindDate {
			out.WriteByte('@')
		}
		out.WriteString(strconv.FormatFloat(n, 'f', -1, 64))
	case kindString:
		out.WriteString(strconv.Quote(string(utf16.Decode(units(body)))))
	case kindBinary:
		out.WriteString("0x")
		for i := 0; i < len(body)-1; i++ {
			b := body[i]
			if b == 1 {
				i++
				b = body[i] - 1
			}
			out.WriteString(strconv.FormatUint(uint64(b)|0x100, 16)[1:])
		}
	case kindArray:
		out.WriteByte('[')
		for first := true; body[0] != end; first = false {
			if !first {
				out.WriteString(", ")
			}
			body, _ = show(out, body)
		}
		out.WriteByte(']')
	}
	return rest, true
}

// units are the code units of the body of a written string.
func units(body string) []uint16 {
	var out []uint16
	for i := 0; i < len(body)-1; {
		switch b := body[i]; {
		case b < 0x80:
			out = append(out, uint16(b)-1)
			i++
		case b < 0xc0:
			out = append(out, uint16(b&0x3f)<<8+uint16(body[i+1])+0x7f)
			i += 2
		default:
			out = append(out, uint16(body[i+1])<<8+uint16(body[i+2]))
			i += 3
		}
	}
	return out
}

// Range is the keys between two, either left out for none, each within
// the range unless open.
type Range struct {
	Lower, Upper         Key
	LowerOpen, UpperOpen bool
}

// Only is the range of one key.
func Only(k Key) Range { return Range{Lower: k, Upper: k} }

// Includes reports whether k is within the range.
func (r Range) Includes(k Key) bool {
	if r.Lower != "" && (k < r.Lower || (r.LowerOpen && k == r.Lower)) {
		return false
	}
	return r.Upper == "" || k < r.Upper || (!r.UpperOpen && k == r.Upper)
}

func (r Range) valid() bool {
	return (r.Lower == "" || r.Lower.Valid()) && (r.Upper == "" || r.Upper.Valid())
}
