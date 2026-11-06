package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"github.com/kron-smart-chain/webwallet/internal/auth"
	"github.com/kron-smart-chain/webwallet/internal/config"
	"github.com/kron-smart-chain/webwallet/internal/models"
	"github.com/kron-smart-chain/webwallet/internal/ratelimit"
	"github.com/kron-smart-chain/webwallet/internal/rpc"
	"github.com/kron-smart-chain/webwallet/internal/secret"
	"github.com/kron-smart-chain/webwallet/internal/wallet"
	"github.com/kron-smart-chain/webwallet/internal/webui"
)

const (
	ctxUser       = "user"
	ctxSession    = "session"
	ctxToken      = "session_token"
	ctxReqID      = "request_id"
	ctxAPIKey     = "api_key"
	ctxAuthMethod = "auth_method"
)

type Server struct {
	cfg           *config.Config
	db            *gorm.DB
	rpc           *rpc.Client
	secrets       *secret.Manager
	auth          *auth.Service
	wallets       *wallet.Service
	router        *gin.Engine
	limiters      map[string]*ratelimit.Limiter
	apiKeyLimiter *ratelimit.Window
	static        fs.FS
}

func New(cfg *config.Config, db *gorm.DB, client *rpc.Client, secrets *secret.Manager, authSvc *auth.Service, walletSvc *wallet.Service) (*Server, error) {
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	sub, err := fs.Sub(webui.Files, "dist")
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:           cfg,
		db:            db,
		rpc:           client,
		secrets:       secrets,
		auth:          authSvc,
		wallets:       walletSvc,
		router:        gin.New(),
		limiters:      make(map[string]*ratelimit.Limiter),
		apiKeyLimiter: ratelimit.NewWindow(auth.APIKeyRateLimit, time.Minute),
		static:        sub,
	}
	if len(cfg.TrustedProxies) > 0 {
		if err := s.router.SetTrustedProxies(cfg.TrustedProxies); err != nil {
			return nil, fmt.Errorf("trusted proxies: %w", err)
		}
	} else {
		_ = s.router.SetTrustedProxies(nil)
	}
	s.limiters["login"] = ratelimit.New(rate.Every(12*time.Second), 5)
	s.limiters["register"] = ratelimit.New(rate.Every(15*time.Minute), 3)
	s.limiters["send"] = ratelimit.New(rate.Every(6*time.Second), 10)
	s.limiters["contract"] = ratelimit.New(rate.Every(2*time.Second), 30)
	s.limiters["token"] = ratelimit.New(rate.Every(3*time.Minute), 20)
	s.limiters["recovery"] = ratelimit.New(rate.Every(30*time.Second), 5)

	s.routes()
	return s, nil
}

func (s *Server) Router() *gin.Engine { return s.router }

