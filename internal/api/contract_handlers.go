package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/kron-smart-chain/webwallet/internal/amount"
	"github.com/kron-smart-chain/webwallet/internal/evm"
	"github.com/kron-smart-chain/webwallet/internal/models"
	"github.com/kron-smart-chain/webwallet/internal/rpc"
)

func (s *Server) listContracts(c *gin.Context) {
	user := currentUser(c)
	var contracts []models.Contract
	if err := s.db.WithContext(c.Request.Context()).Where("user_id = ?", user.ID).Order("created_at DESC").Find(&contracts).Error; err != nil {
		internal(c, err)
		return
	}
	out := make([]gin.H, 0, len(contracts))
	for i := range contracts {
		item := gin.H{
			"id":         contracts[i].ID,
			"address":    contracts[i].Address,
			"name":       contracts[i].Name,
			"deployed":   contracts[i].Deployed,
			"has_abi":    contracts[i].ABI != "",
			"created_at": contracts[i].CreatedAt,
		}
		if contracts[i].ABI != "" {
			if info, err := evm.Inspect(contracts[i].ABI); err == nil {
				item["methods"] = info.Methods
				item["events"] = info.Events
			}
		}
		out = append(out, item)
	}
	respondOK(c, gin.H{"contracts": out})
}

type contractRequest struct {
	Address  string `json:"address"`
	Name     string `json:"name"`
	ABI      string `json:"abi"`
	Bytecode string `json:"bytecode"`
}

func (s *Server) addContract(c *gin.Context) {
	user := currentUser(c)
	var req contractRequest
	if !bindJSON(c, &req) {
		return
	}
	address, err := normalizeContractAddress(req.Address)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_contract", err.Error())
		return
	}
	info, err := evm.Inspect(req.ABI)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_abi", err.Error())
		return
	}
	code, codeErr := s.rpc.GetContractCode(c.Request.Context(), address, -1)
	hasCode := codeErr == nil && strings.TrimSpace(code) != ""

	var contract models.Contract
	err = s.db.WithContext(c.Request.Context()).
		Where("user_id = ? AND address = ?", user.ID, address).First(&contract).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		contract = models.Contract{
			UserID:   user.ID,
			Address:  address,
			Name:     truncateString(req.Name, 120),
			ABI:      req.ABI,
			Bytecode: truncateString(req.Bytecode, 200_000),
			Deployed: hasCode,
		}
		if err := s.db.WithContext(c.Request.Context()).Create(&contract).Error; err != nil {
			internal(c, err)
			return
		}
	} else if err != nil {
		internal(c, err)
		return
	} else {
		if err := s.db.WithContext(c.Request.Context()).Model(&contract).Updates(map[string]any{
			"name":     truncateString(req.Name, 120),
			"abi":      req.ABI,
			"bytecode": truncateString(req.Bytecode, 200_000),
			"deployed": hasCode,
		}).Error; err != nil {
			internal(c, err)
			return
		}
	}
	audit(c, &user.ID, "contract.imported", "address="+address)
	respondOK(c, gin.H{
		"address":  address,
		"name":     contract.Name,
		"has_code": hasCode,
		"abi":      info,
	})
}

func (s *Server) getContract(c *gin.Context) {
	user := currentUser(c)
	address, err := normalizeContractAddress(c.Param("address"))
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_contract", err.Error())
		return
	}
	var contract models.Contract
	err = s.db.WithContext(c.Request.Context()).
		Where("user_id = ? AND address = ?", user.ID, address).First(&contract).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// The contract may be deployed on-chain but not yet imported in this
		// account. Return its address so the UI can show/copy it and offer an
		// ABI import instead of a dead 404.
		code, codeErr := s.rpc.GetContractCode(c.Request.Context(), address, -1)
		if codeErr != nil {
			var rpcErr *rpc.Error
			if errors.As(codeErr, &rpcErr) && (rpcErr.Code == -5 || rpcErr.Code == -8) {
				fail(c, http.StatusNotFound, "contract_not_found", "No hay un contrato desplegado en esa dirección.")
				return
			}
			mapError(c, codeErr)
			return
		}
		if strings.TrimSpace(code) == "" {
			fail(c, http.StatusNotFound, "contract_not_found", "No hay un contrato desplegado en esa dirección.")
			return
		}
		respondOK(c, gin.H{
			"address":    address,
			"name":       "",
			"abi":        "",
			"bytecode":   "",
			"deployed":   true,
			"imported":   false,
			"methods":    []any{},
			"events":     []any{},
			"created_at": nil,
		})
		return
	}
	if err != nil {
		internal(c, err)
		return
	}
	out := gin.H{
		"address":    contract.Address,
		"name":       contract.Name,
		"abi":        contract.ABI,
		"bytecode":   contract.Bytecode,
		"deployed":   contract.Deployed,
		"imported":   true,
		"has_abi":    contract.ABI != "",
		"created_at": contract.CreatedAt,
	}
	if info, err := evm.Inspect(contract.ABI); err == nil {
		out["methods"] = info.Methods
		out["events"] = info.Events
		out["constructor"] = info.Constructor
	}
	respondOK(c, out)
}

