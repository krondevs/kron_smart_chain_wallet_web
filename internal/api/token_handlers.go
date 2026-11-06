package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/kron-smart-chain/webwallet/internal/amount"
	"github.com/kron-smart-chain/webwallet/internal/models"
)

func tokenJSON(token *models.Token, balance string) gin.H {
	return gin.H{
		"contract":     token.Contract,
		"name":         token.Name,
		"symbol":       token.Symbol,
		"decimals":     token.Decimals,
		"total_supply": token.TotalSupply,
		"verified":     token.Verified,
		"balance":      balance,
	}
}

func (s *Server) allWalletAddresses(ctx context.Context, userID uint) ([]string, error) {
	return s.wallets.Addresses(ctx, userID)
}

func (s *Server) tokenBalanceAll(ctx context.Context, userID uint, token *models.Token) (string, []gin.H, error) {
	addresses, err := s.allWalletAddresses(ctx, userID)
	if err != nil {
		return "0", nil, err
	}
	sum := big.NewInt(0)
	perAddress := make([]gin.H, 0)
	succeeded := false
	var lastErr error
	for _, address := range addresses {
		balance, err := s.rpc.KRC20BalanceOf(ctx, token.Contract, address)
		if err != nil {
			lastErr = err
			continue
		}
		raw, err := amount.ParseTokenAmount(balance, token.Decimals)
		if err != nil {
			lastErr = err
			continue
		}
		n, ok := new(big.Int).SetString(raw, 10)
		if !ok {
			continue
		}
		succeeded = true
		sum.Add(sum, n)
		if n.Sign() > 0 {
			perAddress = append(perAddress, gin.H{"address": address, "balance": balance})
		}
	}
	if !succeeded && lastErr != nil {
		return "0", perAddress, lastErr
	}
	return amount.FormatTokenAmount(sum.String(), token.Decimals), perAddress, nil
}

func (s *Server) requireGas(c *gin.Context, user *models.User, address string) bool {
	utxos, err := s.wallets.UTXOsForAddress(c.Request.Context(), user.ID, address)
	if err != nil {
		mapError(c, err)
		return false
	}
	if len(utxos) == 0 {
		fail(c, http.StatusBadRequest, "gas_not_ready", fmt.Sprintf("La dirección %s no tiene KSC para pagar gas. Prepará gas para esa dirección o consolidá tu KSC y reintentá.", address))
		return false
	}
	return true
}

func (s *Server) listTokens(c *gin.Context) {
	user := currentUser(c)
	var links []models.UserToken
	if err := s.db.WithContext(c.Request.Context()).Where("user_id = ?", user.ID).Order("id ASC").Find(&links).Error; err != nil {
		internal(c, err)
		return
	}
	out := make([]gin.H, 0, len(links))
	for _, link := range links {
		var token models.Token
		if err := s.db.WithContext(c.Request.Context()).First(&token, link.TokenID).Error; err != nil {
			continue
		}
		balance, balances, err := s.tokenBalanceAll(c.Request.Context(), user.ID, &token)
		if err != nil {
			balance = "0"
		}
		item := tokenJSON(&token, balance)
		item["balances"] = balances
		if err != nil {
			item["balance_error"] = err.Error()
		}
		out = append(out, item)
	}
	respondOK(c, gin.H{"tokens": out})
}

type addTokenRequest struct {
	Contract string `json:"contract"`
}

