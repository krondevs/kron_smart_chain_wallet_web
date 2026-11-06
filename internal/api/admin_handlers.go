package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kron-smart-chain/webwallet/internal/amount"
	"github.com/kron-smart-chain/webwallet/internal/models"
)

func (s *Server) adminUsers(c *gin.Context) {
	page := 1
	perPage := 50
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	offset := (page - 1) * perPage
	q := s.db.WithContext(c.Request.Context()).Model(&models.User{})
	if search := strings.TrimSpace(c.Query("q")); search != "" {
		q = q.Where("username LIKE ?", "%"+strings.ToLower(search)+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		internal(c, err)
		return
	}
	var users []models.User
	if err := q.Order("id DESC").Limit(perPage).Offset(offset).Find(&users).Error; err != nil {
		internal(c, err)
		return
	}
	out := make([]gin.H, 0, len(users))
	for i := range users {
		walletAddress := ""
		if w, err := s.wallets.GetByUser(c.Request.Context(), users[i].ID); err == nil {
			walletAddress = w.Address
		}
		item := gin.H{
			"id":             users[i].ID,
			"username":       users[i].Username,
			"role":           users[i].Role,
			"status":         users[i].Status,
			"tier":           users[i].Tier,
			"totp_enabled":   users[i].TOTPEnabled,
			"per_tx_limit":   users[i].PerTxLimit,
			"daily_limit":    users[i].DailyLimit,
			"created_at":     users[i].CreatedAt,
			"last_login_at":  users[i].LastLoginAt,
			"wallet_address": walletAddress,
		}
		out = append(out, item)
	}
	respondOK(c, gin.H{"users": out, "total": total, "page": page, "per_page": perPage})
}

func (s *Server) adminUser(c *gin.Context) (*models.User, bool) {
	id, valid := idParam(c, "id")
	if !valid {
		return nil, false
	}
	var user models.User
	if err := s.db.WithContext(c.Request.Context()).First(&user, id).Error; err != nil {
		mapError(c, err)
		return nil, false
	}
	return &user, true
}

type adminStatusRequest struct {
	Status string `json:"status"`
}

func (s *Server) adminSetStatus(c *gin.Context) {
	admin := currentUser(c)
	user, ok := s.adminUser(c)
	if !ok {
		return
	}
	var req adminStatusRequest
	if !bindJSON(c, &req) {
		return
	}
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if req.Status != "active" && req.Status != "suspended" {
		fail(c, http.StatusBadRequest, "invalid_status", "Status must be active or suspended.")
		return
	}
	if user.ID == admin.ID && req.Status == "suspended" {
		fail(c, http.StatusBadRequest, "invalid_status", "You cannot suspend your own account.")
		return
	}
	if err := s.db.WithContext(c.Request.Context()).Model(user).Update("status", req.Status).Error; err != nil {
		internal(c, err)
		return
	}
	if req.Status == "suspended" {
		_ = s.auth.RevokeAllSessions(c.Request.Context(), user.ID)
	}
	audit(c, &admin.ID, "admin.user_status", "user_id="+strconv.FormatUint(uint64(user.ID), 10)+" status="+req.Status)
	respondOK(c, gin.H{"id": user.ID, "status": req.Status})
}

type adminLimitsRequest struct {
	PerTxLimit string `json:"per_tx_limit"`
	DailyLimit string `json:"daily_limit"`
	Tier       *int   `json:"tier"`
}

func (s *Server) adminSetLimits(c *gin.Context) {
	admin := currentUser(c)
	user, ok := s.adminUser(c)
	if !ok {
		return
	}
	var req adminLimitsRequest
	if !bindJSON(c, &req) {
		return
	}
	perTx, err := amount.ParseKSC(req.PerTxLimit)
	if err != nil || perTx < 0 {
		fail(c, http.StatusBadRequest, "invalid_limit", "Invalid per-transaction limit.")
		return
	}
	daily, err := amount.ParseKSC(req.DailyLimit)
	if err != nil || daily < 0 {
		fail(c, http.StatusBadRequest, "invalid_limit", "Invalid daily limit.")
		return
	}
	updates := map[string]any{
		"per_tx_limit": perTx,
		"daily_limit":  daily,
	}
	if req.Tier != nil {
		updates["tier"] = *req.Tier
	}
	if err := s.db.WithContext(c.Request.Context()).Model(user).Updates(updates).Error; err != nil {
		internal(c, err)
		return
	}
	audit(c, &admin.ID, "admin.user_limits", "user_id="+strconv.FormatUint(uint64(user.ID), 10)+" per_tx="+strconv.FormatInt(perTx, 10)+" daily="+strconv.FormatInt(daily, 10))
	respondOK(c, gin.H{"id": user.ID, "per_tx_limit": perTx, "daily_limit": daily, "tier": user.Tier})
}

type adminResetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

func (s *Server) adminResetPassword(c *gin.Context) {
	admin := currentUser(c)
	user, ok := s.adminUser(c)
	if !ok {
		return
	}
	var req adminResetPasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := s.auth.ResetPassword(c.Request.Context(), user, req.NewPassword); err != nil {
		mapError(c, err)
		return
	}
	audit(c, &admin.ID, "admin.password_reset", "user_id="+strconv.FormatUint(uint64(user.ID), 10))
	respondOK(c, gin.H{"reset": true})
}

func (s *Server) adminResetTOTP(c *gin.Context) {
	admin := currentUser(c)
	user, ok := s.adminUser(c)
	if !ok {
		return
	}
	secret, url, recovery, err := s.auth.ResetTOTP(c.Request.Context(), user)
	if err != nil {
		internal(c, err)
		return
	}
	audit(c, &admin.ID, "admin.totp_reset", "user_id="+strconv.FormatUint(uint64(user.ID), 10))
	respondOK(c, gin.H{"secret": secret, "otpauth_url": url, "recovery_codes": recovery})
}

func (s *Server) adminProvision(c *gin.Context) {
	admin := currentUser(c)
	user, ok := s.adminUser(c)
	if !ok {
		return
	}
	if existing, err := s.wallets.GetByUser(c.Request.Context(), user.ID); err == nil {
		respondOK(c, gin.H{"created": false, "address": existing.Address})
		return
	}
	w, err := s.wallets.Provision(c.Request.Context(), user)
	if err != nil {
		mapError(c, err)
		return
	}
	audit(c, &admin.ID, "admin.provisioned_wallet", "user_id="+strconv.FormatUint(uint64(user.ID), 10)+" wallet="+w.Name)
	created(c, gin.H{"created": true, "address": w.Address})
}

func (s *Server) adminTokens(c *gin.Context) {
	var tokens []models.Token
	if err := s.db.WithContext(c.Request.Context()).Order("created_at DESC").Find(&tokens).Error; err != nil {
		internal(c, err)
		return
	}
	out := make([]gin.H, 0, len(tokens))
	for i := range tokens {
		var users int64
		_ = s.db.WithContext(c.Request.Context()).Model(&models.UserToken{}).Where("token_id = ?", tokens[i].ID).Count(&users).Error
		out = append(out, gin.H{
			"id":           tokens[i].ID,
			"contract":     tokens[i].Contract,
			"name":         tokens[i].Name,
			"symbol":       tokens[i].Symbol,
			"decimals":     tokens[i].Decimals,
			"total_supply": tokens[i].TotalSupply,
			"verified":     tokens[i].Verified,
			"users":        users,
			"created_at":   tokens[i].CreatedAt,
		})
	}
	respondOK(c, gin.H{"tokens": out})
}

type adminVerifyTokenRequest struct {
	Verified bool `json:"verified"`
}

func (s *Server) adminVerifyToken(c *gin.Context) {
	admin := currentUser(c)
	id, valid := idParam(c, "id")
	if !valid {
		return
	}
	var req adminVerifyTokenRequest
	if !bindJSON(c, &req) {
		return
	}
	var token models.Token
	if err := s.db.WithContext(c.Request.Context()).First(&token, id).Error; err != nil {
		mapError(c, err)
		return
	}
	if err := s.db.WithContext(c.Request.Context()).Model(&token).Update("verified", req.Verified).Error; err != nil {
		internal(c, err)
		return
	}
	audit(c, &admin.ID, "admin.token_verified", "token_id="+strconv.FormatUint(uint64(token.ID), 10)+" verified="+strconv.FormatBool(req.Verified))
	respondOK(c, gin.H{"id": token.ID, "verified": req.Verified})
}

func (s *Server) adminAudit(c *gin.Context) {
	page := 1
	perPage := 100
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	q := s.db.WithContext(c.Request.Context()).Model(&models.AuditEvent{})
	if action := strings.TrimSpace(c.Query("action")); action != "" {
		q = q.Where("action = ?", action)
	}
	if userID := strings.TrimSpace(c.Query("user_id")); userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		internal(c, err)
		return
	}
	var events []models.AuditEvent
	if err := q.Order("id DESC").Limit(perPage).Offset((page - 1) * perPage).Find(&events).Error; err != nil {
		internal(c, err)
		return
	}
	respondOK(c, gin.H{"events": events, "total": total, "page": page, "per_page": perPage})
}

