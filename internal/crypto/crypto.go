// Package crypto provides zero-knowledge, passphrase-based encryption for nikte
// content. Data is encrypted on the client with a key derived from a passphrase
// (scrypt) and sealed with AES-256-GCM. The server only ever sees ciphertext;
// the passphrase and derived key never leave the machine.
package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	// textPrefix marks an encrypted text short so `nk g` can detect it.
	textPrefix = "nkenc:v1:"
	// FileSuffix marks an encrypted file by name (in addition to the magic header).
	FileSuffix = ".nkenc"

	saltLen  = 16
	nonceLen = 12
	keyLen   = 32

	// scrypt cost parameters (N must be a power of two). N=32768 is a sane
	// interactive default (~tens of ms) while remaining resistant to brute force.
	scryptN = 1 << 15
	scryptR = 8
	scryptP = 1
)

// fileMagic prefixes encrypted file bytes so encryption is self-describing even
// if the .nkenc suffix is stripped.
var fileMagic = []byte("NKENCFL1")

// ErrWrongPassphrase is returned when decryption fails authentication, which
// almost always means a wrong passphrase (or corrupted/altered ciphertext).
var ErrWrongPassphrase = errors.New("decryption failed: wrong passphrase or corrupted data")

func deriveKey(passphrase string, salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, keyLen)
}

// encryptRaw produces salt | nonce | ciphertext.
func encryptRaw(passphrase string, plaintext []byte) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 0, saltLen+nonceLen+len(ciphertext))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

// decryptRaw reverses encryptRaw.
func decryptRaw(passphrase string, blob []byte) ([]byte, error) {
	if len(blob) < saltLen+nonceLen {
		return nil, errors.New("ciphertext too short")
	}
	salt := blob[:saltLen]
	nonce := blob[saltLen : saltLen+nonceLen]
	ciphertext := blob[saltLen+nonceLen:]

	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrWrongPassphrase
	}
	return plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// EncryptText encrypts plaintext and returns a self-describing string suitable
// for storing as a text short's content.
func EncryptText(passphrase, plaintext string) (string, error) {
	blob, err := encryptRaw(passphrase, []byte(plaintext))
	if err != nil {
		return "", err
	}
	return textPrefix + base64.StdEncoding.EncodeToString(blob), nil
}

// IsEncryptedText reports whether a stored text short is an encrypted blob.
func IsEncryptedText(s string) bool {
	return strings.HasPrefix(s, textPrefix)
}

// DecryptText reverses EncryptText.
func DecryptText(passphrase, s string) (string, error) {
	if !IsEncryptedText(s) {
		return "", errors.New("content is not encrypted")
	}
	blob, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, textPrefix))
	if err != nil {
		return "", errors.New("malformed encrypted content")
	}
	plaintext, err := decryptRaw(passphrase, blob)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// EncryptBytes encrypts arbitrary bytes (e.g. a file), prefixing a magic header
// so the result is self-identifying.
func EncryptBytes(passphrase string, data []byte) ([]byte, error) {
	blob, err := encryptRaw(passphrase, data)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(fileMagic)+len(blob))
	out = append(out, fileMagic...)
	out = append(out, blob...)
	return out, nil
}

// IsEncryptedBytes reports whether data was produced by EncryptBytes.
func IsEncryptedBytes(data []byte) bool {
	return len(data) >= len(fileMagic) && bytes.Equal(data[:len(fileMagic)], fileMagic)
}

// DecryptBytes reverses EncryptBytes.
func DecryptBytes(passphrase string, data []byte) ([]byte, error) {
	if !IsEncryptedBytes(data) {
		return nil, errors.New("data is not an encrypted nikte file")
	}
	return decryptRaw(passphrase, data[len(fileMagic):])
}