func (s *Server) saveContract(ctx context.Context, contract *models.Contract) error {
	var existing models.Contract
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND address = ?", contract.UserID, contract.Address).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.WithContext(ctx).Create(contract).Error
	}
	if err != nil {
		return err
	}
	contract.ID = existing.ID
	return s.db.WithContext(ctx).Model(&existing).Updates(map[string]any{
		"name":     contract.Name,
		"abi":      contract.ABI,
		"bytecode": contract.Bytecode,
		"deployed": contract.Deployed,
	}).Error
}

func (s *Server) deleteContract(c *gin.Context) {
	user := currentUser(c)
	contract, ok := s.contractForUser(c, user.ID)
	if !ok {
		return
	}
	if err := s.db.WithContext(c.Request.Context()).Delete(contract).Error; err != nil {
		internal(c, err)
		return
	}
	audit(c, &user.ID, "contract.removed", "address="+contract.Address)
	respondOK(c, gin.H{"removed": true})
}

type contractCallRequest struct {
	Address  string            `json:"address"`
	ABI      string            `json:"abi"`
	Method   string            `json:"method"`
	Args     []json.RawMessage `json:"args"`
	Data     string            `json:"data"`
	GasLimit string            `json:"gaslimit"`
	Amount   string            `json:"amount"`
	Block    string            `json:"block"`
}

func (s *Server) callContract(c *gin.Context) {
	user := currentUser(c)
	var req contractCallRequest
	if !bindJSON(c, &req) {
		return
	}
	address, abiJSON, data, ok := s.prepareContractCall(c, user.ID, req.Address, req.ABI, req.Method, req.Args, req.Data)
	if !ok {
		return
	}
	gasLimit, _, err := s.gasParams(c, req.GasLimit, "")
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_gas", err.Error())
		return
	}
	amountSat := int64(0)
	if strings.TrimSpace(req.Amount) != "" {
		amountSat, err = amount.ParseKSC(req.Amount)
		if err != nil || amountSat < 0 {
			fail(c, http.StatusBadRequest, "invalid_amount", "Invalid KSC amount.")
			return
		}
	}
	blockNum := int64(-1)
	if strings.TrimSpace(req.Block) != "" {
		if n, err := strconv.ParseInt(req.Block, 10, 64); err == nil {
			blockNum = n
		}
	}
	sender, ok := s.walletAddress(c, user.ID)
	if !ok {
		return
	}
	result, err := s.rpc.CallContract(c.Request.Context(), address, data, sender, gasLimit, amountSat, blockNum)
	if err != nil {
		mapError(c, err)
		return
	}
	out := gin.H{
		"address":          address,
		"data":             data,
		"gas_used":         result.ExecutionResult.GasUsed,
		"excepted":         result.ExecutionResult.Excepted,
		"excepted_message": result.ExecutionResult.ExceptedMessage,
		"output_raw":       result.ExecutionResult.Output,
		"receipt":          result.TransactionReceipt,
	}
	if abiJSON != "" && req.Method != "" {
		if decoded, err := evm.DecodeOutput(abiJSON, req.Method, result.ExecutionResult.Output); err == nil {
			out["decoded"] = decoded
		}
	}
	respondOK(c, out)
}

type contractWriteRequest struct {
	Address  string            `json:"address"`
	ABI      string            `json:"abi"`
	Method   string            `json:"method"`
	Args     []json.RawMessage `json:"args"`
	Data     string            `json:"data"`
	Amount   string            `json:"amount"`
	GasLimit string            `json:"gaslimit"`
	GasPrice string            `json:"gasprice"`
}

