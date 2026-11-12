package crypto

import (
	"strings"
	"testing"
)

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestRoundTrip(t *testing.T) {
	c, err := NewCipher(testKey)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	for _, plaintext := range []string{"", "access-sandbox-1234", strings.Repeat("x", 4096)} {
		enc, err := c.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", plaintext, err)
		}
		if enc == plaintext && plaintext != "" {
			t.Fatalf("ciphertext equals plaintext")
		}
		dec, err := c.Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if dec != plaintext {
			t.Fatalf("round trip mismatch: got %q want %q", dec, plaintext)
		}
	}
}

func TestEncryptIsNonDeterministic(t *testing.T) {
	c, _ := NewCipher(testKey)
	a, _ := c.Encrypt("same input")
	b, _ := c.Encrypt("same input")
	if a == b {
		t.Fatal("two encryptions of the same plaintext produced identical ciphertexts (nonce reuse?)")
	}
}

func TestTamperDetection(t *testing.T) {
	c, _ := NewCipher(testKey)
	enc, _ := c.Encrypt("secret")
	tampered := []byte(enc)
	tampered[len(tampered)-5] ^= 'x'
	if _, err := c.Decrypt(string(tampered)); err == nil {
		t.Fatal("expected error decrypting tampered ciphertext")
	}
}

func TestWrongKeyFails(t *testing.T) {
	c1, _ := NewCipher(testKey)
	c2, _ := NewCipher(strings.Repeat("ff", 32))
	enc, _ := c1.Encrypt("secret")
	if _, err := c2.Decrypt(enc); err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestBadKeys(t *testing.T) {
	for _, key := range []string{"", "abc", "zz" + strings.Repeat("00", 31), strings.Repeat("00", 16)} {
		if _, err := NewCipher(key); err == nil {
			t.Fatalf("expected error for key %q", key)
		}
	}
}
