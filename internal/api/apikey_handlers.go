package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/kron-smart-chain/webwallet/internal/auth"
	"github.com/kron-smart-chain/webwallet/internal/models"
)

func apiKeyJSON(key *models.APIKey) gin.H {
	status := "active"
	now := time.Now().UTC()
	if key.RevokedAt != nil {
		status = "revoked"
	} else if !key.ExpiresAt.After(now) {
		status = "expired"
	}
	return gin.H{
		"id":           key.ID,
		"name":         key.Name,
		"prefix":       "ksc_" + key.PublicID,
		"expires_at":   key.ExpiresAt,
		"last_used_at": key.LastUsedAt,
		"revoked_at":   key.RevokedAt,
		"created_at":   key.CreatedAt,
		"status":       status,
		"rate_limit":   auth.APIKeyRateLimit,
	}
}

func (s *Server) listAPIKeys(c *gin.Context) {
	user := currentUser(c)
	keys, err := s.auth.ListAPIKeys(c.Request.Context(), user.ID)
	if err != nil {
		internal(c, err)
		return
	}
	out := make([]gin.H, 0, len(keys))
	for i := range keys {
		out = append(out, apiKeyJSON(&keys[i]))
	}
	respondOK(c, gin.H{"api_keys": out, "max_active": auth.MaxActiveAPIKeys})
}

type createAPIKeyRequest struct {
	Name          string `json:"name"`
	ExpiresAt     string `json:"expires_at"`
	ExpiresInDays int    `json:"expires_in_days"`
}

func (s *Server) createAPIKey(c *gin.Context) {
	user := currentUser(c)
	var req createAPIKeyRequest
	if !bindJSON(c, &req) {
		return
	}
	var expiresAt time.Time
	if strings.TrimSpace(req.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ExpiresAt))
		if err != nil {
			fail(c, http.StatusBadRequest, "invalid_expiration", "Expires_at must be an RFC3339 date/time.")
			return
		}
		expiresAt = parsed
	} else if req.ExpiresInDays > 0 {
		expiresAt = time.Now().UTC().Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour)
	} else {
		fail(c, http.StatusBadRequest, "invalid_expiration", "Choose an expiration date.")
		return
	}
	key, plaintext, err := s.auth.CreateAPIKey(c.Request.Context(), user, req.Name, expiresAt)
	if err != nil {
		if errors.Is(err, auth.ErrAPIKeyLimit) {
			fail(c, http.StatusBadRequest, "api_key_limit", "Maximum of 3 active API keys reached.")
			return
		}
		fail(c, http.StatusBadRequest, "invalid_api_key", err.Error())
		return
	}
	audit(c, &user.ID, "api_key.created", "name="+key.Name+" prefix=ksc_"+key.PublicID)
	respondOK(c, gin.H{
		"api_key": apiKeyJSON(key),
		"key":     plaintext,
	})
}

func (s *Server) revokeAPIKey(c *gin.Context) {
	user := currentUser(c)
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	if err := s.auth.RevokeAPIKey(c.Request.Context(), user.ID, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusNotFound, "not_found", "API key not found.")
			return
		}
		internal(c, err)
		return
	}
	audit(c, &user.ID, "api_key.revoked", "api_key_id="+strconv.FormatUint(uint64(id), 10))
	respondOK(c, gin.H{"revoked": true})
}

func (s *Server) appMe(c *gin.Context) {
	user := currentUser(c)
	key := currentAPIKey(c)
	walletAddress := ""
	if w, err := s.wallets.GetByUser(c.Request.Context(), user.ID); err == nil {
		walletAddress = w.Address
	}
	out := gin.H{
		"user":   userJSON(user, walletAddress, false),
		"wallet": gin.H{"address": walletAddress},
		"rate_limit": gin.H{
			"limit":  auth.APIKeyRateLimit,
			"period": "60s",
		},
	}
	if key != nil {
		out["api_key"] = apiKeyJSON(key)
	}
	respondOK(c, out)
}
