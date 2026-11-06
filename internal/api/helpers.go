package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/kron-smart-chain/webwallet/internal/amount"
	"github.com/kron-smart-chain/webwallet/internal/models"
)

func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body: "+err.Error())
		return false
	}
	return true
}

func idParam(c *gin.Context, name string) (uint, bool) {
	raw := c.Param(name)
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n == 0 {
		fail(c, http.StatusBadRequest, "invalid_id", "Invalid identifier.")
		return 0, false
	}
	return uint(n), true
}

func normalizeContractAddress(s string) (string, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if len(s) != 40 {
		return "", fmt.Errorf("contract address must be 40 hex characters")
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", fmt.Errorf("contract address contains invalid hex")
	}
	return strings.ToLower(s), nil
}

func requestID(c *gin.Context) string { return c.GetString(ctxReqID) }

func audit(c *gin.Context, userID *uint, action, detail string) {
	s := c.MustGet("server").(*Server)
	var keyID *uint
	if key := currentAPIKey(c); key != nil {
		id := key.ID
		keyID = &id
	}
	s.auth.AuditWithKey(c.Request.Context(), userID, keyID, action, detail, c.ClientIP(), requestID(c))
}

func (s *Server) beginIntent(c *gin.Context, userID uint, kind, payload string) (*models.TxIntent, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" || len(key) > 80 {
		fail(c, http.StatusBadRequest, "idempotency_key_required", "Provide an Idempotency-Key header.")
		return nil, false
	}
	var existing models.TxIntent
	err := s.db.WithContext(c.Request.Context()).
		Where("user_id = ? AND idempotency_key = ?", userID, key).First(&existing).Error
	if err == nil {
		switch existing.Status {
		case "success":
			respondOK(c, gin.H{"txid": existing.TxID, "status": existing.Status, "replayed": true})
		case "pending":
			fail(c, http.StatusConflict, "in_progress", "An operation with this idempotency key is already in progress.")
		default:
			fail(c, http.StatusConflict, "previous_failed", "This idempotency key was already used by a failed operation. Use a new key.")
		}
		return nil, false
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		internal(c, err)
		return nil, false
	}
	var keyID *uint
	if apiKey := currentAPIKey(c); apiKey != nil {
		id := apiKey.ID
		keyID = &id
	}
	intent := &models.TxIntent{
		UserID:         userID,
		APIKeyID:       keyID,
		IdempotencyKey: key,
		Kind:           kind,
		Status:         "pending",
		Payload:        truncateString(payload, 4000),
	}
	if err := s.db.WithContext(c.Request.Context()).Create(intent).Error; err != nil {
		internal(c, err)
		return nil, false
	}
	return intent, true
}

func (s *Server) completeIntent(c *gin.Context, intent *models.TxIntent, txid string, opErr error) {
	updates := map[string]any{"status": "success", "tx_id": txid}
	if opErr != nil {
		updates["status"] = "failed"
		updates["error"] = truncateString(opErr.Error(), 2000)
	}
	_ = s.db.WithContext(c.Request.Context()).Model(intent).Updates(updates).Error
}

func (s *Server) gasParams(c *gin.Context, gasLimitRaw, gasPriceRaw string) (uint64, int64, error) {
	limit := s.cfg.DefaultGasLimit
	minPrice := int64(40)
	blockLimit := uint64(2_500_000)
	if dgp, err := s.rpc.GetDGPInfo(c.Request.Context()); err == nil {
		if dgp.MinGasPrice > 0 {
			minPrice = int64(dgp.MinGasPrice)
		}
		if dgp.BlockGasLimit > 0 {
			blockLimit = dgp.BlockGasLimit
		}
	}
	if minPrice < 40 {
		minPrice = 40
	}
	maxLimit := s.cfg.MaxContractGas
	if maxLimit == 0 || maxLimit > blockLimit {
		maxLimit = blockLimit
	}
	if strings.TrimSpace(gasLimitRaw) != "" {
		n, err := strconv.ParseUint(strings.TrimSpace(gasLimitRaw), 10, 64)
		if err != nil || n < 21_000 {
			return 0, 0, errors.New("gas limit must be an integer greater than or equal to 21000")
		}
		limit = n
	}
	if limit > maxLimit {
		return 0, 0, fmt.Errorf("gas limit exceeds the maximum of %d", maxLimit)
	}
	price := minPrice
	if strings.TrimSpace(gasPriceRaw) != "" {
		p, err := amount.ParseKSC(gasPriceRaw)
		if err != nil || p <= 0 {
			return 0, 0, errors.New("invalid gas price")
		}
		price = p
	}
	if price < minPrice {
		return 0, 0, fmt.Errorf("gas price is below the minimum of %s KSC", amount.FormatKSC(minPrice))
	}
	if s.cfg.MaxGasPriceSat > 0 && price > s.cfg.MaxGasPriceSat {
		return 0, 0, errors.New("gas price exceeds the configured maximum")
	}
	return limit, price, nil
}

func (s *Server) validateAddress(c *gin.Context, address string) bool {
	address = strings.TrimSpace(address)
	if address == "" {
		fail(c, http.StatusBadRequest, "invalid_address", "Address is required.")
		return false
	}
	result, err := s.rpc.ValidateAddress(c.Request.Context(), address)
	if err != nil {
		mapError(c, err)
		return false
	}
	if valid, _ := result["isvalid"].(bool); !valid {
		fail(c, http.StatusBadRequest, "invalid_address", "Address is not valid on this network.")
		return false
	}
	return true
}

func (s *Server) walletAddress(c *gin.Context, userID uint) (string, bool) {
	w, err := s.wallets.GetByUser(c.Request.Context(), userID)
	if err != nil {
		mapError(c, err)
		return "", false
	}
	return w.Address, true
}

func (s *Server) readOnlyWallet(c *gin.Context, userID uint, fn func(ctx context.Context, walletName string) error) bool {
	if err := s.wallets.Read(c.Request.Context(), userID, fn); err != nil {
		mapError(c, err)
		return false
	}
	return true
}

func marshalPayload(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
