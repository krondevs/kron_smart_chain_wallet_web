package auth

import (
	"crypto/rand"
	"encoding/base32"
	"strings"

	"github.com/kron-smart-chain/webwallet/internal/secret"
)

const RecoveryCodeCount = 10

// GenerateRecoveryCodes returns human-readable one-time codes. Each code has
// 80 bits of entropy, so a SHA-256 hash is sufficient for lookup.
func GenerateRecoveryCodes() (plain []string, hashes []string, err error) {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	plain = make([]string, 0, RecoveryCodeCount)
	hashes = make([]string, 0, RecoveryCodeCount)
	for i := 0; i < RecoveryCodeCount; i++ {
		b := make([]byte, 10)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, err
		}
		raw := strings.ToUpper(enc.EncodeToString(b))
		formatted := raw[0:4] + "-" + raw[4:8] + "-" + raw[8:12] + "-" + raw[12:16]
		plain = append(plain, formatted)
		hashes = append(hashes, secret.HashRecoveryCode(NormalizeRecoveryCode(formatted)))
	}
	return plain, hashes, nil
}

func NormalizeRecoveryCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ReplaceAll(code, " ", "")
	return code
}
