package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kron-smart-chain/webwallet/internal/config"
	"github.com/kron-smart-chain/webwallet/internal/models"
	"github.com/kron-smart-chain/webwallet/internal/secret"
	"github.com/kron-smart-chain/webwallet/internal/wallet"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrAccountDisabled    = errors.New("account is disabled")
	ErrDailyLimit         = errors.New("daily limit exceeded")
	ErrPerTxLimit         = errors.New("transaction limit exceeded")
	ErrTOTPRequired       = errors.New("TOTP is required")
	ErrInvalidTOTP        = errors.New("invalid TOTP code")
	ErrInvalidRecovery    = errors.New("invalid or already used recovery code")
	ErrTooManyAccounts    = errors.New("too many accounts created from this address")
	ErrUsernameTaken      = errors.New("username is already taken")
	ErrInvalidUsername    = errors.New("invalid username")
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_.-]{3,32}$`)

type Service struct {
	db      *gorm.DB
	cfg     *config.Config
	secrets *secret.Manager
	wallets *wallet.Service
}

func NewService(db *gorm.DB, cfg *config.Config, secrets *secret.Manager, wallets *wallet.Service) *Service {
	return &Service{db: db, cfg: cfg, secrets: secrets, wallets: wallets}
}

type RegisterResult struct {
	User   *models.User
	Wallet *models.Wallet
}

func (s *Service) Register(ctx context.Context, username, password, ip string) (*RegisterResult, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !usernamePattern.MatchString(username) {
		return nil, fmt.Errorf("%w: use 3-32 letters, numbers, dot, dash or underscore", ErrInvalidUsername)
	}
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}

	var existing int64
	if err := s.db.Model(&models.User{}).Where("username = ?", username).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrUsernameTaken
	}
	if ip != "" && s.cfg.MaxAccountsPerIP > 0 {
		var recent int64
		cutoff := time.Now().UTC().Add(-24 * time.Hour)
		if err := s.db.Model(&models.User{}).
			Where("registration_ip = ? AND created_at > ?", ip, cutoff).
			Count(&recent).Error; err != nil {
			return nil, err
		}
		if recent >= int64(s.cfg.MaxAccountsPerIP) {
			return nil, ErrTooManyAccounts
		}
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	user := &models.User{
		Username:       username,
		PasswordHash:   hash,
		Role:           "user",
		Status:         "active",
		Tier:           0,
		PerTxLimit:     s.cfg.DefaultPerTxLimit,
		DailyLimit:     s.cfg.DefaultDailyLimit,
		TOTPEnabled:    false,
		RegistrationIP: ip,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(user).Error
	})
	if err != nil {
		return nil, err
	}

	w, err := s.wallets.Provision(ctx, user)
	if err != nil {
		// Roll back the account so the username can be retried. The node wallet
		// may already exist; it is harmless and can be cleaned by an operator.
		_ = s.db.WithContext(ctx).Delete(user).Error
		return nil, fmt.Errorf("provision wallet: %w", err)
	}

	return &RegisterResult{
		User:   user,
		Wallet: w,
	}, nil
}

func (s *Service) Authenticate(ctx context.Context, username, password string) (*models.User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	var user models.User
	err := s.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Perform a dummy hash verification to reduce user enumeration timing.
			_ = VerifyPassword("$argon2id$v=19$m=65536,t=3,p=2$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", password)
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if user.Status != "active" {
		return nil, ErrAccountDisabled
	}
	if !VerifyPassword(user.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	now := time.Now().UTC()
	user.LastLoginAt = &now
	_ = s.db.WithContext(ctx).Model(&user).Update("last_login_at", now).Error
	return &user, nil
}

func (s *Service) CreateSession(ctx context.Context, user *models.User, ip, ua string) (string, *models.Session, error) {
	raw, err := secret.RandomToken(32)
	if err != nil {
		return "", nil, err
	}
	sess := &models.Session{
		UserID:    user.ID,
		TokenHash: secret.HashToken(raw),
		IP:        ip,
		UserAgent: truncate(ua, 500),
		ExpiresAt: time.Now().UTC().Add(s.cfg.SessionTTL),
	}
	if err := s.db.WithContext(ctx).Create(sess).Error; err != nil {
		return "", nil, err
	}
	return raw, sess, nil
}

func (s *Service) GetSession(ctx context.Context, rawToken string) (*models.Session, *models.User, error) {
	if rawToken == "" {
		return nil, nil, ErrInvalidCredentials
	}
	var sess models.Session
	err := s.db.WithContext(ctx).Where("token_hash = ?", secret.HashToken(rawToken)).First(&sess).Error
	if err != nil {
		return nil, nil, ErrInvalidCredentials
	}
	if sess.RevokedAt != nil || time.Now().UTC().After(sess.ExpiresAt) {
		return nil, nil, ErrInvalidCredentials
	}
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, sess.UserID).Error; err != nil {
		return nil, nil, ErrInvalidCredentials
	}
	if user.Status != "active" {
		return nil, nil, ErrAccountDisabled
	}
	return &sess, &user, nil
}

func (s *Service) MarkMFAVerified(ctx context.Context, sess *models.Session) error {
	now := time.Now().UTC()
	sess.MFAVerified = true
	sess.MFAVerifiedAt = &now
	return s.db.WithContext(ctx).Model(sess).Updates(map[string]any{
		"mfa_verified":    true,
		"mfa_verified_at": now,
	}).Error
}

func (s *Service) VerifyTOTP(user *models.User, code string) error {
	if !user.TOTPEnabled {
		return ErrTOTPRequired
	}
	secretValue, err := s.decryptTOTP(user.Username, user.TOTPSecret)
	if err != nil {
		return err
	}
	if !ValidateTOTP(secretValue, code) {
		return ErrInvalidTOTP
	}
	return nil
}

func (s *Service) EnableTOTP(ctx context.Context, user *models.User, code string) error {
	secretValue, err := s.decryptTOTP(user.Username, user.TOTPSecret)
	if err != nil {
		return err
	}
	if !ValidateTOTP(secretValue, code) {
		return ErrInvalidTOTP
	}
	user.TOTPEnabled = true
	return s.db.WithContext(ctx).Model(user).Update("totp_enabled", true).Error
}

func (s *Service) SetupTOTP(ctx context.Context, user *models.User) (secretValue, otpauthURL string, recovery []string, err error) {
	secretValue, otpauthURL, err = GenerateTOTP(s.cfg.TOTPIssuer, user.Username)
	if err != nil {
		return "", "", nil, err
	}
	enc, err := s.encryptTOTP(user.Username, secretValue)
	if err != nil {
		return "", "", nil, err
	}
	plain, hashes, err := GenerateRecoveryCodes()
	if err != nil {
		return "", "", nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(user).Updates(map[string]any{
			"totp_secret":  enc,
			"totp_enabled": false,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", user.ID).Delete(&models.RecoveryCode{}).Error; err != nil {
			return err
		}
		for _, h := range hashes {
			if err := tx.Create(&models.RecoveryCode{UserID: user.ID, CodeHash: h}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", "", nil, err
	}
	user.TOTPSecret = enc
	user.TOTPEnabled = false
	return secretValue, otpauthURL, plain, nil
}

func (s *Service) DisableTOTP(ctx context.Context, user *models.User, password, code string) error {
	if !VerifyPassword(user.PasswordHash, password) {
		return ErrInvalidCredentials
	}
	if !user.TOTPEnabled {
		return ErrTOTPRequired
	}
	secretValue, err := s.decryptTOTP(user.Username, user.TOTPSecret)
	if err != nil {
		return err
	}
	if !ValidateTOTP(secretValue, code) {
		return ErrInvalidTOTP
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(user).Updates(map[string]any{
			"totp_secret":  "",
			"totp_enabled": false,
		}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", user.ID).Delete(&models.RecoveryCode{}).Error
	})
	if err != nil {
		return err
	}
	user.TOTPSecret = ""
	user.TOTPEnabled = false
	return nil
}

func (s *Service) RegenerateRecoveryCodes(ctx context.Context, user *models.User) ([]string, error) {
	if !user.TOTPEnabled {
		return nil, ErrTOTPRequired
	}
	plain, hashes, err := GenerateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", user.ID).Delete(&models.RecoveryCode{}).Error; err != nil {
			return err
		}
		for _, h := range hashes {
			if err := tx.Create(&models.RecoveryCode{UserID: user.ID, CodeHash: h}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return plain, nil
}

func (s *Service) UseRecoveryCode(ctx context.Context, user *models.User, code string) error {
	normalized := NormalizeRecoveryCode(code)
	if normalized == "" {
		return ErrInvalidRecovery
	}
	hash := secret.HashRecoveryCode(normalized)
	var rc models.RecoveryCode
	err := s.db.WithContext(ctx).Where("user_id = ? AND code_hash = ? AND used_at IS NULL", user.ID, hash).First(&rc).Error
	if err != nil {
		return ErrInvalidRecovery
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&rc).Update("used_at", now).Error
}

func (s *Service) ChangePassword(ctx context.Context, user *models.User, current, next string) error {
	if !VerifyPassword(user.PasswordHash, current) {
		return ErrInvalidCredentials
	}
	if err := ValidatePassword(next); err != nil {
		return err
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(user).Update("password_hash", hash).Error; err != nil {
			return err
		}
		return tx.Model(&models.Session{}).Where("user_id = ?", user.ID).Update("revoked_at", time.Now().UTC()).Error
	})
}

func (s *Service) ResetPassword(ctx context.Context, user *models.User, next string) error {
	if err := ValidatePassword(next); err != nil {
		return err
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(user).Update("password_hash", hash).Error; err != nil {
			return err
		}
		return tx.Model(&models.Session{}).Where("user_id = ?", user.ID).Update("revoked_at", time.Now().UTC()).Error
	})
}

func (s *Service) ResetTOTP(ctx context.Context, user *models.User) (secretValue, otpauthURL string, recovery []string, err error) {
	secretValue, otpauthURL, err = GenerateTOTP(s.cfg.TOTPIssuer, user.Username)
	if err != nil {
		return "", "", nil, err
	}
	enc, err := s.encryptTOTP(user.Username, secretValue)
	if err != nil {
		return "", "", nil, err
	}
	plain, hashes, err := GenerateRecoveryCodes()
	if err != nil {
		return "", "", nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(user).Updates(map[string]any{
			"totp_secret":  enc,
			"totp_enabled": false,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", user.ID).Delete(&models.RecoveryCode{}).Error; err != nil {
			return err
		}
		for _, h := range hashes {
			if err := tx.Create(&models.RecoveryCode{UserID: user.ID, CodeHash: h}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", "", nil, err
	}
	return secretValue, otpauthURL, plain, nil
}

func (s *Service) RevokeSession(ctx context.Context, userID, sessionID uint) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&models.Session{}).
		Where("id = ? AND user_id = ?", sessionID, userID).
		Update("revoked_at", now).Error
}

func (s *Service) RevokeAllSessions(ctx context.Context, userID uint) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&models.Session{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now).Error
}

func (s *Service) ListSessions(ctx context.Context, userID uint) ([]models.Session, error) {
	var sessions []models.Session
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, time.Now().UTC()).
		Order("created_at DESC").Find(&sessions).Error
	return sessions, err
}

func (s *Service) ReserveSpend(ctx context.Context, user *models.User, sat int64) error {
	if sat <= 0 {
		return nil
	}
	if user.PerTxLimit > 0 && sat > user.PerTxLimit {
		return ErrPerTxLimit
	}
	if user.DailyLimit <= 0 {
		return nil
	}
	day := time.Now().UTC().Format("2006-01-02")
	res := s.db.WithContext(ctx).Exec(`
		INSERT INTO daily_usages (user_id, day, amount)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, day) DO UPDATE SET amount = amount + excluded.amount
		WHERE daily_usages.amount + excluded.amount <= ?`,
		user.ID, day, sat, user.DailyLimit,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrDailyLimit
	}
	return nil
}

func (s *Service) RefundSpend(ctx context.Context, userID uint, sat int64) {
	if sat <= 0 {
		return
	}
	day := time.Now().UTC().Format("2006-01-02")
	_ = s.db.WithContext(ctx).Exec(`
		UPDATE daily_usages SET amount = MAX(amount - ?, 0)
		WHERE user_id = ? AND day = ?`, sat, userID, day).Error
}

func (s *Service) Audit(ctx context.Context, userID *uint, action, detail, ip, requestID string) {
	s.AuditWithKey(ctx, userID, nil, action, detail, ip, requestID)
}

func (s *Service) AuditWithKey(ctx context.Context, userID *uint, apiKeyID *uint, action, detail, ip, requestID string) {
	ev := &models.AuditEvent{
		UserID:    userID,
		APIKeyID:  apiKeyID,
		Action:    action,
		Detail:    truncate(detail, 2000),
		IP:        truncate(ip, 64),
		RequestID: truncate(requestID, 64),
	}
	if err := s.db.WithContext(ctx).Create(ev).Error; err != nil {
		slog.Warn("audit write failed", "error", err, "action", action)
	}
}

func (s *Service) BootstrapAdmin(ctx context.Context) error {
	username := strings.ToLower(strings.TrimSpace(s.cfg.AdminUsername))
	password := s.cfg.AdminPassword
	if username == "" || password == "" {
		return nil
	}
	var user models.User
	err := s.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err := ValidatePassword(password); err != nil {
		return fmt.Errorf("ADMIN_PASSWORD: %w", err)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	totpSecret, _, err := GenerateTOTP(s.cfg.TOTPIssuer, username)
	if err != nil {
		return err
	}
	enc, err := s.encryptTOTP(username, totpSecret)
	if err != nil {
		return err
	}
	user = models.User{
		Username:     username,
		PasswordHash: hash,
		Role:         "admin",
		Status:       "active",
		Tier:         99,
		PerTxLimit:   0,
		DailyLimit:   0,
		TOTPSecret:   enc,
		TOTPEnabled:  false,
	}
	if err := s.db.WithContext(ctx).Create(&user).Error; err != nil {
		return err
	}
	if _, err := s.wallets.Provision(ctx, &user); err != nil {
		// The node may still be starting or syncing. The admin can provision the
		// wallet later from the admin panel.
		slog.Warn("bootstrap admin wallet not provisioned", "username", username, "error", err)
	}
	slog.Info("bootstrap admin created", "username", username)
	return nil
}

func (s *Service) encryptTOTP(username, plain string) (string, error) {
	ct, nonce, err := s.secrets.Encrypt([]byte(plain), "totp:"+username)
	if err != nil {
		return "", err
	}
	return nonce + ":" + ct, nil
}

func (s *Service) decryptTOTP(username, encoded string) (string, error) {
	parts := strings.SplitN(encoded, ":", 2)
	if len(parts) != 2 {
		return "", errors.New("invalid TOTP secret encoding")
	}
	pt, err := s.secrets.Decrypt(parts[1], parts[0], "totp:"+username)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
