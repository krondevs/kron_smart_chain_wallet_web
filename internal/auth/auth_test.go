package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse 1234")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "correct horse 1234") {
		t.Fatal("valid password rejected")
	}
	if VerifyPassword(hash, "wrong password 1234") {
		t.Fatal("invalid password accepted")
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("short1"); err == nil {
		t.Fatal("short password accepted")
	}
	if err := ValidatePassword("alllettersonly"); err == nil {
		t.Fatal("password without digit accepted")
	}
	if err := ValidatePassword("abcdefgh1234"); err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
}

func TestRecoveryCodes(t *testing.T) {
	plain, hashes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) != RecoveryCodeCount || len(hashes) != RecoveryCodeCount {
		t.Fatalf("unexpected code count")
	}
	normalized := NormalizeRecoveryCode(plain[0])
	if len(normalized) != 16 {
		t.Fatalf("normalized length = %d", len(normalized))
	}
	if strings.Contains(normalized, "-") {
		t.Fatal("normalization did not remove dashes")
	}
}

func TestTOTP(t *testing.T) {
	secret, url, err := GenerateTOTP("Test", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "otpauth://totp/") {
		t.Fatalf("unexpected URL: %s", url)
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateTOTP(secret, code) {
		t.Fatal("valid TOTP rejected")
	}
	if ValidateTOTP(secret, "000000") && code != "000000" {
		t.Fatal("invalid TOTP accepted")
	}
}
