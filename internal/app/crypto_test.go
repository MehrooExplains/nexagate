package app

import (
	"bytes"
	"encoding/hex"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPasswordHashAndSession(t *testing.T) {
	hash, err := hashPassword("a-long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(hash, "a-long-test-password") {
		t.Fatal("correct password was rejected")
	}
	if verifyPassword(hash, "wrong-password") {
		t.Fatal("wrong password was accepted")
	}

	key := bytes.Repeat([]byte{0x42}, 32)
	now := time.Unix(1_700_000_000, 0)
	signed := signSession(key, now.Add(time.Hour), "nonce")
	if nonce, ok := verifySession(key, signed, now); !ok || nonce != "nonce" {
		t.Fatal("valid session was rejected")
	}
	if _, ok := verifySession(key, signed, now.Add(2*time.Hour)); ok {
		t.Fatal("expired session was accepted")
	}
	if _, ok := verifySession(key, signed+"tampered", now); ok {
		t.Fatal("tampered session was accepted")
	}
}

func TestPBKDF2KnownVector(t *testing.T) {
	want, err := hex.DecodeString("ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43")
	if err != nil {
		t.Fatal(err)
	}
	got := pbkdf2SHA256([]byte("password"), []byte("salt"), 2, 32)
	if !bytes.Equal(got, want) {
		t.Fatalf("PBKDF2 vector mismatch: %x", got)
	}
}

func TestClientIPTrustsProxyHeaderOnlyFromLoopback(t *testing.T) {
	request := httptest.NewRequest("GET", "https://example.test/", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("X-Real-IP", "203.0.113.9")
	if got := clientIP(request); got != "203.0.113.9" {
		t.Fatalf("unexpected proxied client IP: %s", got)
	}
	request.RemoteAddr = "198.51.100.8:54321"
	if got := clientIP(request); got != "198.51.100.8" {
		t.Fatalf("untrusted proxy header was accepted: %s", got)
	}
}