func (s *Server) adminStats(c *gin.Context) {
	var users, wallets, sessions, tokens, contracts, intents int64
	_ = s.db.WithContext(c.Request.Context()).Model(&models.User{}).Count(&users).Error
	_ = s.db.WithContext(c.Request.Context()).Model(&models.Wallet{}).Count(&wallets).Error
	_ = s.db.WithContext(c.Request.Context()).Model(&models.Session{}).Where("revoked_at IS NULL").Count(&sessions).Error
	_ = s.db.WithContext(c.Request.Context()).Model(&models.Token{}).Count(&tokens).Error
	_ = s.db.WithContext(c.Request.Context()).Model(&models.Contract{}).Count(&contracts).Error
	_ = s.db.WithContext(c.Request.Context()).Model(&models.TxIntent{}).Count(&intents).Error
	chain, chainErr := s.rpc.GetBlockchainInfo(c.Request.Context())
	dgp, dgpErr := s.rpc.GetDGPInfo(c.Request.Context())
	out := gin.H{
		"users":      users,
		"wallets":    wallets,
		"sessions":   sessions,
		"tokens":     tokens,
		"contracts":  contracts,
		"tx_intents": intents,
	}
	if chainErr == nil {
		out["chain"] = gin.H{
			"chain":                chain.Chain,
			"blocks":               chain.Blocks,
			"bestblockhash":        chain.BestBlockHash,
			"initialblockdownload": chain.InitialBlockDownload,
			"verificationprogress": chain.VerificationProgress,
		}
	} else {
		out["chain_error"] = chainErr.Error()
	}
	if dgpErr == nil {
		out["gas"] = gin.H{
			"min_gas_price_sat": dgp.MinGasPrice,
			"block_gas_limit":   dgp.BlockGasLimit,
		}
	}
	respondOK(c, out)
}