func (s *Server) routes() {
	r := s.router
	r.Use(gin.Recovery())
	r.Use(s.requestID())
	r.Use(s.securityHeaders())
	r.Use(s.bodyLimit(4 << 20))
	r.Use(s.accessLog())
	r.Use(s.csrf())

	r.GET("/healthz", s.health)
	r.GET("/metrics", s.metrics)
	r.GET("/api/v1/openapi.json", s.openapiJSON)
	r.GET("/llms.txt", s.llmsTXT)

	api := r.Group("/api/v1")
	api.GET("/auth/csrf", s.csrfToken)
	api.GET("/auth/session", s.sessionInfo)

	api.POST("/auth/register", s.rate("register"), s.register)
	api.POST("/auth/login", s.rate("login"), s.login)

	protected := api.Group("")
	protected.Use(s.requireAuth())
	{
		protected.POST("/auth/logout", s.logout)
		protected.GET("/auth/me", s.me)
		protected.POST("/auth/totp/verify", s.rate("recovery"), s.verifyTOTP)
		protected.POST("/auth/totp/setup", s.setupTOTP)
		protected.POST("/auth/totp/disable", s.rate("recovery"), s.disableTOTP)
		protected.POST("/auth/recovery", s.rate("recovery"), s.useRecovery)
		protected.POST("/auth/recovery-codes/regenerate", s.rate("recovery"), s.requireSensitive(), s.regenerateRecoveryCodes)
		protected.POST("/auth/password", s.requireSensitive(), s.changePassword)
		protected.GET("/auth/sessions", s.listSessions)
		protected.GET("/api-keys", s.listAPIKeys)
		protected.POST("/api-keys", s.requireSensitive(), s.createAPIKey)
		protected.DELETE("/api-keys/:id", s.requireSensitive(), s.revokeAPIKey)
		protected.DELETE("/auth/sessions/:id", s.revokeSession)

		protected.GET("/wallet", s.getWallet)
		protected.GET("/wallet/address", s.getAddress)
		protected.GET("/wallet/transactions", s.listTransactions)
		protected.GET("/wallet/transactions/:txid", s.getTransaction)
		protected.GET("/wallet/fee-estimate", s.feeEstimate)
		protected.POST("/wallet/send", s.rate("send"), s.requireSensitive(), s.send)
		protected.POST("/wallet/consolidate", s.rate("send"), s.requireSensitive(), s.consolidate)
		protected.POST("/wallet/prepare-gas", s.rate("send"), s.requireSensitive(), s.prepareGas)

		protected.GET("/tokens", s.listTokens)
		protected.POST("/tokens", s.rate("token"), s.addToken)
		protected.DELETE("/tokens/:contract", s.removeToken)
		protected.GET("/tokens/:contract", s.getToken)
		protected.GET("/tokens/:contract/transactions", s.tokenTransactions)
		protected.GET("/tokens/:contract/allowance", s.tokenAllowance)
		protected.POST("/tokens/:contract/transfer", s.rate("send"), s.requireSensitive(), s.tokenTransfer)
		protected.POST("/tokens/:contract/approve", s.rate("send"), s.requireSensitive(), s.tokenApprove)
		protected.POST("/tokens/:contract/transfer-from", s.rate("send"), s.requireSensitive(), s.tokenTransferFrom)
		protected.POST("/tokens/:contract/burn", s.rate("send"), s.requireSensitive(), s.tokenBurn)
		protected.POST("/tokens/:contract/burn-from", s.rate("send"), s.requireSensitive(), s.tokenBurnFrom)

		protected.GET("/contracts", s.listContracts)
		protected.POST("/contracts", s.rate("token"), s.addContract)
		protected.GET("/contracts/:address", s.getContract)
		protected.DELETE("/contracts/:address", s.deleteContract)
		protected.POST("/contracts/read", s.rate("contract"), s.callContract)
		protected.POST("/contracts/write", s.rate("contract"), s.requireSensitive(), s.writeContract)
		protected.POST("/contracts/deploy", s.rate("contract"), s.requireSensitive(), s.deployContract)
		protected.GET("/contracts/:address/receipts", s.contractReceipts)
		protected.GET("/contracts/:address/logs", s.contractLogs)
		protected.GET("/contracts/:address/code", s.contractCode)

		protected.GET("/address-book", s.listAddressBook)
		protected.POST("/address-book", s.addAddressBook)
		protected.DELETE("/address-book/:id", s.deleteAddressBook)

		admin := protected.Group("/admin", s.requireAdmin())
		{
			admin.GET("/users", s.adminUsers)
			admin.POST("/users/:id/status", s.requireSensitive(), s.adminSetStatus)
			admin.POST("/users/:id/limits", s.requireSensitive(), s.adminSetLimits)
			admin.POST("/users/:id/reset-password", s.requireSensitive(), s.adminResetPassword)
			admin.POST("/users/:id/reset-totp", s.requireSensitive(), s.adminResetTOTP)
			admin.GET("/tokens", s.adminTokens)
			admin.POST("/tokens/:id/verify", s.requireSensitive(), s.adminVerifyToken)
			admin.GET("/audit", s.adminAudit)
			admin.GET("/stats", s.adminStats)
			admin.POST("/users/:id/provision", s.requireSensitive(), s.adminProvision)
		}
	}

	app := r.Group("/api/v1/app")
	app.Use(s.requireAPIKey())
	{
		app.GET("/me", s.appMe)
		app.GET("/wallet", s.getWallet)
		app.GET("/wallet/address", s.getAddress)
		app.GET("/wallet/transactions", s.listTransactions)
		app.GET("/wallet/transactions/:txid", s.getTransaction)
		app.GET("/wallet/fee-estimate", s.feeEstimate)
		app.POST("/wallet/send", s.send)
		app.POST("/wallet/consolidate", s.consolidate)
		app.POST("/wallet/prepare-gas", s.prepareGas)

		app.GET("/tokens", s.listTokens)
		app.POST("/tokens", s.addToken)
		app.DELETE("/tokens/:contract", s.removeToken)
		app.GET("/tokens/:contract", s.getToken)
		app.GET("/tokens/:contract/transactions", s.tokenTransactions)
		app.GET("/tokens/:contract/allowance", s.tokenAllowance)
		app.POST("/tokens/:contract/transfer", s.tokenTransfer)
		app.POST("/tokens/:contract/approve", s.tokenApprove)
		app.POST("/tokens/:contract/transfer-from", s.tokenTransferFrom)
		app.POST("/tokens/:contract/burn", s.tokenBurn)
		app.POST("/tokens/:contract/burn-from", s.tokenBurnFrom)

		app.GET("/contracts", s.listContracts)
		app.POST("/contracts", s.addContract)
		app.GET("/contracts/:address", s.getContract)
		app.DELETE("/contracts/:address", s.deleteContract)
		app.POST("/contracts/read", s.callContract)
		app.POST("/contracts/write", s.writeContract)
		app.POST("/contracts/deploy", s.deployContract)
		app.GET("/contracts/:address/receipts", s.contractReceipts)
		app.GET("/contracts/:address/logs", s.contractLogs)
		app.GET("/contracts/:address/code", s.contractCode)

		app.GET("/address-book", s.listAddressBook)
		app.POST("/address-book", s.addAddressBook)
		app.DELETE("/address-book/:id", s.deleteAddressBook)
	}

	r.NoRoute(s.serveStatic)
}