func (s *Server) addToken(c *gin.Context) {
	user := currentUser(c)
	var req addTokenRequest
	if !bindJSON(c, &req) {
		return
	}
	contract, err := normalizeContractAddress(req.Contract)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_contract", err.Error())
		return
	}

	name, err := s.rpc.KRC20Name(c.Request.Context(), contract)
	if err != nil {
		slog.Warn("token metadata lookup failed", "contract", contract, "field", "name", "error", err)
		fail(c, http.StatusBadRequest, "token_lookup_failed", fmt.Sprintf("No se pudo leer name() del contrato: %v", err))
		return
	}
	if strings.TrimSpace(name) == "" {
		fail(c, http.StatusBadRequest, "not_krc20", "El contrato no devolvió name(). No parece un token KRC20/QRC20.")
		return
	}
	symbol, err := s.rpc.KRC20Symbol(c.Request.Context(), contract)
	if err != nil {
		slog.Warn("token metadata lookup failed", "contract", contract, "field", "symbol", "error", err)
		fail(c, http.StatusBadRequest, "token_lookup_failed", fmt.Sprintf("No se pudo leer symbol() del contrato: %v", err))
		return
	}
	if strings.TrimSpace(symbol) == "" {
		fail(c, http.StatusBadRequest, "not_krc20", "El contrato no devolvió symbol().")
		return
	}
	decimals, err := s.rpc.KRC20Decimals(c.Request.Context(), contract)
	if err != nil {
		slog.Warn("token metadata lookup failed", "contract", contract, "field", "decimals", "error", err)
		fail(c, http.StatusBadRequest, "token_lookup_failed", fmt.Sprintf("No se pudo leer decimals() del contrato: %v", err))
		return
	}
	if decimals < 0 || decimals > 36 {
		fail(c, http.StatusBadRequest, "not_krc20", "El contrato devolvió decimales inválidos.")
		return
	}
	supply, err := s.rpc.KRC20TotalSupply(c.Request.Context(), contract)
	if err != nil {
		slog.Warn("token metadata lookup failed", "contract", contract, "field", "totalSupply", "error", err)
		fail(c, http.StatusBadRequest, "token_lookup_failed", fmt.Sprintf("No se pudo leer totalSupply() del contrato: %v", err))
		return
	}

	var token models.Token
	err = s.db.WithContext(c.Request.Context()).Where("contract = ?", contract).First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		token = models.Token{
			Contract:    contract,
			Name:        truncateString(name, 120),
			Symbol:      truncateString(symbol, 32),
			Decimals:    decimals,
			TotalSupply: truncateString(supply, 120),
		}
		if err := s.db.WithContext(c.Request.Context()).Create(&token).Error; err != nil {
			internal(c, err)
			return
		}
	} else if err != nil {
		internal(c, err)
		return
	} else {
		// Refresh cached metadata from the chain.
		_ = s.db.WithContext(c.Request.Context()).Model(&token).Updates(map[string]any{
			"name":         truncateString(name, 120),
			"symbol":       truncateString(symbol, 32),
			"decimals":     decimals,
			"total_supply": truncateString(supply, 120),
		}).Error
	}

	link := models.UserToken{UserID: user.ID, TokenID: token.ID}
	if err := s.db.WithContext(c.Request.Context()).Where("user_id = ? AND token_id = ?", user.ID, token.ID).
		FirstOrCreate(&link).Error; err != nil {
		internal(c, err)
		return
	}
	audit(c, &user.ID, "token.added", "contract="+contract)
	respondOK(c, tokenJSON(&token, "0"))
}

func (s *Server) removeToken(c *gin.Context) {
	user := currentUser(c)
	contract, err := normalizeContractAddress(c.Param("contract"))
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_contract", err.Error())
		return
	}
	var token models.Token
	if err := s.db.WithContext(c.Request.Context()).Where("contract = ?", contract).First(&token).Error; err != nil {
		mapError(c, err)
		return
	}
	if err := s.db.WithContext(c.Request.Context()).Where("user_id = ? AND token_id = ?", user.ID, token.ID).
		Delete(&models.UserToken{}).Error; err != nil {
		internal(c, err)
		return
	}
	audit(c, &user.ID, "token.removed", "contract="+contract)
	respondOK(c, gin.H{"removed": true})
}

func (s *Server) getToken(c *gin.Context) {
	user := currentUser(c)
	token, ok := s.tokenForUser(c, user.ID)
	if !ok {
		return
	}
	primary, ok := s.walletAddress(c, user.ID)
	if !ok {
		return
	}
	balance, balances, err := s.tokenBalanceAll(c.Request.Context(), user.ID, token)
	if err != nil {
		balance = "0"
	}
	history, _ := s.rpc.KRC20ListTransactions(c.Request.Context(), token.Contract, primary, 0, 1)
	out := tokenJSON(token, balance)
	out["balances"] = balances
	out["transactions"] = history
	respondOK(c, out)
}

func (s *Server) tokenTransactions(c *gin.Context) {
	user := currentUser(c)
	token, ok := s.tokenForUser(c, user.ID)
	if !ok {
		return
	}
	address, ok := s.walletAddress(c, user.ID)
	if !ok {
		return
	}
	fromBlock := int64(0)
	minconf := int64(1)
	if v := c.Query("fromblock"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			fromBlock = n
		}
	}
	if v := c.Query("minconf"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			minconf = n
		}
	}
	txs, err := s.rpc.KRC20ListTransactions(c.Request.Context(), token.Contract, address, fromBlock, minconf)
	if err != nil {
		mapError(c, err)
		return
	}
	respondOK(c, gin.H{"transactions": txs})
}

