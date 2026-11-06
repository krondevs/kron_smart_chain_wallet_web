package auth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kron-smart-chain/webwallet/internal/models"
	"github.com/kron-smart-chain/webwallet/internal/secret"
)

var (
	ErrAPIKeyLimit   = errors.New("maximum of 3 active API keys")
	ErrAPIKeyInvalid = errors.New("invalid API key")
	ErrAPIKeyExpired = errors.New("API key expired")
)

const (
	MaxActiveAPIKeys = 3
	APIKeyRateLimit  = 30
)

func (s *Service) CreateAPIKey(ctx context.Context, user *models.User, name string, expiresAt time.Time) (*models.APIKey, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return nil, "", errors.New("API key name is required and must be at most 80 characters")
	}
	now := time.Now().UTC()
	if expiresAt.Before(now.Add(time.Hour)) {
		return nil, "", errors.New("API key expiration must be at least 1 hour in the future")
	}
	if expiresAt.After(now.Add(365 * 24 * time.Hour)) {
		return nil, "", errors.New("API key expiration cannot be more than 365 days in the future")
	}

	var active int64
	if err := s.db.WithContext(ctx).Model(&models.APIKey{}).
		Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", user.ID, now).
		Count(&active).Error; err != nil {
		return nil, "", err
	}
	if active >= MaxActiveAPIKeys {
		return nil, "", ErrAPIKeyLimit
	}

	publicBytes, err := secret.RandomBytes(8)
	if err != nil {
		return nil, "", err
	}
	publicID := hex.EncodeToString(publicBytes)
	secretBytes, err := secret.RandomBytes(32)
	if err != nil {
		return nil, "", err
	}
	secretPart := base64.RawURLEncoding.EncodeToString(secretBytes)
	plaintext := "ksc_" + publicID + "_" + secretPart

	key := &models.APIKey{
		UserID:     user.ID,
		Name:       name,
		PublicID:   publicID,
		SecretHash: secret.HashToken(plaintext),
		ExpiresAt:  expiresAt.UTC(),
	}
	if err := s.db.WithContext(ctx).Create(key).Error; err != nil {
		return nil, "", fmt.Errorf("create API key: %w", err)
	}
	return key, plaintext, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, userID uint) ([]models.APIKey, error) {
	var keys []models.APIKey
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&keys).Error
	return keys, err
}

func (s *Service) RevokeAPIKey(ctx context.Context, userID, keyID uint) error {
	now := time.Now().UTC()
	res := s.db.WithContext(ctx).Model(&models.APIKey{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", keyID, userID).
		Update("revoked_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) AuthenticateAPIKey(ctx context.Context, token string) (*models.APIKey, *models.User, error) {
	token = strings.TrimSpace(token)
	parts := strings.SplitN(token, "_", 3)
	if len(parts) != 3 || parts[0] != "ksc" || parts[1] == "" || parts[2] == "" {
		return nil, nil, ErrAPIKeyInvalid
	}
	var key models.APIKey
	if err := s.db.WithContext(ctx).Where("public_id = ?", parts[1]).First(&key).Error; err != nil {
		return nil, nil, ErrAPIKeyInvalid
	}
	if subtle.ConstantTimeCompare([]byte(key.SecretHash), []byte(secret.HashToken(token))) != 1 {
		return nil, nil, ErrAPIKeyInvalid
	}
	now := time.Now().UTC()
	if key.RevokedAt != nil {
		return nil, nil, ErrAPIKeyInvalid
	}
	if !key.ExpiresAt.After(now) {
		return nil, nil, ErrAPIKeyExpired
	}
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, key.UserID).Error; err != nil {
		return nil, nil, ErrAPIKeyInvalid
	}
	if user.Status != "active" {
		return nil, nil, ErrAccountDisabled
	}
	// Throttle the write amplification from last_used_at.
	if key.LastUsedAt == nil || now.Sub(*key.LastUsedAt) > time.Minute {
		_ = s.db.WithContext(ctx).Model(&key).Update("last_used_at", now).Error
		key.LastUsedAt = &now
	}
	return &key, &user, nil
}