// ---- middleware ----

func (s *Server) requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if id == "" || len(id) > 64 {
			id = uuid.NewString()
		}
		c.Set(ctxReqID, id)
		c.Set("server", s)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func (s *Server) securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Permissions-Policy", "camera=(self), microphone=(), geolocation=()")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Header("Cache-Control", "no-store")
		}
		if s.cfg.CookieSecure {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

func (s *Server) bodyLimit(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		}
		c.Next()
	}
}

func (s *Server) accessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if strings.HasPrefix(c.Request.URL.Path, "/assets/") {
			return
		}
		slog.Info("http",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
			"request_id", c.GetString(ctxReqID),
		)
	}
}

func (s *Server) csrf() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			if _, err := c.Cookie(s.cfg.CSRFCookieName); err != nil {
				s.setCSRFCookie(c)
			}
			c.Next()
			return
		}
		// Bearer API-key requests are not cookie-authenticated and therefore
		// are not vulnerable to CSRF.
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/app/") {
			c.Next()
			return
		}
		cookie, err := c.Cookie(s.cfg.CSRFCookieName)
		if err != nil || cookie == "" {
			fail(c, http.StatusForbidden, "csrf_failed", "CSRF token is missing. Reload the page and try again.")
			return
		}
		header := c.GetHeader("X-CSRF-Token")
		if header == "" || subtle.ConstantTimeCompare([]byte(cookie), []byte(header)) != 1 {
			fail(c, http.StatusForbidden, "csrf_failed", "CSRF token is invalid. Reload the page and try again.")
			return
		}
		c.Next()
	}
}

func (s *Server) setCSRFCookie(c *gin.Context) string {
	c.SetSameSite(http.SameSiteStrictMode)
	token := c.GetString("csrf_token")
	if token == "" {
		var err error
		token, err = secret.RandomToken(32)
		if err != nil {
			token = uuid.NewString()
		}
	}
	// The token is HttpOnly and is returned by /auth/csrf. The SPA keeps it in
	// memory and sends it in X-CSRF-Token.
	c.SetCookie(s.cfg.CSRFCookieName, token, 12*3600, "/", "", s.cfg.CookieSecure, true)
	c.Set("csrf_token", token)
	return token
}

func (s *Server) rate(name string) gin.HandlerFunc {
	limiter := s.limiters[name]
	if limiter == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		if !limiter.Allow(c.ClientIP() + "|" + name) {
			fail(c, http.StatusTooManyRequests, "rate_limited", "Too many requests. Please wait and try again.")
			return
		}
		c.Next()
	}
}

func (s *Server) requireAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if token == "" {
			authz := strings.TrimSpace(c.GetHeader("Authorization"))
			if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
				token = strings.TrimSpace(authz[7:])
			}
		}
		if token == "" {
			fail(c, http.StatusUnauthorized, "api_key_required", "Provide Authorization: Bearer <api_key> or X-API-Key.")
			return
		}
		key, user, err := s.auth.AuthenticateAPIKey(c.Request.Context(), token)
		if err != nil {
			code := "api_key_invalid"
			if errors.Is(err, auth.ErrAPIKeyExpired) {
				code = "api_key_expired"
			}
			fail(c, http.StatusUnauthorized, code, "Invalid or expired API key.")
			return
		}
		allowed, remaining, reset := s.apiKeyLimiter.Allow(fmt.Sprintf("%d", key.ID))
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", auth.APIKeyRateLimit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		seconds := int(reset.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", seconds))
		if !allowed {
			c.Header("Retry-After", fmt.Sprintf("%d", seconds))
			fail(c, http.StatusTooManyRequests, "rate_limited", "API key rate limit exceeded: 30 requests per minute.")
			return
		}
		c.Set(ctxUser, user)
		c.Set(ctxAPIKey, key)
		c.Set(ctxAuthMethod, "api_key")
		c.Next()
	}
}

