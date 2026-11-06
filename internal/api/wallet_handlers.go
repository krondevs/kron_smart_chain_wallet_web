package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/kron-smart-chain/webwallet/internal/amount"
	"github.com/kron-smart-chain/webwallet/internal/models"
)

func (s *Server) getWallet(c *gin.Context) {
	user := currentUser(c)
	w, err := s.wallets.GetByUser(c.Request.Context(), user.ID)
	if err != nil {
		mapError(c, err)
		return
	}
	out := gin.H{
		"address": w.Address,
		"name":    w.Name,
		"balance": "0",
		"uri":     "kron:" + w.Address,
	}
	var nodeErr string
	_ = s.wallets.Read(c.Request.Context(), user.ID, func(ctx context.Context, walletName string) error {
		balance, err := s.rpc.GetBalance(ctx, walletName)
		if err != nil {
			nodeErr = err.Error()
			return nil
		}
		out["balance"] = amount.FormatKSC(balance)
		out["balance_sat"] = balance
		if balances, err := s.rpc.GetBalances(ctx, walletName); err == nil {
			out["balance_pending"] = formatRPCAmount(balances.Mine.UntrustedPending)
			out["balance_immature"] = formatRPCAmount(balances.Mine.Immature)
			out["balance_stake"] = formatRPCAmount(balances.Mine.Stake)
		}
		if info, err := s.rpc.GetWalletInfo(ctx, walletName); err == nil {
			out["wallet_info"] = info
		}
		return nil
	})
	chain, err := s.rpc.GetBlockchainInfo(c.Request.Context())
	if err == nil {
		out["chain"] = gin.H{
			"chain":                chain.Chain,
			"blocks":               chain.Blocks,
			"bestblockhash":        chain.BestBlockHash,
			"difficulty":           chain.Difficulty,
			"initialblockdownload": chain.InitialBlockDownload,
			"verificationprogress": chain.VerificationProgress,
			"moneysupply":          chain.MoneySupply.String(),
		}
	} else {
		nodeErr = err.Error()
	}
	if dgp, err := s.rpc.GetDGPInfo(c.Request.Context()); err == nil {
		out["gas"] = gin.H{
			"min_gas_price_sat": dgp.MinGasPrice,
			"min_gas_price":     amount.FormatKSC(int64(dgp.MinGasPrice)),
			"block_gas_limit":   dgp.BlockGasLimit,
		}
	}
	var tokenCount int64
	_ = s.db.WithContext(c.Request.Context()).Model(&models.UserToken{}).Where("user_id = ?", user.ID).Count(&tokenCount).Error
	out["token_count"] = tokenCount
	out["primary_address"] = w.Address

	var primarySat int64
	if utxos, err := s.wallets.PrimaryUTXOs(c.Request.Context(), user.ID); err == nil {
		out["primary_utxo_count"] = len(utxos)
		for _, utxo := range utxos {
			if sat, err := amount.ParseKSC(utxo.Amount.String()); err == nil {
				primarySat += sat
			}
		}
		out["primary_balance"] = amount.FormatKSC(primarySat)
		out["primary_balance_sat"] = primarySat
	}
	if groups, err := s.wallets.AddressGroupings(c.Request.Context(), user.ID); err == nil {
		addresses := make([]gin.H, 0)
		seenPrimary := false
		for _, group := range groups {
			for _, entry := range group {
				if entry.Address == w.Address {
					seenPrimary = true
				}
				addresses = append(addresses, gin.H{
					"address":    entry.Address,
					"amount":     entry.Amount,
					"amount_sat": entry.AmountSat,
					"label":      entry.Label,
					"primary":    entry.Address == w.Address,
				})
			}
		}
		if !seenPrimary && primarySat > 0 {
			addresses = append(addresses, gin.H{
				"address":    w.Address,
				"amount":     amount.FormatKSC(primarySat),
				"amount_sat": primarySat,
				"primary":    true,
			})
		}
		out["addresses"] = addresses
	}
	if nodeErr != "" {
		out["node_error"] = nodeErr
	}
	respondOK(c, out)
}

func (s *Server) getAddress(c *gin.Context) {
	user := currentUser(c)
	w, err := s.wallets.GetByUser(c.Request.Context(), user.ID)
	if err != nil {
		mapError(c, err)
		return
	}
	respondOK(c, gin.H{
		"address": w.Address,
		"uri":     "kron:" + w.Address,
	})
}

func (s *Server) listTransactions(c *gin.Context) {
	user := currentUser(c)
	count := 50
	skip := 0
	if v := c.Query("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			count = n
		}
	}
	if v := c.Query("skip"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			skip = n
		}
	}
	var txs []map[string]any
	err := s.wallets.Read(c.Request.Context(), user.ID, func(ctx context.Context, walletName string) error {
		items, err := s.rpc.ListTransactions(ctx, walletName, count, skip)
		if err != nil {
			return err
		}
		txs = items
		return nil
	})
	if err != nil {
		mapError(c, err)
		return
	}
	out := make([]gin.H, 0, len(txs))
	for _, tx := range txs {
		out = append(out, normalizeTransaction(tx))
	}
	respondOK(c, gin.H{"transactions": out, "count": len(out), "skip": skip})
}

