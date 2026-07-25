package crypto

import (
	"strings"
	"testing"
)

func TestSealRoundTrip(t *testing.T) {
	key, err := GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSealer(key)
	if err != nil {
		t.Fatal(err)
	}
	ct, nonce, err := s.Seal([]byte("sk-or-v1-supersecret"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ct), "supersecret") {
		t.Fatal("ciphertext leaks plaintext")
	}
	got, err := s.Open(ct, nonce)
	if err != nil || string(got) != "sk-or-v1-supersecret" {
		t.Fatalf("open: %q err=%v", got, err)
	}
}

func TestWrongKeyFails(t *testing.T) {
	k1, _ := GenerateMasterKey()
	k2, _ := GenerateMasterKey()
	s1, _ := NewSealer(k1)
	s2, _ := NewSealer(k2)
	ct, nonce, _ := s1.Seal([]byte("data"))
	if _, err := s2.Open(ct, nonce); err == nil {
		t.Fatal("wrong master key must fail to open")
	}
}

func TestBadKeys(t *testing.T) {
	if _, err := NewSealer(""); err == nil {
		t.Fatal("empty key must fail")
	}
	if _, err := NewSealer("not-base64!!"); err == nil {
		t.Fatal("non-base64 must fail")
	}
	if _, err := NewSealer("c2hvcnQ="); err == nil {
		t.Fatal("short key must fail")
	}
}

func TestMask(t *testing.T) {
	if got := Mask("sk-or-v1-abcdef123456"); got != "sk-or…3456" {
		t.Fatalf("mask: %q", got)
	}
	if got := Mask("tiny"); got != "****" {
		t.Fatalf("short mask: %q", got)
	}
}