func (s *Server) tokenAllowance(c *gin.Context) {
	user := currentUser(c)
	token, ok := s.tokenForUser(c, user.ID)
	if !ok {
		return
	}
	address, ok := s.walletAddress(c, user.ID)
	if !ok {
		return
	}
	spender := strings.TrimSpace(c.Query("spender"))
	if !s.validateAddress(c, spender) {
		return
	}
	allowance, err := s.rpc.KRC20Allowance(c.Request.Context(), token.Contract, address, spender)
	if err != nil {
		mapError(c, err)
		return
	}
	respondOK(c, gin.H{"allowance": allowance, "spender": spender, "owner": address})
}

func (s *Server) tokenForUser(c *gin.Context, userID uint) (*models.Token, bool) {
	contract, err := normalizeContractAddress(c.Param("contract"))
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_contract", err.Error())
		return nil, false
	}
	var token models.Token
	if err := s.db.WithContext(c.Request.Context()).Where("contract = ?", contract).First(&token).Error; err != nil {
		mapError(c, err)
		return nil, false
	}
	var count int64
	if err := s.db.WithContext(c.Request.Context()).Model(&models.UserToken{}).
		Where("user_id = ? AND token_id = ?", userID, token.ID).Count(&count).Error; err != nil {
		internal(c, err)
		return nil, false
	}
	if count == 0 {
		fail(c, http.StatusNotFound, "token_not_added", "Add this token to your portfolio first.")
		return nil, false
	}
	return &token, true
}

type tokenTransferRequest struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Amount   string `json:"amount"`
	GasLimit string `json:"gaslimit"`
	GasPrice string `json:"gasprice"`
}

func (s *Server) tokenTransfer(c *gin.Context) {
	user := currentUser(c)
	token, ok := s.tokenForUser(c, user.ID)
	if !ok {
		return
	}
	var req tokenTransferRequest
	if !bindJSON(c, &req) {
		return
	}
	if !s.validateAddress(c, req.To) {
		return
	}
	raw, err := amount.ParseTokenAmount(req.Amount, token.Decimals)
	if err != nil || raw == "0" || strings.HasPrefix(raw, "-") {
		fail(c, http.StatusBadRequest, "invalid_amount", "Amount must be a positive token value.")
		return
	}
	// The node RPC expects the human decimal string, not the raw integer.
	tokenAmount := amount.FormatTokenAmount(raw, token.Decimals)
	gasLimit, gasPrice, err := s.gasParams(c, req.GasLimit, req.GasPrice)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_gas", err.Error())
		return
	}
	address, ok := s.walletAddress(c, user.ID)
	if !ok {
		return
	}
	owner := address
	if strings.TrimSpace(req.From) != "" {
		if !s.validateAddress(c, req.From) {
			return
		}
		owner = strings.TrimSpace(req.From)
	}
	if !s.requireGas(c, user, owner) {
		return
	}
	intent, ok := s.beginIntent(c, user.ID, "krc20_transfer", marshalPayload(req))
	if !ok {
		return
	}
	var txid string
	err = s.wallets.Do(c.Request.Context(), user.ID, func(ctx context.Context, walletName string) error {
		id, err := s.rpc.KRC20Transfer(ctx, walletName, token.Contract, owner, req.To, tokenAmount, gasLimit, gasPrice)
		if err != nil {
			return err
		}
		txid = id
		return nil
	})
	s.completeIntent(c, intent, txid, err)
	if err != nil {
		audit(c, &user.ID, "token.transfer_failed", "contract="+token.Contract+" error="+err.Error())
		mapError(c, err)
		return
	}
	audit(c, &user.ID, "token.transfer", "txid="+txid+" contract="+token.Contract+" to="+req.To)
	respondOK(c, gin.H{"txid": txid, "status": "success"})
}

type tokenApproveRequest struct {
	From     string `json:"from"`
	Spender  string `json:"spender"`
	Amount   string `json:"amount"`
	GasLimit string `json:"gaslimit"`
	GasPrice string `json:"gasprice"`
}

