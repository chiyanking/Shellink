package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

type SecretBox struct{ key [32]byte }

func NewSecretBox(key [32]byte) SecretBox { return SecretBox{key} }
func InsecureDefaultKey() [32]byte        { return sha256.Sum256([]byte("shellink-insecure-default-key")) }

// MasterKeyFromHex matches TS parseMasterKey: the decoded bytes are the key.
func MasterKeyFromHex(s string) ([32]byte, error) {
	if s == "" {
		return InsecureDefaultKey(), nil
	}
	var key [32]byte
	b, e := hex.DecodeString(s)
	if e != nil || len(b) != 32 {
		return key, fmt.Errorf("SHELLINK_MASTER_KEY must be a 32-byte hex string (64 hex characters)")
	}
	copy(key[:], b)
	return key, nil
}
func (s SecretBox) aead() (cipher.AEAD, error) {
	block, e := aes.NewCipher(s.key[:])
	if e != nil {
		return nil, e
	}
	return cipher.NewGCM(block)
}
func (s SecretBox) Encrypt(plain string) (string, error) {
	a, e := s.aead()
	if e != nil {
		return "", e
	}
	nonce := make([]byte, a.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return "", e
	}
	sealed := a.Seal(nil, nonce, []byte(plain), nil)
	n := len(sealed) - a.Overhead()
	out := append(nonce, sealed[n:]...)
	out = append(out, sealed[:n]...)
	return base64.StdEncoding.EncodeToString(out), nil
}
func (s SecretBox) Decrypt(payload string) (string, error) {
	b, e := base64.StdEncoding.DecodeString(payload)
	if e != nil {
		return "", e
	}
	a, e := s.aead()
	if e != nil {
		return "", e
	}
	n, tag := a.NonceSize(), a.Overhead()
	if len(b) < n+tag {
		return "", fmt.Errorf("truncated encrypted secret")
	}
	sealed := append([]byte(nil), b[n+tag:]...)
	sealed = append(sealed, b[n:n+tag]...)
	plain, e := a.Open(nil, b[:n], sealed, nil)
	if e != nil {
		return "", e
	}
	return string(plain), nil
}
