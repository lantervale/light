package idb_test

import (
	"math"
	"testing"

	"github.com/lantervale/light/pkg/idb"
)

func TestKeysCompareInTheOrderOfTheStandard(t *testing.T) {
	t.Parallel()
	ordered := []idb.Key{
		idb.Number(math.Inf(-1)), idb.Number(-1e300), idb.Number(-1), idb.Number(-0.5), idb.Number(0),
		idb.Number(5e-324), idb.Number(1), idb.Number(1.5), idb.Number(2), idb.Number(1 << 53), idb.Number(math.Inf(1)),
		idb.Date(math.Inf(-1)), idb.Date(-1), idb.Date(0), idb.Date(1.7e12),
		idb.Text(""), idb.Text("\x00"), idb.Text("\x00\x00"), idb.Text("\x01"), idb.Text("A"), idb.Text("a"),
		idb.Text("a\x00"), idb.Text("aa"), idb.Text("b"), idb.Text("~"), idb.Text("\x7f"), idb.Text("é"),
		idb.Text("䁾"), idb.Text("䁿"), idb.Text("䂀"),
		idb.Text("𐀀"), idb.Units([]uint16{0xdbff}), idb.Text("￿"),
		idb.Binary(nil), idb.Binary([]byte{0}), idb.Binary([]byte{0, 0}), idb.Binary([]byte{0, 1}),
		idb.Binary([]byte{1}), idb.Binary([]byte{1, 0}), idb.Binary([]byte{2}), idb.Binary([]byte{255}),
		idb.Array(), idb.Array(idb.Number(1)), idb.Array(idb.Number(1), idb.Number(0)),
		idb.Array(idb.Number(1), idb.Text("a")), idb.Array(idb.Number(2)), idb.Array(idb.Text("")),
		idb.Array(idb.Text("a")), idb.Array(idb.Text("a"), idb.Number(1)), idb.Array(idb.Text("aa")),
		idb.Array(idb.Binary([]byte{0})), idb.Array(idb.Array()), idb.Array(idb.Array(), idb.Number(1)),
		idb.Array(idb.Array(idb.Number(1))),
	}
	for i, k := range ordered {
		if !k.Valid() {
			t.Errorf("%s (%d) is said to be no key", k, i)
		}
		if i > 0 && ordered[i-1] >= k {
			t.Errorf("%s is not before %s", ordered[i-1], k)
		}
	}
	if idb.Number(math.Copysign(0, -1)) != idb.Number(0) {
		t.Error("-0 and 0 are two keys")
	}
}

func TestWhatIsNoKey(t *testing.T) {
	t.Parallel()
	if idb.Number(math.NaN()) != "" || idb.Date(math.NaN()) != "" || idb.Array(idb.Number(1), "") != "" {
		t.Error("NaN, a date that is none or an array of one made a key")
	}
	number := string(idb.Number(1))
	deep := idb.Number(1)
	for range 600 {
		deep = idb.Array(deep)
	}
	for _, k := range []idb.Key{
		"", "\x00", "\x10", idb.Key(number[:8]), idb.Key(number + "x"), "\x10\xff\xf8\x00\x00\x00\x00\x00\x00",
		"\x10\x00\x07\xff\xff\xff\xff\xff\xff", "\x30a", "\x30\x80", "\x30\xc0\x00\x00\x00", "\x30\xff\x00",
		"\x40\x05", "\x40\x01\x03\x00", "\x40\x01", "\x50", "\x50\x10\x00", idb.Key("\x50" + number), "\x60\x00", deep,
	} {
		if k.Valid() {
			t.Errorf("%q is said to be a key", string(k))
		}
	}
}

func TestKeysReadAsADeveloperWritesThem(t *testing.T) {
	t.Parallel()
	for want, k := range map[string]idb.Key{
		"3":                  idb.Number(3),
		"-0.5":               idb.Number(-0.5),
		"@86400000":          idb.Date(86400000),
		`"a\x00é𐀀"`:          idb.Text("a\x00é𐀀"),
		"0x0001ff":           idb.Binary([]byte{0, 1, 255}),
		`[1, "b", [0x, @0]]`: idb.Array(idb.Number(1), idb.Text("b"), idb.Array(idb.Binary(nil), idb.Date(0))),
		"?":                  "\x99",
		`"䁾䁿"`:               idb.Text("\u407e\u407f"),
	} {
		if got := k.String(); got != want {
			t.Errorf("%q reads %s, not %s", string(k), got, want)
		}
	}
}

func TestRangesInclude(t *testing.T) {
	t.Parallel()
	one, two, three := idb.Number(1), idb.Number(2), idb.Number(3)
	for _, c := range []struct {
		r    idb.Range
		k    idb.Key
		want bool
	}{
		{idb.Range{}, one, true},
		{idb.Only(two), two, true}, {idb.Only(two), one, false}, {idb.Only(two), three, false},
		{idb.Range{Lower: two}, two, true}, {idb.Range{Lower: two, LowerOpen: true}, two, false},
		{idb.Range{Lower: two}, one, false}, {idb.Range{Lower: two}, idb.Text("a"), true},
		{idb.Range{Upper: two}, two, true}, {idb.Range{Upper: two, UpperOpen: true}, two, false},
		{idb.Range{Upper: two}, three, false}, {idb.Range{Lower: one, Upper: three}, two, true},
	} {
		if got := c.r.Includes(c.k); got != c.want {
			t.Errorf("%+v includes %s: %v", c.r, c.k, got)
		}
	}
}
