package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptTextRoundTrip(t *testing.T) {
	plain := "my super secret token: abc123"
	enc, err := EncryptText("hunter2", plain)
	if err != nil {
		t.Fatalf("EncryptText: %v", err)
	}
	if !IsEncryptedText(enc) {
		t.Fatalf("IsEncryptedText = false for encrypted output %q", enc)
	}
	if enc == plain || bytes.Contains([]byte(enc), []byte("super secret")) {
		t.Fatalf("ciphertext leaks plaintext: %q", enc)
	}
	got, err := DecryptText("hunter2", enc)
	if err != nil {
		t.Fatalf("DecryptText: %v", err)
	}
	if got != plain {
		t.Fatalf("round trip mismatch: got %q want %q", got, plain)
	}
}

func TestDecryptTextWrongPassphrase(t *testing.T) {
	enc, err := EncryptText("right", "secret")
	if err != nil {
		t.Fatalf("EncryptText: %v", err)
	}
	if _, err := DecryptText("wrong", enc); err != ErrWrongPassphrase {
		t.Fatalf("expected ErrWrongPassphrase, got %v", err)
	}
}

func TestEncryptTextRandomized(t *testing.T) {
	// Same input + passphrase must produce different ciphertext (random salt/nonce).
	a, _ := EncryptText("p", "same")
	b, _ := EncryptText("p", "same")
	if a == b {
		t.Fatal("two encryptions of the same plaintext are identical (salt/nonce not random)")
	}
}

func TestIsEncryptedText(t *testing.T) {
	if IsEncryptedText("just some plain text") {
		t.Fatal("plain text reported as encrypted")
	}
	enc, _ := EncryptText("p", "x")
	if !IsEncryptedText(enc) {
		t.Fatal("encrypted text not detected")
	}
}

func TestEncryptDecryptBytesRoundTrip(t *testing.T) {
	data := bytes.Repeat([]byte{0x00, 0x01, 0x02, 0xff, 0xab}, 1000)
	enc, err := EncryptBytes("pw", data)
	if err != nil {
		t.Fatalf("EncryptBytes: %v", err)
	}
	if !IsEncryptedBytes(enc) {
		t.Fatal("IsEncryptedBytes = false for encrypted output")
	}
	if bytes.Contains(enc, data[:50]) {
		t.Fatal("ciphertext leaks plaintext bytes")
	}
	got, err := DecryptBytes("pw", enc)
	if err != nil {
		t.Fatalf("DecryptBytes: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("byte round trip mismatch")
	}
}

func TestDecryptBytesWrongPassphrase(t *testing.T) {
	enc, _ := EncryptBytes("right", []byte("payload"))
	if _, err := DecryptBytes("wrong", enc); err != ErrWrongPassphrase {
		t.Fatalf("expected ErrWrongPassphrase, got %v", err)
	}
}

func TestIsEncryptedBytes(t *testing.T) {
	if IsEncryptedBytes([]byte("plain file contents")) {
		t.Fatal("plain bytes reported as encrypted")
	}
	if IsEncryptedBytes([]byte{0x01}) {
		t.Fatal("short buffer reported as encrypted")
	}
}
