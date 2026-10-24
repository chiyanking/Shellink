package store

import (
	"crypto/sha256"
	"testing"
)

func TestNodeInterop(t *testing.T) {
	box := NewSecretBox(sha256.Sum256([]byte("interop-key")))
	v, e := box.Decrypt("BwcHBwcHBwcHBwcHuDosXAIoo7RWmWgXfiEk7bukxlTc1XCBE6MDCfY=")
	if e != nil || v != "secret-中文" {
		t.Fatalf("%q %v", v, e)
	}
}
func TestSecretRoundTrip(t *testing.T) {
	box := NewSecretBox(InsecureDefaultKey())
	for _, v := range []string{"", "hello", "中文\x00"} {
		encoded, e := box.Encrypt(v)
		if e != nil {
			t.Fatal(e)
		}
		got, e := box.Decrypt(encoded)
		if e != nil || got != v {
			t.Fatalf("%q %v", got, e)
		}
	}
	for _, v := range []string{"invalid!", "AA==", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="} {
		if _, e := box.Decrypt(v); e == nil {
			t.Fatal("accepted corrupt secret")
		}
	}
}
func TestMasterKey(t *testing.T) {
	key, e := MasterKeyFromHex("0101010101010101010101010101010101010101010101010101010101010101")
	if e != nil || key[0] != 1 || key[31] != 1 {
		t.Fatal(key, e)
	}
	if _, e := MasterKeyFromHex("ab"); e == nil {
		t.Fatal("accepted short key")
	}
}
