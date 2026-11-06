package secret

import (
	"strings"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	m, err := LoadOrCreate("", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ct, nonce, err := m.Encrypt([]byte("super secret"), "wallet-1")
	if err != nil {
		t.Fatal(err)
	}
	pt, err := m.Decrypt(ct, nonce, "wallet-1")
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != "super secret" {
		t.Fatalf("got %q", pt)
	}
	if _, err := m.Decrypt(ct, nonce, "wallet-2"); err == nil {
		t.Fatal("decryption with wrong AAD succeeded")
	}
	if !strings.HasPrefix(ct, "") {
		t.Fatal("sanity")
	}
}

func TestHashToken(t *testing.T) {
	a := HashToken("abc")
	b := HashToken("abc")
	if a != b || len(a) != 64 {
		t.Fatalf("unexpected hash: %s", a)
	}
}
