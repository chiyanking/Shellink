package protocol

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestMessagePackGolden(t *testing.T) {
	tests := []struct {
		value Value
		hex   string
	}{
		{[]KV{}, "80"}, {"abc", "a3616263"}, {int64(127), "7f"}, {int64(128), "cc80"},
		{int64(-1), "ff"}, {int64(-128), "d080"}, {int64(-129), "d1ff7f"},
		{int64(65535), "cdffff"}, {int64(65536), "ce00010000"}, {int64(-32768), "d18000"}, {int64(-32769), "d2ffff7fff"},
		{float64(1.5), "cb3ff8000000000000"}, {true, "c3"}, {nil, "c0"},
		{[]byte{1, 2, 3}, "c403010203"}, {[]Value{1, 2, 3}, "93010203"},
		{[]KV{{"a", 1}, {"b", 2}}, "82a16101a16202"},
		{"中文", "a6e4b8ade69687"},
	}
	for _, tt := range tests {
		b, e := EncodeMsgpack(tt.value)
		if e != nil {
			t.Fatal(e)
		}
		if hex.EncodeToString(b) != tt.hex {
			t.Fatalf("%v: %x != %s", tt.value, b, tt.hex)
		}
		v, e := DecodeMsgpack(b)
		if e != nil {
			t.Fatal(e)
		}
		again, e := EncodeMsgpack(v)
		if e != nil || !bytes.Equal(b, again) {
			t.Fatalf("roundtrip %x: %v", b, e)
		}
	}
}

func TestMessagePackRejectsEveryTruncatedPrefix(t *testing.T) {
	for _, v := range []Value{uint64(1 << 63), int64(-1 << 60), float64(1.5), string(bytes.Repeat([]byte("x"), 300)), []byte{1, 2, 3}, []Value{1, "abc"}, []KV{{"test", []Value{1, 2}}}} {
		b, e := EncodeMsgpack(v)
		if e != nil {
			t.Fatal(e)
		}
		for i := 0; i < len(b); i++ {
			if _, e := DecodeMsgpack(b[:i]); e == nil {
				t.Fatalf("accepted truncated prefix %x", b[:i])
			}
		}
	}
}
func FuzzDecodeMsgpack(f *testing.F) {
	for _, b := range [][]byte{{0xc1}, {0xdd, 255, 255, 255, 255}, {0xd9}, {0x81}, bytes.Repeat([]byte{0x91}, 256)} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) { DecodeMsgpack(b) })
}
