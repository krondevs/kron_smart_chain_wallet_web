package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kron-smart-chain/webwallet/internal/auth"
	"github.com/kron-smart-chain/webwallet/internal/models"
)

type credsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func userJSON(u *models.User, walletAddress string, mfaVerified bool) gin.H {
	out := gin.H{
		"id":            u.ID,
		"username":      u.Username,
		"role":          u.Role,
		"status":        u.Status,
		"tier":          u.Tier,
		"totp_enabled":  u.TOTPEnabled,
		"per_tx_limit":  u.PerTxLimit,
		"daily_limit":   u.DailyLimit,
		"mfa_verified":  mfaVerified,
		"created_at":    u.CreatedAt,
		"last_login_at": u.LastLoginAt,
	}
	if walletAddress != "" {
		out["wallet_address"] = walletAddress
	}
	return out
}

func (s *Server) csrfToken(c *gin.Context) {
	if existing, err := c.Cookie(s.cfg.CSRFCookieName); err == nil && existing != "" {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"csrf_token": existing}})
		return
	}
	token := s.setCSRFCookie(c)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"csrf_token": token}})
}

func (s *Server) register(c *gin.Context) {
	if !s.cfg.RegistrationEnabled {
		fail(c, http.StatusForbidden, "registration_disabled", "Registration is currently closed.")
		return
	}
	chain, err := s.rpc.GetBlockchainInfo(c.Request.Context())
	if err != nil {
		fail(c, http.StatusServiceUnavailable, "node_unavailable", "The Kron node is not available. Try again later.")
		return
	}
	if chain.InitialBlockDownload {
		fail(c, http.StatusServiceUnavailable, "node_syncing", "The Kron node is still syncing. Try again later.")
		return
	}
	var req credsRequest
	if !bindJSON(c, &req) {
		return
	}
	result, err := s.auth.Register(c.Request.Context(), req.Username, req.Password, c.ClientIP())
	if err != nil {
		mapError(c, err)
		return
	}
	raw, sess, err := s.auth.CreateSession(c.Request.Context(), result.User, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		internal(c, err)
		return
	}
	s.setSessionCookie(c, raw, sess.ExpiresAt)
	s.auth.Audit(c.Request.Context(), &result.User.ID, "auth.register", "wallet="+result.Wallet.Name, c.ClientIP(), requestID(c))

	c.JSON(http.StatusCreated, gin.H{"data": gin.H{
		"user":         userJSON(result.User, result.Wallet.Address, false),
		"wallet":       gin.H{"address": result.Wallet.Address},
		"mfa_required": false,
	}})
}

func (s *Server) login(c *gin.Context) {
	var req credsRequest
	if !bindJSON(c, &req) {
		return
	}
	user, err := s.auth.Authenticate(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		s.auth.Audit(c.Request.Context(), nil, "auth.login_failed", "username="+strings.ToLower(strings.TrimSpace(req.Username)), c.ClientIP(), requestID(c))
		mapError(c, err)
		return
	}
	raw, sess, err := s.auth.CreateSession(c.Request.Context(), user, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		internal(c, err)
		return
	}
	s.setSessionCookie(c, raw, sess.ExpiresAt)
	s.auth.Audit(c.Request.Context(), &user.ID, "auth.login", "", c.ClientIP(), requestID(c))

	walletAddress := ""
	if w, err := s.wallets.GetByUser(c.Request.Context(), user.ID); err == nil {
		walletAddress = w.Address
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"user":         userJSON(user, walletAddress, false),
		"mfa_required": user.TOTPEnabled,
		"totp_enabled": user.TOTPEnabled,
	}})
}

func (s *Server) logout(c *gin.Context) {
	sess := currentSession(c)
	user := currentUser(c)
	if sess != nil {
		_ = s.auth.RevokeSession(c.Request.Context(), sess.UserID, sess.ID)
	}
	if user != nil {
		s.auth.Audit(c.Request.Context(), &user.ID, "auth.logout", "", c.ClientIP(), requestID(c))
	}
	s.clearSessionCookie(c)
	respondOK(c, gin.H{"logged_out": true})
}

func (s *Server) sessionInfo(c *gin.Context) {
	token, err := c.Cookie(s.cfg.SessionCookieName)
	if err != nil || token == "" {
		respondOK(c, gin.H{"user": nil, "wallet": gin.H{"address": ""}})
		return
	}
	sess, user, err := s.auth.GetSession(c.Request.Context(), token)
	if err != nil {
		respondOK(c, gin.H{"user": nil, "wallet": gin.H{"address": ""}})
		return
	}
	walletAddress := ""
	if w, err := s.wallets.GetByUser(c.Request.Context(), user.ID); err == nil {
		walletAddress = w.Address
	}
	respondOK(c, gin.H{
		"user":   userJSON(user, walletAddress, sess.MFAVerified),
		"wallet": gin.H{"address": walletAddress},
	})
}

func (s *Server) me(c *gin.Context) {
	user := currentUser(c)
	sess := currentSession(c)
	walletAddress := ""
	if w, err := s.wallets.GetByUser(c.Request.Context(), user.ID); err == nil {
		walletAddress = w.Address
	}
	respondOK(c, gin.H{
		"user":   userJSON(user, walletAddress, sess.MFAVerified),
		"wallet": gin.H{"address": walletAddress},
	})
}

