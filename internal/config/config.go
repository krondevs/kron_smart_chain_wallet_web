package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kron-smart-chain/webwallet/internal/amount"
)

// Config is the typed runtime configuration used by the application.
type Config struct {
	Environment string
	HTTPAddr    string
	PublicURL   string

	TrustedProxies []string
	DataDir        string
	DBPath         string

	RPCURL      string
	RPCUser     string
	RPCPassword string
	RPCTimeout  time.Duration

	MasterKey     string
	MasterKeyFile string

	SessionCookieName string
	CSRFCookieName    string
	CookieSecure      bool
	SessionTTL        time.Duration
	MFAReverifyAfter  time.Duration
	WalletUnlockSecs  int

	RegistrationEnabled bool
	RequireTOTP         bool
	AdminUsername       string
	AdminPassword       string
	MetricsToken        string

	DefaultPerTxLimit int64
	DefaultDailyLimit int64
	MaxAccountsPerIP  int

	MaxLoadedWallets  int
	WalletLoadOnStart bool

	MaxContractGas   uint64
	MaxGasPriceSat   int64
	DefaultGasLimit  uint64
	DefaultCreateGas uint64
	KSCDecimals      int
	WalletPrefix     string
	TOTPIssuer       string

	BackupDir string
}

// File is the on-disk settings.json schema. Durations and KSC amounts are
// strings so the file stays human editable.
type File struct {
	Environment string `json:"environment"`
	HTTPAddr    string `json:"http_addr"`
	PublicURL   string `json:"public_url"`

	TrustedProxies []string `json:"trusted_proxies"`
	DataDir        string   `json:"data_dir"`
	DBPath         string   `json:"db_path"`

	RPCURL      string `json:"rpc_url"`
	RPCUser     string `json:"rpc_user"`
	RPCPassword string `json:"rpc_password"`
	RPCTimeout  string `json:"rpc_timeout"`

	MasterKey     string `json:"master_key"`
	MasterKeyFile string `json:"master_key_file"`

	SessionCookieName string `json:"session_cookie_name"`
	CSRFCookieName    string `json:"csrf_cookie_name"`
	CookieSecure      bool   `json:"cookie_secure"`
	SessionTTL        string `json:"session_ttl"`
	MFAReverifyAfter  string `json:"mfa_reverify_after"`
	WalletUnlockSecs  int    `json:"wallet_unlock_seconds"`

	RegistrationEnabled bool   `json:"registration_enabled"`
	RequireTOTP         bool   `json:"require_totp"`
	AdminUsername       string `json:"admin_username"`
	AdminPassword       string `json:"admin_password"`
	MetricsToken        string `json:"metrics_token"`

	DefaultPerTxLimit string `json:"default_per_tx_limit"`
	DefaultDailyLimit string `json:"default_daily_limit"`
	MaxAccountsPerIP  int    `json:"max_accounts_per_ip"`

	MaxLoadedWallets  int  `json:"max_loaded_wallets"`
	WalletLoadOnStart bool `json:"wallet_load_on_startup"`

	MaxContractGas   uint64 `json:"max_contract_gas"`
	MaxGasPriceSat   int64  `json:"max_gas_price_sat"`
	DefaultGasLimit  uint64 `json:"default_gas_limit"`
	DefaultCreateGas uint64 `json:"default_create_gas"`
	KSCDecimals      int    `json:"ksc_decimals"`
	WalletPrefix     string `json:"wallet_prefix"`
	TOTPIssuer       string `json:"totp_issuer"`

	BackupDir string `json:"wallet_backup_dir"`
}

func Default() *Config {
	return &Config{
		Environment:         "development",
		HTTPAddr:            ":8080",
		PublicURL:           "http://localhost:8080",
		TrustedProxies:      []string{"127.0.0.1", "::1"},
		DataDir:             "./data",
		DBPath:              "./data/webwallet.db",
		RPCURL:              "http://127.0.0.1:20889",
		RPCTimeout:          30 * time.Second,
		SessionCookieName:   "ksc_session",
		CSRFCookieName:      "ksc_csrf",
		SessionTTL:          12 * time.Hour,
		MFAReverifyAfter:    10 * time.Minute,
		WalletUnlockSecs:    60,
		RegistrationEnabled: true,
		RequireTOTP:         false,
		DefaultPerTxLimit:   100_00000000,  // 100 KSC
		DefaultDailyLimit:   1000_00000000, // 1000 KSC
		MaxAccountsPerIP:    5,
		MaxLoadedWallets:    100,
		MaxContractGas:      2_500_000,
		MaxGasPriceSat:      10_000,
		DefaultGasLimit:     250_000,
		DefaultCreateGas:    2_500_000,
		KSCDecimals:         8,
		WalletPrefix:        "u_",
		TOTPIssuer:          "Kron Smart Chain Wallet",
	}
}

