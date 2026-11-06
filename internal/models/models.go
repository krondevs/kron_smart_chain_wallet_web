package models

import "time"

// User is a custodial wallet account. PasswordHash is an Argon2id encoded
// string and TOTPSecret is encrypted with the application master key.
type User struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Username       string     `gorm:"uniqueIndex;size:32;not null" json:"username"`
	PasswordHash   string     `gorm:"not null" json:"-"`
	Role           string     `gorm:"size:16;not null;default:user" json:"role"`
	Status         string     `gorm:"size:16;not null;default:active" json:"status"`
	Tier           int        `gorm:"not null;default:0" json:"tier"`
	PerTxLimit     int64      `gorm:"not null;default:0" json:"per_tx_limit"`
	DailyLimit     int64      `gorm:"not null;default:0" json:"daily_limit"`
	TOTPSecret     string     `gorm:"size:2048" json:"-"`
	TOTPEnabled    bool       `gorm:"not null;default:false" json:"totp_enabled"`
	RegistrationIP string     `gorm:"size:64" json:"-"`
	LastLoginAt    *time.Time `json:"last_login_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Session struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	UserID        uint       `gorm:"index;not null" json:"user_id"`
	TokenHash     string     `gorm:"uniqueIndex;size:64;not null" json:"-"`
	MFAVerified   bool       `gorm:"not null;default:false" json:"mfa_verified"`
	MFAVerifiedAt *time.Time `json:"mfa_verified_at,omitempty"`
	IP            string     `gorm:"size:64" json:"ip"`
	UserAgent     string     `gorm:"size:512" json:"user_agent"`
	ExpiresAt     time.Time  `gorm:"index;not null" json:"expires_at"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Wallet struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"uniqueIndex;not null" json:"user_id"`
	Name      string    `gorm:"uniqueIndex;size:80;not null" json:"name"`
	Address   string    `gorm:"size:80;not null" json:"address"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type WalletSecret struct {
	ID         uint   `gorm:"primaryKey"`
	WalletID   uint   `gorm:"uniqueIndex;not null"`
	Ciphertext string `gorm:"type:text;not null"`
	Nonce      string `gorm:"size:64;not null"`
	KeyVersion int    `gorm:"not null;default:1"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Token struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Contract    string    `gorm:"uniqueIndex;size:64;not null" json:"contract"`
	Name        string    `gorm:"size:128" json:"name"`
	Symbol      string    `gorm:"size:32" json:"symbol"`
	Decimals    int       `gorm:"not null;default:0" json:"decimals"`
	TotalSupply string    `gorm:"size:128" json:"total_supply"`
	Verified    bool      `gorm:"not null;default:false" json:"verified"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UserToken struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"uniqueIndex:idx_user_token;not null" json:"user_id"`
	TokenID   uint      `gorm:"uniqueIndex:idx_user_token;not null" json:"token_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Contract struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"uniqueIndex:idx_user_contract;not null" json:"user_id"`
	Address   string    `gorm:"uniqueIndex:idx_user_contract;size:64;not null" json:"address"`
	Name      string    `gorm:"size:128" json:"name"`
	ABI       string    `gorm:"type:text" json:"abi"`
	Bytecode  string    `gorm:"type:text" json:"bytecode,omitempty"`
	Deployed  bool      `gorm:"not null;default:false" json:"deployed"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AddressBookEntry struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Label     string    `gorm:"size:128;not null" json:"label"`
	Address   string    `gorm:"size:80;not null" json:"address"`
	CreatedAt time.Time `json:"created_at"`
}

type TxIntent struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"uniqueIndex:idx_user_idem;not null" json:"user_id"`
	APIKeyID       *uint     `gorm:"index" json:"api_key_id,omitempty"`
	IdempotencyKey string    `gorm:"uniqueIndex:idx_user_idem;size:80;not null" json:"-"`
	Kind           string    `gorm:"size:32;not null" json:"kind"`
	Status         string    `gorm:"size:24;not null" json:"status"`
	TxID           string    `gorm:"size:80" json:"txid"`
	Error          string    `gorm:"type:text" json:"error,omitempty"`
	Payload        string    `gorm:"type:text" json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type AuditEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    *uint     `gorm:"index" json:"user_id,omitempty"`
	APIKeyID  *uint     `gorm:"index" json:"api_key_id,omitempty"`
	Action    string    `gorm:"size:64;not null" json:"action"`
	Detail    string    `gorm:"type:text" json:"detail,omitempty"`
	IP        string    `gorm:"size:64" json:"ip,omitempty"`
	RequestID string    `gorm:"size:64" json:"request_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type RecoveryCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"index;not null" json:"user_id"`
	CodeHash  string     `gorm:"uniqueIndex;size:64;not null" json:"-"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type DailyUsage struct {
	ID     uint   `gorm:"primaryKey"`
	UserID uint   `gorm:"uniqueIndex:idx_user_day;not null"`
	Day    string `gorm:"uniqueIndex:idx_user_day;size:10;not null"`
	Amount int64  `gorm:"not null;default:0"`
}

type APIKey struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	UserID     uint       `gorm:"index;not null" json:"user_id"`
	Name       string     `gorm:"size:80;not null" json:"name"`
	PublicID   string     `gorm:"uniqueIndex;size:32;not null" json:"public_id"`
	SecretHash string     `gorm:"uniqueIndex;size:64;not null" json:"-"`
	ExpiresAt  time.Time  `gorm:"index;not null" json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}