func (s *Server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(s.cfg.SessionCookieName)
		if err != nil || token == "" {
			fail(c, http.StatusUnauthorized, "unauthorized", "Authentication required.")
			return
		}
		sess, user, err := s.auth.GetSession(c.Request.Context(), token)
		if err != nil {
			s.clearSessionCookie(c)
			fail(c, http.StatusUnauthorized, "unauthorized", "Session expired. Please sign in again.")
			return
		}
		c.Set(ctxUser, user)
		c.Set(ctxSession, sess)
		c.Set(ctxToken, token)
		c.Next()
	}
}

func (s *Server) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := currentUser(c)
		if user == nil || user.Role != "admin" {
			fail(c, http.StatusForbidden, "forbidden", "Administrator access required.")
			return
		}
		c.Next()
	}
}

func (s *Server) checkMFA(c *gin.Context) bool {
	user := currentUser(c)
	sess := currentSession(c)
	if user == nil || sess == nil {
		fail(c, http.StatusUnauthorized, "unauthorized", "Authentication required.")
		return false
	}
	if !user.TOTPEnabled {
		return true
	}
	if !sess.MFAVerified {
		fail(c, http.StatusForbidden, "mfa_required", "Two-factor authentication is required.")
		return false
	}
	return true
}

func (s *Server) requireMFA() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.checkMFA(c) {
			return
		}
		c.Next()
	}
}

func (s *Server) requireSensitive() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.checkMFA(c) {
			return
		}
		user := currentUser(c)
		if user.TOTPEnabled {
			sess := currentSession(c)
			if sess.MFAVerifiedAt == nil || time.Since(*sess.MFAVerifiedAt) > s.cfg.MFAReverifyAfter {
				fail(c, http.StatusForbidden, "reauth_required", "Please confirm your two-factor code again.")
				return
			}
		}
		c.Next()
	}
}

func currentUser(c *gin.Context) *models.User {
	v, ok := c.Get(ctxUser)
	if !ok {
		return nil
	}
	u, _ := v.(*models.User)
	return u
}

func currentAPIKey(c *gin.Context) *models.APIKey {
	v, ok := c.Get(ctxAPIKey)
	if !ok {
		return nil
	}
	k, _ := v.(*models.APIKey)
	return k
}

func currentSession(c *gin.Context) *models.Session {
	v, ok := c.Get(ctxSession)
	if !ok {
		return nil
	}
	s, _ := v.(*models.Session)
	return s
}

// ---- cookie helpers ----

func (s *Server) setSessionCookie(c *gin.Context, token string, expires time.Time) {
	maxAge := int(time.Until(expires).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(s.cfg.SessionCookieName, token, maxAge, "/", "", s.cfg.CookieSecure, true)
}

func (s *Server) clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(s.cfg.SessionCookieName, "", -1, "/", "", s.cfg.CookieSecure, true)
	c.SetCookie(s.cfg.CSRFCookieName, "", -1, "/", "", s.cfg.CookieSecure, true)
}

// ---- responses ----

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, gin.H{"data": data})
}

func fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

func internal(c *gin.Context, err error) {
	slog.Error("request failed", "error", err, "path", c.Request.URL.Path, "request_id", c.GetString(ctxReqID))
	fail(c, http.StatusInternalServerError, "internal_error", "Internal server error.")
}