func FileFromConfig(c *Config) File {
	return File{
		Environment:         c.Environment,
		HTTPAddr:            c.HTTPAddr,
		PublicURL:           c.PublicURL,
		TrustedProxies:      append([]string{}, c.TrustedProxies...),
		DataDir:             c.DataDir,
		DBPath:              c.DBPath,
		RPCURL:              c.RPCURL,
		RPCUser:             c.RPCUser,
		RPCPassword:         c.RPCPassword,
		RPCTimeout:          c.RPCTimeout.String(),
		MasterKey:           c.MasterKey,
		MasterKeyFile:       c.MasterKeyFile,
		SessionCookieName:   c.SessionCookieName,
		CSRFCookieName:      c.CSRFCookieName,
		CookieSecure:        c.CookieSecure,
		SessionTTL:          c.SessionTTL.String(),
		MFAReverifyAfter:    c.MFAReverifyAfter.String(),
		WalletUnlockSecs:    c.WalletUnlockSecs,
		RegistrationEnabled: c.RegistrationEnabled,
		RequireTOTP:         c.RequireTOTP,
		AdminUsername:       c.AdminUsername,
		AdminPassword:       c.AdminPassword,
		MetricsToken:        c.MetricsToken,
		DefaultPerTxLimit:   amount.FormatKSC(c.DefaultPerTxLimit),
		DefaultDailyLimit:   amount.FormatKSC(c.DefaultDailyLimit),
		MaxAccountsPerIP:    c.MaxAccountsPerIP,
		MaxLoadedWallets:    c.MaxLoadedWallets,
		WalletLoadOnStart:   c.WalletLoadOnStart,
		MaxContractGas:      c.MaxContractGas,
		MaxGasPriceSat:      c.MaxGasPriceSat,
		DefaultGasLimit:     c.DefaultGasLimit,
		DefaultCreateGas:    c.DefaultCreateGas,
		KSCDecimals:         c.KSCDecimals,
		WalletPrefix:        c.WalletPrefix,
		TOTPIssuer:          c.TOTPIssuer,
		BackupDir:           c.BackupDir,
	}
}

// Load reads settings.json. If the file does not exist it is created with
// defaults and a fresh master key. Missing fields in an existing file keep
// their defaults.
func Load(path string) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		path = "settings.json"
	}
	cfg := Default()
	f := FileFromConfig(cfg)

	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		key, keyErr := newMasterKey()
		if keyErr != nil {
			return nil, keyErr
		}
		cfg.MasterKey = key
		f = FileFromConfig(cfg)
		if err := writeFile(path, f); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, fmt.Errorf("read %s: %w", path, err)
	default:
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		applyFile(cfg, &f)
	}

	if strings.TrimSpace(cfg.MasterKey) == "" && strings.TrimSpace(cfg.MasterKeyFile) == "" {
		key, keyErr := newMasterKey()
		if keyErr != nil {
			return nil, keyErr
		}
		cfg.MasterKey = key
		f = FileFromConfig(cfg)
		if err := writeFile(path, f); err != nil {
			return nil, err
		}
	}

	normalize(cfg)
	// settings.json contains the master key and RPC credentials.
	_ = os.Chmod(path, 0o600)
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return cfg, nil
}