type totpRequest struct {
	Code string `json:"code"`
}

func (s *Server) verifyTOTP(c *gin.Context) {
	user := currentUser(c)
	sess := currentSession(c)
	var req totpRequest
	if !bindJSON(c, &req) {
		return
	}
	if !user.TOTPEnabled {
		if err := s.auth.EnableTOTP(c.Request.Context(), user, req.Code); err != nil {
			mapError(c, err)
			return
		}
	} else if err := s.auth.VerifyTOTP(user, req.Code); err != nil {
		mapError(c, err)
		return
	}
	if err := s.auth.MarkMFAVerified(c.Request.Context(), sess); err != nil {
		internal(c, err)
		return
	}
	s.auth.Audit(c.Request.Context(), &user.ID, "auth.mfa_verified", "", c.ClientIP(), requestID(c))
	respondOK(c, gin.H{"mfa_verified": true, "totp_enabled": true})
}

func (s *Server) setupTOTP(c *gin.Context) {
	user := currentUser(c)
	if user.TOTPEnabled {
		fail(c, http.StatusBadRequest, "totp_already_enabled", "TOTP is already enabled. Use an administrator reset first.")
		return
	}
	secretValue, otpauthURL, recovery, err := s.auth.SetupTOTP(c.Request.Context(), user)
	if err != nil {
		internal(c, err)
		return
	}
	s.auth.Audit(c.Request.Context(), &user.ID, "auth.totp_setup", "", c.ClientIP(), requestID(c))
	respondOK(c, gin.H{"secret": secretValue, "otpauth_url": otpauthURL, "recovery_codes": recovery})
}

type recoveryRequest struct {
	Code string `json:"code"`
}

func (s *Server) useRecovery(c *gin.Context) {
	user := currentUser(c)
	sess := currentSession(c)
	var req recoveryRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := s.auth.UseRecoveryCode(c.Request.Context(), user, req.Code); err != nil {
		mapError(c, err)
		return
	}
	if err := s.auth.MarkMFAVerified(c.Request.Context(), sess); err != nil {
		internal(c, err)
		return
	}
	s.auth.Audit(c.Request.Context(), &user.ID, "auth.recovery_code_used", "", c.ClientIP(), requestID(c))
	respondOK(c, gin.H{"mfa_verified": true})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) changePassword(c *gin.Context) {
	user := currentUser(c)
	var req changePasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := s.auth.ChangePassword(c.Request.Context(), user, req.CurrentPassword, req.NewPassword); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			fail(c, http.StatusBadRequest, "invalid_password", "Current password is incorrect.")
			return
		}
		mapError(c, err)
		return
	}
	s.auth.Audit(c.Request.Context(), &user.ID, "auth.password_changed", "", c.ClientIP(), requestID(c))
	s.clearSessionCookie(c)
	respondOK(c, gin.H{"password_changed": true, "reauthentication_required": true})
}

type disableTOTPRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func (s *Server) disableTOTP(c *gin.Context) {
	user := currentUser(c)
	var req disableTOTPRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := s.auth.DisableTOTP(c.Request.Context(), user, req.Password, req.Code); err != nil {
		mapError(c, err)
		return
	}
	s.auth.Audit(c.Request.Context(), &user.ID, "auth.totp_disabled", "", c.ClientIP(), requestID(c))
	respondOK(c, gin.H{"totp_enabled": false})
}

func (s *Server) regenerateRecoveryCodes(c *gin.Context) {
	user := currentUser(c)
	codes, err := s.auth.RegenerateRecoveryCodes(c.Request.Context(), user)
	if err != nil {
		internal(c, err)
		return
	}
	s.auth.Audit(c.Request.Context(), &user.ID, "auth.recovery_codes_regenerated", "", c.ClientIP(), requestID(c))
	respondOK(c, gin.H{"recovery_codes": codes})
}

func (s *Server) listSessions(c *gin.Context) {
	user := currentUser(c)
	current := currentSession(c)
	sessions, err := s.auth.ListSessions(c.Request.Context(), user.ID)
	if err != nil {
		internal(c, err)
		return
	}
	out := make([]gin.H, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, gin.H{
			"id":           sess.ID,
			"ip":           sess.IP,
			"user_agent":   sess.UserAgent,
			"created_at":   sess.CreatedAt,
			"expires_at":   sess.ExpiresAt,
			"mfa_verified": sess.MFAVerified,
			"current":      current != nil && current.ID == sess.ID,
		})
	}
	respondOK(c, gin.H{"sessions": out})
}

func (s *Server) revokeSession(c *gin.Context) {
	user := currentUser(c)
	id, valid := idParam(c, "id")
	if !valid {
		return
	}
	if err := s.auth.RevokeSession(c.Request.Context(), user.ID, id); err != nil {
		internal(c, err)
		return
	}
	s.auth.Audit(c.Request.Context(), &user.ID, "auth.session_revoked", "session_id="+strconv.FormatUint(uint64(id), 10), c.ClientIP(), requestID(c))
	if current := currentSession(c); current != nil && current.ID == id {
		s.clearSessionCookie(c)
	}
	respondOK(c, gin.H{"revoked": true})
}