func (s *Server) writeContract(c *gin.Context) {
	user := currentUser(c)
	var req contractWriteRequest
	if !bindJSON(c, &req) {
		return
	}
	address, _, data, ok := s.prepareContractCall(c, user.ID, req.Address, req.ABI, req.Method, req.Args, req.Data)
	if !ok {
		return
	}
	gasLimit, gasPrice, err := s.gasParams(c, req.GasLimit, req.GasPrice)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_gas", err.Error())
		return
	}
	amountSat := int64(0)
	if strings.TrimSpace(req.Amount) != "" {
		amountSat, err = amount.ParseKSC(req.Amount)
		if err != nil || amountSat < 0 {
			fail(c, http.StatusBadRequest, "invalid_amount", "Invalid KSC amount.")
			return
		}
	}
	addressSender, ok := s.walletAddress(c, user.ID)
	if !ok {
		return
	}
	if !s.requireGas(c, user, addressSender) {
		return
	}
	intent, ok := s.beginIntent(c, user.ID, "contract_write", marshalPayload(req))
	if !ok {
		return
	}
	var txid string
	err = s.wallets.Do(c.Request.Context(), user.ID, func(ctx context.Context, walletName string) error {
		id, err := s.rpc.SendToContract(ctx, walletName, address, data, amountSat, gasLimit, gasPrice, addressSender)
		if err != nil {
			return err
		}
		txid = id
		return nil
	})
	s.completeIntent(c, intent, txid, err)
	if err != nil {
		audit(c, &user.ID, "contract.write_failed", "address="+address+" error="+err.Error())
		mapError(c, err)
		return
	}
	audit(c, &user.ID, "contract.write", "txid="+txid+" address="+address)
	respondOK(c, gin.H{"txid": txid, "data": data, "status": "success"})
}

type contractDeployRequest struct {
	ABI             string            `json:"abi"`
	Name            string            `json:"name"`
	Bytecode        string            `json:"bytecode"`
	ConstructorArgs []json.RawMessage `json:"constructor_args"`
	GasLimit        string            `json:"gaslimit"`
	GasPrice        string            `json:"gasprice"`
}

func (s *Server) deployContract(c *gin.Context) {
	user := currentUser(c)
	var req contractDeployRequest
	if !bindJSON(c, &req) {
		return
	}
	bytecode := normalizeHex(req.Bytecode)
	if bytecode == "" {
		fail(c, http.StatusBadRequest, "invalid_bytecode", "Contract bytecode is required.")
		return
	}
	if len(req.ConstructorArgs) > 0 {
		if req.ABI == "" {
			fail(c, http.StatusBadRequest, "abi_required", "ABI is required for constructor arguments.")
			return
		}
		encoded, err := evm.EncodeConstructor(req.ABI, req.ConstructorArgs)
		if err != nil {
			fail(c, http.StatusBadRequest, "invalid_constructor", err.Error())
			return
		}
		bytecode += encoded
	}
	gasLimit, gasPrice, err := s.gasParams(c, req.GasLimit, req.GasPrice)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_gas", err.Error())
		return
	}
	if strings.TrimSpace(req.GasLimit) == "" {
		gasLimit = s.cfg.DefaultCreateGas
		maxLimit := s.cfg.MaxContractGas
		if maxLimit > 0 && gasLimit > maxLimit {
			gasLimit = maxLimit
		}
		if gasLimit < 21_000 {
			gasLimit = 2_500_000
		}
	}
	sender, ok := s.walletAddress(c, user.ID)
	if !ok {
		return
	}
	if !s.requireGas(c, user, sender) {
		return
	}
	intent, ok := s.beginIntent(c, user.ID, "contract_deploy", marshalPayload(gin.H{"name": req.Name, "bytecode_len": len(bytecode)}))
	if !ok {
		return
	}
	var result rpc.CreateContractResult
	err = s.wallets.Do(c.Request.Context(), user.ID, func(ctx context.Context, walletName string) error {
		res, err := s.rpc.CreateContract(ctx, walletName, bytecode, gasLimit, gasPrice, sender)
		if err != nil {
			return err
		}
		result = res
		return nil
	})
	if err == nil && strings.TrimSpace(result.Address) == "" {
		err = errors.New("el nodo no devolvió la dirección del contrato")
	}
	s.completeIntent(c, intent, result.TxID, err)
	if err != nil {
		audit(c, &user.ID, "contract.deploy_failed", "error="+err.Error())
		mapError(c, err)
		return
	}
	contract := models.Contract{
		UserID:   user.ID,
		Address:  strings.ToLower(strings.TrimPrefix(result.Address, "0x")),
		Name:     truncateString(req.Name, 120),
		ABI:      req.ABI,
		Bytecode: truncateString(bytecode, 200_000),
		Deployed: true,
	}
	stored := true
	if err := s.saveContract(c.Request.Context(), &contract); err != nil {
		stored = false
		slog.Error("contract deployed but could not be saved", "address", contract.Address, "txid", result.TxID, "error", err)
	}
	audit(c, &user.ID, "contract.deploy", "txid="+result.TxID+" address="+contract.Address)
	respondOK(c, gin.H{"txid": result.TxID, "address": contract.Address, "sender": result.Sender, "stored": stored, "status": "success"})
}