func applyFile(c *Config, f *File) {
	if f.Environment != "" {
		c.Environment = f.Environment
	}
	if f.HTTPAddr != "" {
		c.HTTPAddr = f.HTTPAddr
	}
	if f.PublicURL != "" {
		c.PublicURL = f.PublicURL
	}
	c.TrustedProxies = f.TrustedProxies
	if f.DataDir != "" {
		c.DataDir = f.DataDir
	}
	if f.DBPath != "" {
		c.DBPath = f.DBPath
	}
	if f.RPCURL != "" {
		c.RPCURL = f.RPCURL
	}
	c.RPCUser = f.RPCUser
	c.RPCPassword = f.RPCPassword
	if d, err := time.ParseDuration(strings.TrimSpace(f.RPCTimeout)); err == nil {
		c.RPCTimeout = d
	}
	c.MasterKey = strings.TrimSpace(f.MasterKey)
	c.MasterKeyFile = strings.TrimSpace(f.MasterKeyFile)
	if f.SessionCookieName != "" {
		c.SessionCookieName = f.SessionCookieName
	}
	if f.CSRFCookieName != "" {
		c.CSRFCookieName = f.CSRFCookieName
	}
	c.CookieSecure = f.CookieSecure
	if d, err := time.ParseDuration(strings.TrimSpace(f.SessionTTL)); err == nil {
		c.SessionTTL = d
	}
	if d, err := time.ParseDuration(strings.TrimSpace(f.MFAReverifyAfter)); err == nil {
		c.MFAReverifyAfter = d
	}
	if f.WalletUnlockSecs > 0 {
		c.WalletUnlockSecs = f.WalletUnlockSecs
	}
	c.RegistrationEnabled = f.RegistrationEnabled
	c.RequireTOTP = f.RequireTOTP
	c.AdminUsername = f.AdminUsername
	c.AdminPassword = f.AdminPassword
	c.MetricsToken = f.MetricsToken
	c.DefaultPerTxLimit = parseLimit(f.DefaultPerTxLimit, c.DefaultPerTxLimit)
	c.DefaultDailyLimit = parseLimit(f.DefaultDailyLimit, c.DefaultDailyLimit)
	if f.MaxAccountsPerIP > 0 {
		c.MaxAccountsPerIP = f.MaxAccountsPerIP
	}
	if f.MaxLoadedWallets > 0 {
		c.MaxLoadedWallets = f.MaxLoadedWallets
	}
	c.WalletLoadOnStart = f.WalletLoadOnStart
	if f.MaxContractGas > 0 {
		c.MaxContractGas = f.MaxContractGas
	}
	if f.MaxGasPriceSat > 0 {
		c.MaxGasPriceSat = f.MaxGasPriceSat
	}
	if f.DefaultGasLimit > 0 {
		c.DefaultGasLimit = f.DefaultGasLimit
	}
	if f.DefaultCreateGas > 0 {
		c.DefaultCreateGas = f.DefaultCreateGas
	}
	if f.KSCDecimals > 0 {
		c.KSCDecimals = f.KSCDecimals
	}
	if f.WalletPrefix != "" {
		c.WalletPrefix = f.WalletPrefix
	}
	if f.TOTPIssuer != "" {
		c.TOTPIssuer = f.TOTPIssuer
	}
	c.BackupDir = f.BackupDir
}

func normalize(c *Config) {
	if c.DBPath == "" {
		c.DBPath = filepath.Join(c.DataDir, "webwallet.db")
	}
	if c.Environment == "production" {
		c.CookieSecure = true
	}
	if c.RPCTimeout < 5*time.Second {
		c.RPCTimeout = 5 * time.Second
	}
	if c.SessionTTL < 5*time.Minute {
		c.SessionTTL = 5 * time.Minute
	}
	if c.MFAReverifyAfter < time.Minute {
		c.MFAReverifyAfter = time.Minute
	}
	if c.WalletUnlockSecs < 5 {
		c.WalletUnlockSecs = 5
	}
	if c.MaxLoadedWallets < 1 {
		c.MaxLoadedWallets = 100
	}
	if c.MaxContractGas < c.DefaultGasLimit {
		c.MaxContractGas = c.DefaultGasLimit
	}
	if c.DefaultGasLimit < 21_000 {
		c.DefaultGasLimit = 250_000
	}
	if c.DefaultCreateGas < c.DefaultGasLimit {
		c.DefaultCreateGas = 2_500_000
	}
	if c.KSCDecimals != 8 {
		// The amount package and KSC network use 8 decimals. Refuse silent
		// misconfiguration.
		c.KSCDecimals = 8
	}
	if c.DefaultPerTxLimit < 0 {
		c.DefaultPerTxLimit = 0
	}
	if c.DefaultDailyLimit < 0 {
		c.DefaultDailyLimit = 0
	}
	if c.MaxGasPriceSat <= 0 {
		c.MaxGasPriceSat = 10_000
	}
	if c.WalletPrefix == "" {
		c.WalletPrefix = "u_"
	}
	if c.TOTPIssuer == "" {
		c.TOTPIssuer = "Kron Smart Chain Wallet"
	}
}

func parseLimit(value string, fallback int64) int64 {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return fallback
	}
	if strings.HasSuffix(v, "sat") {
		n, err := strconv.ParseInt(strings.TrimSpace(strings.TrimSuffix(v, "sat")), 10, 64)
		if err != nil {
			return fallback
		}
		return n
	}
	v = strings.TrimSuffix(v, "ksc")
	n, err := amount.ParseKSC(strings.TrimSpace(v))
	if err != nil {
		return fallback
	}
	return n
}

func newMasterKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

func writeFile(path string, f File) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	// os.WriteFile preserves an existing file's mode; always tighten it because
	// this file contains secrets.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}
