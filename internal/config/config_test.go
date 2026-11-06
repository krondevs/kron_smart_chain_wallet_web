package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	file := File{
		Environment:         "production",
		HTTPAddr:            ":9090",
		PublicURL:           "https://wallet.example.com",
		DataDir:             filepath.Join(dir, "data"),
		DBPath:              filepath.Join(dir, "data", "wallet.db"),
		RPCURL:              "http://127.0.0.1:20893",
		RPCUser:             "webwallet",
		RPCPassword:         "secret-rpc",
		RPCTimeout:          "45s",
		MasterKey:           "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		SessionTTL:          "8h",
		MFAReverifyAfter:    "5m",
		WalletUnlockSecs:    30,
		RegistrationEnabled: false,
		RequireTOTP:         true,
		DefaultPerTxLimit:   "12.5",
		DefaultDailyLimit:   "250",
		MaxAccountsPerIP:    3,
		MaxContractGas:      3_000_000,
		MaxGasPriceSat:      500,
		DefaultGasLimit:     300_000,
		DefaultCreateGas:    3_000_000,
		KSCDecimals:         8,
		WalletPrefix:        "w_",
		TOTPIssuer:          "Test Wallet",
	}
	write, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, write, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("http addr = %q", cfg.HTTPAddr)
	}
	if cfg.RPCPassword != "secret-rpc" {
		t.Fatalf("rpc password = %q", cfg.RPCPassword)
	}
	if cfg.RPCTimeout != 45*time.Second {
		t.Fatalf("rpc timeout = %s", cfg.RPCTimeout)
	}
	if cfg.SessionTTL != 8*time.Hour {
		t.Fatalf("session ttl = %s", cfg.SessionTTL)
	}
	if cfg.DefaultPerTxLimit != 12_50000000 {
		t.Fatalf("per tx = %d", cfg.DefaultPerTxLimit)
	}
	if cfg.DefaultDailyLimit != 250_00000000 {
		t.Fatalf("daily = %d", cfg.DefaultDailyLimit)
	}
	if cfg.WalletPrefix != "w_" {
		t.Fatalf("prefix = %q", cfg.WalletPrefix)
	}
	if cfg.CookieSecure != true {
		t.Fatal("production must force secure cookies")
	}
}

func TestLoadCreatesSettingsAndMasterKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MasterKey == "" {
		t.Fatal("master key was not generated")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if file.MasterKey == "" || file.HTTPAddr == "" {
		t.Fatalf("incomplete generated settings: %#v", file)
	}
}

func TestParseLimit(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"100", 100_00000000},
		{"100.5", 100_50000000},
		{"10000000000sat", 10_000_000_000},
		{"1ksc", 100_000000},
	}
	for _, tc := range cases {
		if got := parseLimit(tc.in, 0); got != tc.want {
			t.Errorf("parseLimit(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
