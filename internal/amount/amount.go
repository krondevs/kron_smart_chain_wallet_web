package amount

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

const KSCDecimals = 8

var pow10 = [...]int64{
	1, 10, 100, 1000, 10000, 100000, 1000000, 10000000, 100000000,
}

func ParseKSC(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("amount is required")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	if s == "" {
		return 0, errors.New("invalid amount")
	}
	parts := strings.SplitN(s, ".", 2)
	wholeStr := parts[0]
	if wholeStr == "" {
		wholeStr = "0"
	}
	whole, ok := new(big.Int).SetString(wholeStr, 10)
	if !ok || whole.Sign() < 0 {
		return 0, errors.New("invalid amount")
	}
	frac := int64(0)
	if len(parts) == 2 {
		f := parts[1]
		if len(f) > KSCDecimals {
			return 0, fmt.Errorf("amount supports at most %d decimals", KSCDecimals)
		}
		for len(f) < KSCDecimals {
			f += "0"
		}
		if f != "" {
			n, ok := new(big.Int).SetString(f, 10)
			if !ok {
				return 0, errors.New("invalid amount")
			}
			frac = n.Int64()
		}
	}
	whole.Mul(whole, big.NewInt(pow10[KSCDecimals]))
	whole.Add(whole, big.NewInt(frac))
	if !whole.IsInt64() {
		return 0, errors.New("amount is too large")
	}
	v := whole.Int64()
	if neg {
		v = -v
	}
	return v, nil
}

func FormatKSC(sat int64) string {
	neg := sat < 0
	if neg {
		sat = -sat
	}
	whole := sat / pow10[KSCDecimals]
	frac := sat % pow10[KSCDecimals]
	out := fmt.Sprintf("%d", whole)
	if frac > 0 {
		f := fmt.Sprintf("%08d", frac)
		f = strings.TrimRight(f, "0")
		out += "." + f
	}
	if neg {
		out = "-" + out
	}
	return out
}

// ParseTokenAmount converts a human token amount to the raw integer string
// expected by the QRC20/KRC20 RPCs.
func ParseTokenAmount(s string, decimals int) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("amount is required")
	}
	if decimals < 0 || decimals > 76 {
		return "", errors.New("unsupported token decimals")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	parts := strings.SplitN(s, ".", 2)
	wholeStr := parts[0]
	if wholeStr == "" {
		wholeStr = "0"
	}
	whole, ok := new(big.Int).SetString(wholeStr, 10)
	if !ok || whole.Sign() < 0 {
		return "", errors.New("invalid amount")
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole.Mul(whole, scale)
	if len(parts) == 2 {
		f := parts[1]
		if len(f) > decimals {
			return "", fmt.Errorf("amount supports at most %d decimals", decimals)
		}
		for len(f) < decimals {
			f += "0"
		}
		if f != "" {
			frac, ok := new(big.Int).SetString(f, 10)
			if !ok {
				return "", errors.New("invalid amount")
			}
			whole.Add(whole, frac)
		}
	}
	if neg {
		whole.Neg(whole)
	}
	return whole.String(), nil
}

// FormatTokenAmount converts a raw token integer string to a human decimal.
func FormatTokenAmount(raw string, decimals int) string {
	n, ok := new(big.Int).SetString(strings.TrimSpace(raw), 10)
	if !ok {
		return raw
	}
	neg := n.Sign() < 0
	if neg {
		n.Abs(n)
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole := new(big.Int).Quo(n, scale)
	frac := new(big.Int).Rem(n, scale)
	out := whole.String()
	if frac.Sign() > 0 {
		f := frac.String()
		for len(f) < decimals {
			f = "0" + f
		}
		f = strings.TrimRight(f, "0")
		out += "." + f
	}
	if neg {
		out = "-" + out
	}
	return out
}

func AddSat(a, b int64) int64 { return a + b }

func CmpSat(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