func (s *Server) getTransaction(c *gin.Context) {
	user := currentUser(c)
	txid := strings.TrimSpace(c.Param("txid"))
	if len(txid) != 64 {
		fail(c, http.StatusBadRequest, "invalid_txid", "Invalid transaction id.")
		return
	}
	var tx map[string]any
	err := s.wallets.Read(c.Request.Context(), user.ID, func(ctx context.Context, walletName string) error {
		item, err := s.rpc.GetTransaction(ctx, walletName, txid)
		if err != nil {
			return err
		}
		tx = item
		return nil
	})
	if err != nil {
		mapError(c, err)
		return
	}
	respondOK(c, normalizeTransaction(tx))
}

func (s *Server) feeEstimate(c *gin.Context) {
	blocks := 6
	if v := c.Query("blocks"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 1008 {
			blocks = n
		}
	}
	feerate, err := s.rpc.EstimateSmartFee(c.Request.Context(), blocks)
	if err != nil {
		// Fall back to the node's minimum relay fee rather than failing the UI.
		respondOK(c, gin.H{"blocks": blocks, "feerate_ksc_per_kvb": "0", "estimated_fee": "0", "source": "unavailable"})
		return
	}
	// Typical 1 input / 2 outputs P2PKH transaction is ~226 vbytes.
	estimatedSat := int64(feerate * 226 / 1000 * 100_000_000)
	if estimatedSat < 1000 {
		estimatedSat = 1000
	}
	respondOK(c, gin.H{
		"blocks":              blocks,
		"feerate_ksc_per_kvb": feerate,
		"estimated_fee":       amount.FormatKSC(estimatedSat),
		"estimated_fee_sat":   estimatedSat,
		"source":              "estimatesmartfee",
	})
}

