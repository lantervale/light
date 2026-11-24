//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package script_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/script"
)

func TestBytesCrossBothWaysThroughASharedBuffer(t *testing.T) {
	ctx := open(t, script.Options{})
	every := make([]byte, 256)
	for i := range every {
		every[i] = byte(i)
	}
	var got []byte
	err := ctx.Bind("host", func(args []any) (any, error) {
		shared, _ := args[1].([]byte)
		if args[0] == "take" {
			got = bytes.Clone(shared)
			return len(shared), nil
		}
		return copy(shared, every), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	back, err := ctx.Run(`
		const filled = new SharedArrayBuffer(256);
		const wrote = host("fill", filled);
		const bytes = new Uint8Array(filled);
		const sent = new SharedArrayBuffer(4);
		new Uint8Array(sent).set([0, 255, 128, 10]);
		[wrote, bytes[0], bytes[127], bytes[128], bytes[255], host("take", sent),
			host("take", new SharedArrayBuffer(0))].join()`,
		"test.js")
	if err != nil || back != "256,0,127,128,255,4,0" {
		t.Fatalf("the script read %v, %v", back, err)
	}
	if len(got) != 0 {
		t.Errorf("an empty buffer came as %d bytes", len(got))
	}
	if _, err = ctx.Run(`host("take", sent)`, "test.js"); err != nil || !bytes.Equal(got, []byte{0, 255, 128, 10}) {
		t.Errorf("the function was given %v, %v", got, err)
	}
}

func TestMegabytesCrossInMilliseconds(t *testing.T) {
	ctx := open(t, script.Options{})
	big := make([]byte, 5<<20)
	for i := range big {
		big[i] = byte(i * 7)
	}
	var back []byte
	if err := ctx.Bind("host", func(args []any) (any, error) {
		shared, _ := args[1].([]byte)
		if args[0] == "take" {
			back = bytes.Clone(shared)
			return len(back), nil
		}
		return copy(shared, big), nil
	}); err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	sum, err := ctx.Run(`
		const shared = new SharedArrayBuffer(5 << 20);
		host("fill", shared);
		const own = new Uint8Array(5 << 20);
		own.set(new Uint8Array(shared));
		const again = new SharedArrayBuffer(own.length);
		new Uint8Array(again).set(own);
		host("take", again);
		own[1] + own[own.length - 1]`, "test.js")
	took := time.Since(began)
	if err != nil || sum != float64(big[1])+float64(big[len(big)-1]) || !bytes.Equal(back, big) {
		t.Fatalf("5 MB came back changed: %v, %v", sum, err)
	}
	t.Logf("5 MB to the script and back took %s", took)
	if took > time.Second {
		t.Errorf("5 MB to the script and back took %s", took)
	}
}