func (s *Server) tokenApprove(c *gin.Context) {
	user := currentUser(c)
	token, ok := s.tokenForUser(c, user.ID)
	if !ok {
		return
	}
	var req tokenApproveRequest
	if !bindJSON(c, &req) {
		return
	}
	if !s.validateAddress(c, req.Spender) {
		return
	}
	raw, err := amount.ParseTokenAmount(req.Amount, token.Decimals)
	if err != nil || strings.HasPrefix(raw, "-") {
		fail(c, http.StatusBadRequest, "invalid_amount", "Amount must be a non-negative token value.")
		return
	}
	tokenAmount := amount.FormatTokenAmount(raw, token.Decimals)
	gasLimit, gasPrice, err := s.gasParams(c, req.GasLimit, req.GasPrice)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_gas", err.Error())
		return
	}
	address, ok := s.walletAddress(c, user.ID)
	if !ok {
		return
	}
	owner := address
	if strings.TrimSpace(req.From) != "" {
		if !s.validateAddress(c, req.From) {
			return
		}
		owner = strings.TrimSpace(req.From)
	}
	if !s.requireGas(c, user, owner) {
		return
	}
	intent, ok := s.beginIntent(c, user.ID, "krc20_approve", marshalPayload(req))
	if !ok {
		return
	}
	var txid string
	err = s.wallets.Do(c.Request.Context(), user.ID, func(ctx context.Context, walletName string) error {
		id, err := s.rpc.KRC20Approve(ctx, walletName, token.Contract, owner, req.Spender, tokenAmount, gasLimit, gasPrice)
		if err != nil {
			return err
		}
		txid = id
		return nil
	})
	s.completeIntent(c, intent, txid, err)
	if err != nil {
		audit(c, &user.ID, "token.approve_failed", "contract="+token.Contract+" error="+err.Error())
		mapError(c, err)
		return
	}
	audit(c, &user.ID, "token.approve", "txid="+txid+" contract="+token.Contract+" spender="+req.Spender)
	respondOK(c, gin.H{"txid": txid, "status": "success"})
}

type tokenTransferFromRequest struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Amount   string `json:"amount"`
	GasLimit string `json:"gaslimit"`
	GasPrice string `json:"gasprice"`
}

func (s *Server) tokenTransferFrom(c *gin.Context) {
	user := currentUser(c)
	token, ok := s.tokenForUser(c, user.ID)
	if !ok {
		return
	}
	var req tokenTransferFromRequest
	if !bindJSON(c, &req) {
		return
	}
	if !s.validateAddress(c, req.From) || !s.validateAddress(c, req.To) {
		return
	}
	raw, err := amount.ParseTokenAmount(req.Amount, token.Decimals)
	if err != nil || raw == "0" || strings.HasPrefix(raw, "-") {
		fail(c, http.StatusBadRequest, "invalid_amount", "Amount must be a positive token value.")
		return
	}
	// The node RPC expects the human decimal string, not the raw integer.
	tokenAmount := amount.FormatTokenAmount(raw, token.Decimals)
	gasLimit, gasPrice, err := s.gasParams(c, req.GasLimit, req.GasPrice)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_gas", err.Error())
		return
	}
	address, ok := s.walletAddress(c, user.ID)
	if !ok {
		return
	}
	if !s.requireGas(c, user, address) {
		return
	}
	intent, ok := s.beginIntent(c, user.ID, "krc20_transfer_from", marshalPayload(req))
	if !ok {
		return
	}
	var txid string
	err = s.wallets.Do(c.Request.Context(), user.ID, func(ctx context.Context, walletName string) error {
		id, err := s.rpc.KRC20TransferFrom(ctx, walletName, token.Contract, req.From, address, req.To, tokenAmount, gasLimit, gasPrice)
		if err != nil {
			return err
		}
		txid = id
		return nil
	})
	s.completeIntent(c, intent, txid, err)
	if err != nil {
		audit(c, &user.ID, "token.transfer_from_failed", "contract="+token.Contract+" error="+err.Error())
		mapError(c, err)
		return
	}
	audit(c, &user.ID, "token.transfer_from", "txid="+txid+" contract="+token.Contract+" from="+req.From+" to="+req.To)
	respondOK(c, gin.H{"txid": txid, "status": "success"})
}