type sendRequest struct {
	Address        string `json:"address"`
	Amount         string `json:"amount"`
	Comment        string `json:"comment"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (s *Server) send(c *gin.Context) {
	user := currentUser(c)
	var req sendRequest
	if !bindJSON(c, &req) {
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		key = strings.TrimSpace(req.IdempotencyKey)
	}
	if key == "" || len(key) > 80 {
		fail(c, http.StatusBadRequest, "idempotency_key_required", "Provide an Idempotency-Key header.")
		return
	}
	address := strings.TrimSpace(req.Address)
	if address == "" {
		fail(c, http.StatusBadRequest, "invalid_address", "Destination address is required.")
		return
	}
	sat, err := amount.ParseKSC(req.Amount)
	if err != nil || sat <= 0 {
		fail(c, http.StatusBadRequest, "invalid_amount", "Amount must be a positive KSC value with up to 8 decimals.")
		return
	}

	validation, err := s.rpc.ValidateAddress(c.Request.Context(), address)
	if err != nil {
		mapError(c, err)
		return
	}
	if valid, _ := validation["isvalid"].(bool); !valid {
		fail(c, http.StatusBadRequest, "invalid_address", "Destination is not a valid Kron address.")
		return
	}

	var existing models.TxIntent
	err = s.db.WithContext(c.Request.Context()).
		Where("user_id = ? AND idempotency_key = ?", user.ID, key).First(&existing).Error
	if err == nil {
		switch existing.Status {
		case "success":
			respondOK(c, gin.H{"txid": existing.TxID, "status": existing.Status, "replayed": true})
		case "pending":
			fail(c, http.StatusConflict, "in_progress", "An operation with this idempotency key is already in progress.")
		default:
			fail(c, http.StatusConflict, "previous_failed", "This idempotency key was already used by a failed operation. Use a new key.")
		}
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		internal(c, err)
		return
	}

	if err := s.auth.ReserveSpend(c.Request.Context(), user, sat); err != nil {
		mapError(c, err)
		return
	}
	intent := &models.TxIntent{
		UserID:         user.ID,
		IdempotencyKey: key,
		Kind:           "ksc_send",
		Status:         "pending",
		Payload:        `{"address":"` + address + `","amount_sat":` + strconv.FormatInt(sat, 10) + `}`,
	}
	if err := s.db.WithContext(c.Request.Context()).Create(intent).Error; err != nil {
		s.auth.RefundSpend(c.Request.Context(), user.ID, sat)
		internal(c, err)
		return
	}

	var txid string
	txid, err = s.wallets.SendGeneral(c.Request.Context(), user.ID, address, sat)
	if err != nil {
		s.auth.RefundSpend(c.Request.Context(), user.ID, sat)
		_ = s.db.WithContext(c.Request.Context()).Model(intent).Updates(map[string]any{
			"status": "failed",
			"error":  err.Error(),
		}).Error
		audit(c, &user.ID, "wallet.send_failed", "address="+address+" amount_sat="+strconv.FormatInt(sat, 10)+" error="+err.Error())
		mapError(c, err)
		return
	}
	_ = s.db.WithContext(c.Request.Context()).Model(intent).Updates(map[string]any{
		"status": "success",
		"tx_id":  txid,
	}).Error
	audit(c, &user.ID, "wallet.send", "txid="+txid+" address="+address+" amount_sat="+strconv.FormatInt(sat, 10))
	respondOK(c, gin.H{"txid": txid, "status": "success"})
}

type prepareGasRequest struct {
	Address string `json:"address"`
}

const gasTopUpSat int64 = 20_000_000 // 0.2 KSC

func (s *Server) prepareGas(c *gin.Context) {
	user := currentUser(c)
	var req prepareGasRequest
	if !bindJSON(c, &req) {
		return
	}
	target, ok := s.walletAddress(c, user.ID)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Address) != "" {
		if !s.validateAddress(c, req.Address) {
			return
		}
		target = strings.TrimSpace(req.Address)
	}
	owned, err := s.walletOwnsAddress(c, user.ID, target)
	if err != nil {
		mapError(c, err)
		return
	}
	if !owned {
		fail(c, http.StatusBadRequest, "invalid_address", "La dirección de gas no pertenece a esta wallet.")
		return
	}
	intent, ok := s.beginIntent(c, user.ID, "gas_prepare", marshalPayload(gin.H{"address": target}))
	if !ok {
		return
	}
	txid, err := s.wallets.SendGeneral(c.Request.Context(), user.ID, target, gasTopUpSat)
	s.completeIntent(c, intent, txid, err)
	if err != nil {
		audit(c, &user.ID, "wallet.prepare_gas_failed", "address="+target+" error="+err.Error())
		mapError(c, err)
		return
	}
	audit(c, &user.ID, "wallet.prepare_gas", "txid="+txid+" address="+target)
	respondOK(c, gin.H{"txid": txid, "address": target, "status": "success"})
}

type consolidateRequest struct {
	Address string `json:"address"`
}

func (s *Server) consolidate(c *gin.Context) {
	user := currentUser(c)
	var req consolidateRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body: "+err.Error())
			return
		}
	}
	target, ok := s.walletAddress(c, user.ID)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Address) != "" {
		if !s.validateAddress(c, req.Address) {
			return
		}
		target = strings.TrimSpace(req.Address)
	}
	owned, err := s.walletOwnsAddress(c, user.ID, target)
	if err != nil {
		mapError(c, err)
		return
	}
	if !owned {
		fail(c, http.StatusBadRequest, "invalid_address", "La dirección de destino no pertenece a esta wallet.")
		return
	}
	intent, ok := s.beginIntent(c, user.ID, "ksc_consolidate", marshalPayload(gin.H{"address": target}))
	if !ok {
		return
	}
	txid, err := s.wallets.Consolidate(c.Request.Context(), user.ID, target)
	s.completeIntent(c, intent, txid, err)
	if err != nil {
		audit(c, &user.ID, "wallet.consolidate_failed", "address="+target+" error="+err.Error())
		mapError(c, err)
		return
	}
	audit(c, &user.ID, "wallet.consolidate", "txid="+txid+" address="+target)
	respondOK(c, gin.H{"txid": txid, "address": target, "status": "success"})
}

func normalizeTransaction(tx map[string]any) gin.H {
	out := gin.H{}
	for k, v := range tx {
		out[k] = v
	}
	if n, ok := tx["amount"].(json.Number); ok {
		if sat, err := amount.ParseKSC(n.String()); err == nil {
			out["amount_sat"] = sat
			out["amount"] = amount.FormatKSC(sat)
		}
	}
	if n, ok := tx["fee"].(json.Number); ok {
		if sat, err := amount.ParseKSC(n.String()); err == nil {
			out["fee_sat"] = sat
			out["fee"] = amount.FormatKSC(sat)
		}
	}
	if t, ok := tx["time"].(json.Number); ok {
		if sec, err := t.Int64(); err == nil {
			out["time_iso"] = time.Unix(sec, 0).UTC().Format(time.RFC3339)
		}
	}
	return out
}

func (s *Server) walletOwnsAddress(c *gin.Context, userID uint, address string) (bool, error) {
	owned := false
	err := s.wallets.Read(c.Request.Context(), userID, func(ctx context.Context, walletName string) error {
		info, err := s.rpc.GetAddressInfo(ctx, walletName, address)
		if err != nil {
			return err
		}
		owned = info.IsMine
		return nil
	})
	return owned, err
}

func formatRPCAmount(n json.Number) string {
	if n.String() == "" {
		return "0"
	}
	sat, err := amount.ParseKSC(n.String())
	if err != nil {
		return n.String()
	}
	return amount.FormatKSC(sat)
}