func (s *Server) contractReceipts(c *gin.Context) {
	user := currentUser(c)
	contract, ok := s.contractForUser(c, user.ID)
	if !ok {
		return
	}
	txid := strings.TrimSpace(c.Query("txid"))
	if len(txid) != 64 {
		fail(c, http.StatusBadRequest, "invalid_txid", "Provide a valid txid query parameter.")
		return
	}
	receipts, err := s.rpc.GetTransactionReceipt(c.Request.Context(), txid)
	if err != nil {
		mapError(c, err)
		return
	}
	respondOK(c, gin.H{"address": contract.Address, "txid": txid, "receipts": receipts})
}

func (s *Server) contractLogs(c *gin.Context) {
	user := currentUser(c)
	contract, ok := s.contractForUser(c, user.ID)
	if !ok {
		return
	}
	from := int64(0)
	to := int64(-1)
	if v := c.Query("fromblock"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			from = n
		}
	}
	if v := c.Query("toblock"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			to = n
		}
	}
	var topics []any
	if v := c.Query("topics"); v != "" {
		_ = json.Unmarshal([]byte(v), &topics)
	}
	logs, err := s.rpc.SearchLogs(c.Request.Context(), from, to, []string{contract.Address}, topics, 0)
	if err != nil {
		mapError(c, err)
		return
	}
	respondOK(c, gin.H{"address": contract.Address, "logs": logs})
}

func (s *Server) contractCode(c *gin.Context) {
	user := currentUser(c)
	contract, ok := s.contractForUser(c, user.ID)
	if !ok {
		return
	}
	code, err := s.rpc.GetContractCode(c.Request.Context(), contract.Address, -1)
	if err != nil {
		mapError(c, err)
		return
	}
	respondOK(c, gin.H{"address": contract.Address, "code": code})
}

func (s *Server) contractForUser(c *gin.Context, userID uint) (*models.Contract, bool) {
	address, err := normalizeContractAddress(c.Param("address"))
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_contract", err.Error())
		return nil, false
	}
	var contract models.Contract
	err = s.db.WithContext(c.Request.Context()).
		Where("user_id = ? AND address = ?", userID, address).First(&contract).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusNotFound, "contract_not_found", "Contract is not imported. Import it with its ABI first.")
			return nil, false
		}
		internal(c, err)
		return nil, false
	}
	return &contract, true
}

func (s *Server) prepareContractCall(c *gin.Context, userID uint, addressRaw, abiRaw, method string, args []json.RawMessage, dataRaw string) (address, abiJSON, data string, ok bool) {
	address, err := normalizeContractAddress(addressRaw)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_contract", err.Error())
		return "", "", "", false
	}
	abiJSON = strings.TrimSpace(abiRaw)
	if abiJSON == "" {
		var contract models.Contract
		if err := s.db.WithContext(c.Request.Context()).
			Where("user_id = ? AND address = ?", userID, address).First(&contract).Error; err == nil {
			abiJSON = contract.ABI
		}
	}
	data = normalizeHex(dataRaw)
	if data == "" {
		if method == "" {
			fail(c, http.StatusBadRequest, "method_required", "Provide a method or raw calldata.")
			return "", "", "", false
		}
		if abiJSON == "" {
			fail(c, http.StatusBadRequest, "abi_required", "Provide an ABI or raw calldata.")
			return "", "", "", false
		}
		data, err = evm.EncodeCall(abiJSON, method, args)
		if err != nil {
			fail(c, http.StatusBadRequest, "invalid_call", err.Error())
			return "", "", "", false
		}
	}
	return address, abiJSON, data, true
}

func normalizeHex(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	return strings.ToLower(s)
}
