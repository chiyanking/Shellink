package protocol

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestGoldenFrame(t *testing.T) {
	f, e := EncodeFrame(Request, []KV{{"id", int64(1)}, {"method", "system.ping"}, {"params", []KV{}}})
	if e != nil {
		t.Fatal(e)
	}
	want, _ := hex.DecodeString("5350494c010001002000000083a2696401a66d6574686f64ab73797374656d2e70696e67a6706172616d7380")
	if !bytes.Equal(f, want) {
		t.Fatalf("got %x want %x", f, want)
	}
}
func TestFrameSplitsAndSticky(t *testing.T) {
	a, _ := EncodeFrame(Request, []KV{{"id", "a"}})
	b, _ := EncodeFrame(Response, []KV{{"id", "a"}, {"ok", true}})
	for i := 1; i < len(a); i++ {
		d := NewFrameDecoder(0)
		if got, _ := d.Push(a[:i]); len(got) != 0 {
			t.Fatal(i)
		}
		got, e := d.Push(append(a[i:], b...))
		if e != nil || len(got) != 2 {
			t.Fatalf("split %d: %v %d", i, e, len(got))
		}
	}
}
func TestInvalidFrame(t *testing.T) {
	f, _ := EncodeFrame(Request, []KV{})
	for _, mut := range []func([]byte){func(x []byte) { x[0] = 0 }, func(x []byte) { x[4] = 2 }, func(x []byte) { x[6] = 9 }, func(x []byte) { x[7] = 1 }} {
		x := append([]byte(nil), f...)
		mut(x)
		if _, e := NewFrameDecoder(0).Push(x); e == nil {
			t.Fatal("expected error")
		}
	}
}
func TestBinaryRoundTrip(t *testing.T) {
	f, _ := EncodeFrame(Response, []KV{{"data", []byte{0, 1, 255}}})
	got, e := NewFrameDecoder(0).Push(f)
	if e != nil {
		t.Fatal(e)
	}
	m := got[0].Payload.([]KV)
	if !bytes.Equal(m[0].Val.([]byte), []byte{0, 1, 255}) {
		t.Fatal(m)
	}
}
func TestTruncated(t *testing.T) {
	f, _ := EncodeFrame(Request, []KV{})
	d := NewFrameDecoder(0)
	d.Push(f[:len(f)-1])
	if e := d.Finish(); e == nil {
		t.Fatal("expected truncation")
	}
}