type tokenBurnRequest struct {
	From     string `json:"from"`
	Amount   string `json:"amount"`
	GasLimit string `json:"gaslimit"`
	GasPrice string `json:"gasprice"`
}

func (s *Server) tokenBurn(c *gin.Context) {
	user := currentUser(c)
	token, ok := s.tokenForUser(c, user.ID)
	if !ok {
		return
	}
	var req tokenBurnRequest
	if !bindJSON(c, &req) {
		return
	}
	raw, err := amount.ParseTokenAmount(req.Amount, token.Decimals)
	if err != nil || raw == "0" || strings.HasPrefix(raw, "-") {
		fail(c, http.StatusBadRequest, "invalid_amount", "Amount must be a positive token value.")
		return
	}
	// The node RPC expects the human decimal string, not the raw integer.
	tokenAmount := amount.FormatTokenAmount(raw, token.Decimals)
	gasLimit, gasPrice, err := s.gasParams(c, req.GasLimit, req.GasPrice)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_gas", err.Error())
		return
	}
	address, ok := s.walletAddress(c, user.ID)
	if !ok {
		return
	}
	owner := address
	if strings.TrimSpace(req.From) != "" {
		if !s.validateAddress(c, req.From) {
			return
		}
		owner = strings.TrimSpace(req.From)
	}
	if !s.requireGas(c, user, owner) {
		return
	}
	intent, ok := s.beginIntent(c, user.ID, "krc20_burn", marshalPayload(req))
	if !ok {
		return
	}
	var txid string
	err = s.wallets.Do(c.Request.Context(), user.ID, func(ctx context.Context, walletName string) error {
		id, err := s.rpc.KRC20Burn(ctx, walletName, token.Contract, owner, tokenAmount, gasLimit, gasPrice)
		if err != nil {
			return err
		}
		txid = id
		return nil
	})
	s.completeIntent(c, intent, txid, err)
	if err != nil {
		audit(c, &user.ID, "token.burn_failed", "contract="+token.Contract+" error="+err.Error())
		mapError(c, err)
		return
	}
	audit(c, &user.ID, "token.burn", "txid="+txid+" contract="+token.Contract)
	respondOK(c, gin.H{"txid": txid, "status": "success"})
}

type tokenBurnFromRequest struct {
	From     string `json:"from"`
	Amount   string `json:"amount"`
	GasLimit string `json:"gaslimit"`
	GasPrice string `json:"gasprice"`
}

func (s *Server) tokenBurnFrom(c *gin.Context) {
	user := currentUser(c)
	token, ok := s.tokenForUser(c, user.ID)
	if !ok {
		return
	}
	var req tokenBurnFromRequest
	if !bindJSON(c, &req) {
		return
	}
	if !s.validateAddress(c, req.From) {
		return
	}
	raw, err := amount.ParseTokenAmount(req.Amount, token.Decimals)
	if err != nil || raw == "0" || strings.HasPrefix(raw, "-") {
		fail(c, http.StatusBadRequest, "invalid_amount", "Amount must be a positive token value.")
		return
	}
	// The node RPC expects the human decimal string, not the raw integer.
	tokenAmount := amount.FormatTokenAmount(raw, token.Decimals)
	gasLimit, gasPrice, err := s.gasParams(c, req.GasLimit, req.GasPrice)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_gas", err.Error())
		return
	}
	address, ok := s.walletAddress(c, user.ID)
	if !ok {
		return
	}
	if !s.requireGas(c, user, address) {
		return
	}
	intent, ok := s.beginIntent(c, user.ID, "krc20_burn_from", marshalPayload(req))
	if !ok {
		return
	}
	var txid string
	err = s.wallets.Do(c.Request.Context(), user.ID, func(ctx context.Context, walletName string) error {
		id, err := s.rpc.KRC20BurnFrom(ctx, walletName, token.Contract, req.From, address, tokenAmount, gasLimit, gasPrice)
		if err != nil {
			return err
		}
		txid = id
		return nil
	})
	s.completeIntent(c, intent, txid, err)
	if err != nil {
		audit(c, &user.ID, "token.burn_from_failed", "contract="+token.Contract+" error="+err.Error())
		mapError(c, err)
		return
	}
	audit(c, &user.ID, "token.burn_from", "txid="+txid+" contract="+token.Contract+" from="+req.From)
	respondOK(c, gin.H{"txid": txid, "status": "success"})
}