func mapError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		fail(c, http.StatusUnauthorized, "invalid_credentials", "Invalid username or password.")
	case errors.Is(err, auth.ErrAccountDisabled):
		fail(c, http.StatusForbidden, "account_disabled", "This account is disabled.")
	case errors.Is(err, auth.ErrUsernameTaken):
		fail(c, http.StatusConflict, "username_taken", "That username is already taken.")
	case errors.Is(err, auth.ErrInvalidUsername):
		fail(c, http.StatusBadRequest, "invalid_username", err.Error())
	case errors.Is(err, auth.ErrWeakPassword):
		fail(c, http.StatusBadRequest, "weak_password", auth.ErrWeakPassword.Error())
	case errors.Is(err, auth.ErrTooManyAccounts):
		fail(c, http.StatusTooManyRequests, "too_many_accounts", "Too many accounts created from this address. Try again later.")
	case errors.Is(err, auth.ErrDailyLimit):
		fail(c, http.StatusBadRequest, "daily_limit_exceeded", "Daily spending limit exceeded.")
	case errors.Is(err, auth.ErrPerTxLimit):
		fail(c, http.StatusBadRequest, "per_tx_limit_exceeded", "Transaction limit exceeded.")
	case errors.Is(err, auth.ErrTOTPRequired):
		fail(c, http.StatusForbidden, "totp_setup_required", "Set up two-factor authentication first.")
	case errors.Is(err, auth.ErrInvalidTOTP):
		fail(c, http.StatusBadRequest, "invalid_totp", "Invalid two-factor code.")
	case errors.Is(err, auth.ErrInvalidRecovery):
		fail(c, http.StatusBadRequest, "invalid_recovery_code", "Invalid or already used recovery code.")
	case errors.Is(err, wallet.ErrWalletNotFound):
		fail(c, http.StatusNotFound, "wallet_not_found", "Wallet not found.")
	case errors.Is(err, gorm.ErrRecordNotFound):
		fail(c, http.StatusNotFound, "not_found", "Resource not found.")
	default:
		var rpcErr *rpc.Error
		if errors.As(err, &rpcErr) {
			if rpcErr.Code == -14 {
				fail(c, http.StatusBadGateway, "wallet_unlock_failed", "No se pudo desbloquear la wallet. Contactá al administrador.")
				return
			}
			fail(c, http.StatusBadGateway, "rpc_error", rpcErr.Message)
			return
		}
		internal(c, err)
	}
}

// ---- static SPA ----

func (s *Server) serveStatic(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		fail(c, http.StatusNotFound, "not_found", "Endpoint not found.")
		return
	}
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		c.Status(http.StatusNotFound)
		return
	}
	requestPath := strings.TrimPrefix(path.Clean(c.Request.URL.Path), "/")
	if requestPath == "" || requestPath == "." {
		requestPath = "index.html"
	}
	data, err := fs.ReadFile(s.static, requestPath)
	if err != nil {
		data, err = fs.ReadFile(s.static, "index.html")
		requestPath = "index.html"
	}
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "Frontend not built.")
		return
	}
	ext := strings.ToLower(filepath.Ext(requestPath))
	contentType := mime.TypeByExtension(ext)
	switch ext {
	case ".webmanifest":
		contentType = "application/manifest+json"
	case ".js":
		contentType = "text/javascript; charset=utf-8"
	case ".css":
		contentType = "text/css; charset=utf-8"
	case ".svg":
		contentType = "image/svg+xml"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if requestPath == "index.html" {
		c.Header("Cache-Control", "no-store")
	} else if strings.HasPrefix(requestPath, "assets/") {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Header("Cache-Control", "public, max-age=3600")
	}
	c.Data(http.StatusOK, contentType, data)
}

// ---- health / metrics ----

func (s *Server) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	status := "ok"
	details := gin.H{}
	if sqlDB, err := s.db.DB(); err != nil || sqlDB.PingContext(ctx) != nil {
		status = "degraded"
		details["database"] = "unavailable"
	} else {
		details["database"] = "ok"
	}
	if info, err := s.rpc.GetBlockchainInfo(ctx); err != nil {
		status = "degraded"
		details["node"] = "unavailable"
	} else {
		details["node"] = "ok"
		details["chain"] = info.Chain
		details["blocks"] = info.Blocks
		details["ibd"] = info.InitialBlockDownload
	}
	code := http.StatusOK
	if status != "ok" {
		code = http.StatusServiceUnavailable
	}
	c.JSON(code, gin.H{"status": status, "details": details})
}

func (s *Server) metrics(c *gin.Context) {
	if s.cfg.MetricsToken == "" {
		c.Status(http.StatusNotFound)
		return
	}
	header := c.GetHeader("Authorization")
	if subtle.ConstantTimeCompare([]byte(header), []byte("Bearer "+s.cfg.MetricsToken)) != 1 {
		c.Status(http.StatusNotFound)
		return
	}
	var users, wallets, audits int64
	_ = s.db.Model(&models.User{}).Count(&users).Error
	_ = s.db.Model(&models.Wallet{}).Count(&wallets).Error
	_ = s.db.Model(&models.AuditEvent{}).Count(&audits).Error
	body := fmt.Sprintf("# HELP webwallet_users Total users\n# TYPE webwallet_users gauge\nwebwallet_users %d\n"+
		"# HELP webwallet_wallets Total wallets\n# TYPE webwallet_wallets gauge\nwebwallet_wallets %d\n"+
		"# HELP webwallet_audit_events Total audit events\n# TYPE webwallet_audit_events counter\nwebwallet_audit_events %d\n",
		users, wallets, audits)
	c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(body))
}
