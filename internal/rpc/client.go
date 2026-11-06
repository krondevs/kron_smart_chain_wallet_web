package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kron-smart-chain/webwallet/internal/amount"
)

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

type Client struct {
	baseURL  string
	user     string
	password string
	http     *http.Client
	nextID   atomic.Uint64
}

func New(baseURL, user, password string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		user:     user,
		password: password,
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type response struct {
	Result json.RawMessage `json:"result"`
	Error  *Error          `json:"error"`
}

func (c *Client) Call(ctx context.Context, wallet, method string, params []any, out any) error {
	if params == nil {
		params = []any{}
	}
	id := c.nextID.Add(1)
	body, err := json.Marshal(request{
		JSONRPC: "1.0",
		ID:      strconv.FormatUint(id, 10),
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return err
	}

	endpoint := c.baseURL
	if wallet != "" {
		endpoint += "/wallet/" + url.PathEscape(wallet)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.user != "" || c.password != "" {
		req.SetBasicAuth(c.user, c.password)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("rpc transport: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, 32<<20)
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("rpc read: %w", err)
	}
	// Bitcoin/Qtum may return an HTTP 500 with a valid JSON-RPC error body.
	// Parse that body first so callers still get a typed rpc.Error.
	var parsed response
	parseErr := json.Unmarshal(data, &parsed)
	if parseErr == nil && parsed.Error != nil {
		return parsed.Error
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("rpc authentication failed (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("rpc HTTP %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	if parseErr != nil {
		return fmt.Errorf("rpc decode: %w", parseErr)
	}
	if out == nil || len(parsed.Result) == 0 || string(parsed.Result) == "null" {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(parsed.Result))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("rpc result decode: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ---- chain / node ----

type BlockchainInfo struct {
	Chain                string      `json:"chain"`
	Blocks               int64       `json:"blocks"`
	Headers              int64       `json:"headers"`
	BestBlockHash        string      `json:"bestblockhash"`
	Difficulty           float64     `json:"difficulty"`
	VerificationProgress float64     `json:"verificationprogress"`
	InitialBlockDownload bool        `json:"initialblockdownload"`
	MoneySupply          json.Number `json:"moneysupply"`
	Warnings             any         `json:"warnings"`
}

type DGPInfo struct {
	MaxBlockSize  uint64 `json:"maxblocksize"`
	MinGasPrice   uint64 `json:"mingasprice"`
	BlockGasLimit uint64 `json:"blockgaslimit"`
}

func (c *Client) GetBlockchainInfo(ctx context.Context) (BlockchainInfo, error) {
	var out BlockchainInfo
	err := c.Call(ctx, "", "getblockchaininfo", nil, &out)
	return out, err
}

func (c *Client) GetBlockCount(ctx context.Context) (int64, error) {
	var out int64
	err := c.Call(ctx, "", "getblockcount", nil, &out)
	return out, err
}

func (c *Client) GetDGPInfo(ctx context.Context) (DGPInfo, error) {
	var out DGPInfo
	err := c.Call(ctx, "", "getdgpinfo", nil, &out)
	if err != nil {
		return out, err
	}
	if out.MinGasPrice == 0 {
		out.MinGasPrice = 40 // DEFAULT_GAS_PRICE in validation.h
	}
	if out.BlockGasLimit == 0 {
		out.BlockGasLimit = 2_500_000
	}
	return out, nil
}

func (c *Client) EstimateSmartFee(ctx context.Context, blocks int) (float64, error) {
	var out struct {
		FeeRate *float64 `json:"feerate"`
		Errors  []string `json:"errors"`
	}
	if err := c.Call(ctx, "", "estimatesmartfee", []any{blocks}, &out); err != nil {
		return 0, err
	}
	if out.FeeRate == nil {
		return 0, errors.New("fee estimation unavailable")
	}
	return *out.FeeRate, nil
}

func (c *Client) ValidateAddress(ctx context.Context, address string) (map[string]any, error) {
	var out map[string]any
	err := c.Call(ctx, "", "validateaddress", []any{address}, &out)
	return out, err
}

// ---- multiwallet lifecycle ----

func (c *Client) ListWallets(ctx context.Context) ([]string, error) {
	var out []string
	err := c.Call(ctx, "", "listwallets", nil, &out)
	return out, err
}

func (c *Client) CreateWallet(ctx context.Context, name, passphrase string) error {
	return c.Call(ctx, "", "createwallet", []any{
		name, false, false, passphrase, true, true, false,
	}, nil)
}

func (c *Client) LoadWallet(ctx context.Context, name string) error {
	return c.Call(ctx, "", "loadwallet", []any{name, false}, nil)
}

func (c *Client) UnloadWallet(ctx context.Context, name string) error {
	return c.Call(ctx, "", "unloadwallet", []any{name, false}, nil)
}

func (c *Client) CreateWalletDescriptor(ctx context.Context, wallet, addressType string) error {
	return c.Call(ctx, wallet, "createwalletdescriptor", []any{addressType}, nil)
}

func (c *Client) GetNewAddress(ctx context.Context, wallet, label, addressType string) (string, error) {
	var out string
	err := c.Call(ctx, wallet, "getnewaddress", []any{label, addressType}, &out)
	return out, err
}

func (c *Client) GetWalletInfo(ctx context.Context, wallet string) (map[string]any, error) {
	var out map[string]any
	err := c.Call(ctx, wallet, "getwalletinfo", nil, &out)
	return out, err
}

func (c *Client) GetBalance(ctx context.Context, wallet string) (int64, error) {
	var raw json.Number
	// avoid_reuse=false is required: the web wallet intentionally reuses its
	// primary address, and the default true would hide its balance in the UI.
	if err := c.Call(ctx, wallet, "getbalance", []any{"*", 0, false, false}, &raw); err != nil {
		return 0, err
	}
	return amount.ParseKSC(raw.String())
}

func (c *Client) ListTransactions(ctx context.Context, wallet string, count, skip int) ([]map[string]any, error) {
	var out []map[string]any
	err := c.Call(ctx, wallet, "listtransactions", []any{"*", count, skip, true}, &out)
	return out, err
}

func (c *Client) GetTransaction(ctx context.Context, wallet, txid string) (map[string]any, error) {
	var out map[string]any
	err := c.Call(ctx, wallet, "gettransaction", []any{txid, true}, &out)
	return out, err
}

func (c *Client) WalletPassphrase(ctx context.Context, wallet, passphrase string, seconds int) error {
	return c.Call(ctx, wallet, "walletpassphrase", []any{passphrase, seconds}, nil)
}

func (c *Client) WalletLock(ctx context.Context, wallet string) error {
	return c.Call(ctx, wallet, "walletlock", nil, nil)
}

func (c *Client) BackupWallet(ctx context.Context, wallet, destination string) error {
	return c.Call(ctx, wallet, "backupwallet", []any{destination}, nil)
}

func (c *Client) SendToAddress(ctx context.Context, wallet, address string, sat int64, comment string) (string, error) {
	params := []any{
		address,
		amount.FormatKSC(sat),
		comment,
		"",
		false,   // subtractfeefromamount
		true,    // replaceable
		6,       // conf_target
		"unset", // estimate_mode
		false,   // avoid_reuse
		nil,     // fee_rate
		false,   // verbose
		nil,     // senderaddress
		false,   // changetosender
	}
	var txid string
	if err := c.Call(ctx, wallet, "sendtoaddress", params, &txid); err != nil {
		return "", err
	}
	if txid == "" {
		return "", errors.New("node returned an empty txid")
	}
	return txid, nil
}

// ---- contracts ----

type CallContractResult struct {
	Address         string `json:"address"`
	ExecutionResult struct {
		GasUsed         uint64 `json:"gasUsed"`
		Excepted        string `json:"excepted"`
		NewAddress      string `json:"newAddress"`
		Output          string `json:"output"`
		ExceptedMessage string `json:"exceptedMessage"`
	} `json:"executionResult"`
	TransactionReceipt map[string]any `json:"transactionReceipt"`
}

func (c *Client) CallContract(ctx context.Context, address, data, sender string, gasLimit uint64, amountSat int64, blockNum int64) (CallContractResult, error) {
	var out CallContractResult
	params := []any{address, data}
	if sender != "" {
		params = append(params, sender)
	} else {
		params = append(params, nil)
	}
	params = append(params, gasLimit, amount.FormatKSC(amountSat), blockNum)
	err := c.Call(ctx, "", "callcontract", params, &out)
	return out, err
}

func (c *Client) SendToContract(ctx context.Context, wallet, contract, data string, amountSat int64, gasLimit uint64, gasPriceSat int64, sender string) (string, error) {
	params := []any{
		contract,
		data,
		amount.FormatKSC(amountSat),
		gasLimit,
		amount.FormatKSC(gasPriceSat),
		sender,
		true,  // broadcast
		true,  // changetosender
		false, // psbt
	}
	var out struct {
		TxID string `json:"txid"`
	}
	if err := c.Call(ctx, wallet, "sendtocontract", params, &out); err != nil {
		return "", err
	}
	if out.TxID == "" {
		return "", errors.New("node returned an empty txid")
	}
	return out.TxID, nil
}

type CreateContractResult struct {
	TxID    string `json:"txid"`
	Sender  string `json:"sender"`
	Hash160 string `json:"hash160"`
	Address string `json:"address"`
}

func (c *Client) CreateContract(ctx context.Context, wallet, bytecode string, gasLimit uint64, gasPriceSat int64, sender string) (CreateContractResult, error) {
	var out CreateContractResult
	params := []any{
		bytecode,
		gasLimit,
		amount.FormatKSC(gasPriceSat),
		sender,
		true,  // broadcast
		true,  // changetosender
		false, // psbt
	}
	if err := c.Call(ctx, wallet, "createcontract", params, &out); err != nil {
		return out, err
	}
	if out.TxID == "" {
		return out, errors.New("node returned an empty txid")
	}
	return out, nil
}

func (c *Client) GetTransactionReceipt(ctx context.Context, txid string) ([]map[string]any, error) {
	var out []map[string]any
	err := c.Call(ctx, "", "gettransactionreceipt", []any{txid}, &out)
	return out, err
}

func (c *Client) SearchLogs(ctx context.Context, from, to int64, addresses []string, topics []any, minconf int) ([]map[string]any, error) {
	addressFilter := map[string]any{}
	if len(addresses) > 0 {
		addressFilter["addresses"] = addresses
	}
	topicFilter := map[string]any{}
	if len(topics) > 0 {
		topicFilter["topics"] = topics
	}
	var out []map[string]any
	err := c.Call(ctx, "", "searchlogs", []any{from, to, addressFilter, topicFilter, minconf}, &out)
	return out, err
}

func (c *Client) ListContracts(ctx context.Context, start, maxDisplay int) (map[string]any, error) {
	var out map[string]any
	err := c.Call(ctx, "", "listcontracts", []any{start, maxDisplay}, &out)
	return out, err
}

func (c *Client) GetContractCode(ctx context.Context, address string, blockNum int64) (string, error) {
	var out string
	err := c.Call(ctx, "", "getcontractcode", []any{address, blockNum}, &out)
	return out, err
}

// ---- balances / UTXOs / general spending ----

type Balances struct {
	Mine struct {
		Trusted          json.Number `json:"trusted"`
		UntrustedPending json.Number `json:"untrusted_pending"`
		Immature         json.Number `json:"immature"`
		Stake            json.Number `json:"stake"`
	} `json:"mine"`
}

func (c *Client) GetBalances(ctx context.Context, wallet string) (Balances, error) {
	var out Balances
	err := c.Call(ctx, wallet, "getbalances", nil, &out)
	return out, err
}

type Unspent struct {
	TxID          string      `json:"txid"`
	Vout          uint32      `json:"vout"`
	Address       string      `json:"address"`
	Amount        json.Number `json:"amount"`
	Confirmations int         `json:"confirmations"`
	Spendable     bool        `json:"spendable"`
	Solvable      bool        `json:"solvable"`
}

func (c *Client) ListUnspent(ctx context.Context, wallet string, minconf, maxconf int, addresses []string) ([]Unspent, error) {
	if addresses == nil {
		addresses = []string{}
	}
	var out []Unspent
	err := c.Call(ctx, wallet, "listunspent", []any{minconf, maxconf, addresses}, &out)
	return out, err
}

type GroupingEntry struct {
	Address   string `json:"address"`
	Amount    string `json:"amount"`
	AmountSat int64  `json:"amount_sat"`
	Label     string `json:"label,omitempty"`
}

func (c *Client) SetWalletFlag(ctx context.Context, wallet, flag string, value bool) error {
	return c.Call(ctx, wallet, "setwalletflag", []any{flag, value}, nil)
}

type AddressInfo struct {
	Address     string `json:"address"`
	IsMine      bool   `json:"ismine"`
	IsWatchOnly bool   `json:"iswatchonly"`
	Solvable    bool   `json:"solvable"`
}

func (c *Client) GetAddressInfo(ctx context.Context, wallet, address string) (AddressInfo, error) {
	var out AddressInfo
	err := c.Call(ctx, wallet, "getaddressinfo", []any{address}, &out)
	return out, err
}

func (c *Client) GetAddressesByLabel(ctx context.Context, wallet, label string) ([]string, error) {
	var raw map[string]any
	if err := c.Call(ctx, wallet, "getaddressesbylabel", []any{label}, &raw); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(raw))
	for address := range raw {
		out = append(out, address)
	}
	return out, nil
}

func (c *Client) ListAddressGroupings(ctx context.Context, wallet string) ([][]GroupingEntry, error) {
	var raw [][][]any
	if err := c.Call(ctx, wallet, "listaddressgroupings", nil, &raw); err != nil {
		return nil, err
	}
	groups := make([][]GroupingEntry, 0, len(raw))
	for _, group := range raw {
		entries := make([]GroupingEntry, 0, len(group))
		for _, item := range group {
			if len(item) < 2 {
				continue
			}
			address, _ := item[0].(string)
			amountStr := ""
			switch v := item[1].(type) {
			case json.Number:
				amountStr = v.String()
			case string:
				amountStr = v
			}
			sat, err := amount.ParseKSC(amountStr)
			if err != nil {
				continue
			}
			label := ""
			if len(item) > 2 {
				label, _ = item[2].(string)
			}
			entries = append(entries, GroupingEntry{
				Address:   address,
				Amount:    amount.FormatKSC(sat),
				AmountSat: sat,
				Label:     label,
			})
		}
		if len(entries) > 0 {
			groups = append(groups, entries)
		}
	}
	return groups, nil
}

func (c *Client) CreateRawTransaction(ctx context.Context, outputs map[string]string) (string, error) {
	var out string
	err := c.Call(ctx, "", "createrawtransaction", []any{[]any{}, outputs}, &out)
	return out, err
}

type FundedTransaction struct {
	Hex       string      `json:"hex"`
	Fee       json.Number `json:"fee"`
	ChangePos int         `json:"changepos"`
}

func (c *Client) FundRawTransaction(ctx context.Context, wallet, hex, changeAddress string) (FundedTransaction, error) {
	var out FundedTransaction
	options := map[string]any{"change_address": changeAddress}
	err := c.Call(ctx, wallet, "fundrawtransaction", []any{hex, options}, &out)
	return out, err
}

type SignedTransaction struct {
	Hex      string `json:"hex"`
	Complete bool   `json:"complete"`
}

func (c *Client) SignRawTransactionWithWallet(ctx context.Context, wallet, hex string) (SignedTransaction, error) {
	var out SignedTransaction
	err := c.Call(ctx, wallet, "signrawtransactionwithwallet", []any{hex}, &out)
	return out, err
}

func (c *Client) SendRawTransaction(ctx context.Context, hex string) (string, error) {
	var out string
	if err := c.Call(ctx, "", "sendrawtransaction", []any{hex}, &out); err != nil {
		return "", err
	}
	if out == "" {
		return "", errors.New("node returned an empty txid")
	}
	return out, nil
}

func (c *Client) SendAll(ctx context.Context, wallet, address string) (string, error) {
	var out struct {
		TxID string `json:"txid"`
	}
	params := []any{[]string{address}, nil, "unset", nil, map[string]any{}}
	if err := c.Call(ctx, wallet, "sendall", params, &out); err != nil {
		return "", err
	}
	if out.TxID == "" {
		return "", errors.New("node returned an empty txid")
	}
	return out.TxID, nil
}

// ---- KRC20 / QRC20 ----

// tokenCall uses the canonical QRC20 RPC names and falls back to the KRC20
// aliases for nodes that only expose the branded names. QRC20 is the on-chain
// standard; KRC20 is a display alias.
func (c *Client) tokenCall(ctx context.Context, wallet, method string, params []any, out any) error {
	err := c.Call(ctx, wallet, method, params, out)
	if err != nil && isMethodNotFound(err) {
		alias := "krc20" + strings.TrimPrefix(method, "qrc20")
		return c.Call(ctx, wallet, alias, params, out)
	}
	return err
}

func isMethodNotFound(err error) bool {
	var rpcErr *Error
	return errors.As(err, &rpcErr) && rpcErr.Code == -32601
}

func (c *Client) KRC20Name(ctx context.Context, contract string) (string, error) {
	var out string
	err := c.tokenCall(ctx, "", "qrc20name", []any{contract}, &out)
	return out, err
}

func (c *Client) KRC20Symbol(ctx context.Context, contract string) (string, error) {
	var out string
	err := c.tokenCall(ctx, "", "qrc20symbol", []any{contract}, &out)
	return out, err
}

func (c *Client) KRC20Decimals(ctx context.Context, contract string) (int, error) {
	var out int
	err := c.tokenCall(ctx, "", "qrc20decimals", []any{contract}, &out)
	return out, err
}

func (c *Client) KRC20TotalSupply(ctx context.Context, contract string) (string, error) {
	var out string
	err := c.tokenCall(ctx, "", "qrc20totalsupply", []any{contract}, &out)
	return out, err
}

func (c *Client) KRC20BalanceOf(ctx context.Context, contract, address string) (string, error) {
	var out string
	err := c.tokenCall(ctx, "", "qrc20balanceof", []any{contract, address}, &out)
	return out, err
}

func (c *Client) KRC20Allowance(ctx context.Context, contract, owner, spender string) (string, error) {
	var out string
	err := c.tokenCall(ctx, "", "qrc20allowance", []any{contract, owner, spender}, &out)
	return out, err
}

func (c *Client) KRC20ListTransactions(ctx context.Context, contract, address string, fromBlock, minconf int64) ([]map[string]any, error) {
	var out []map[string]any
	err := c.tokenCall(ctx, "", "qrc20listtransactions", []any{contract, address, fromBlock, minconf}, &out)
	return out, err
}

func (c *Client) KRC20Transfer(ctx context.Context, wallet, contract, owner, to, tokenAmount string, gasLimit uint64, gasPriceSat int64) (string, error) {
	params := []any{
		contract, owner, to, tokenAmount,
		gasLimit, amount.FormatKSC(gasPriceSat), true,
	}
	var out struct {
		TxID string `json:"txid"`
	}
	if err := c.tokenCall(ctx, wallet, "qrc20transfer", params, &out); err != nil {
		return "", err
	}
	if out.TxID == "" {
		return "", errors.New("node returned an empty txid")
	}
	return out.TxID, nil
}

func (c *Client) KRC20Approve(ctx context.Context, wallet, contract, owner, spender, tokenAmount string, gasLimit uint64, gasPriceSat int64) (string, error) {
	params := []any{
		contract, owner, spender, tokenAmount,
		gasLimit, amount.FormatKSC(gasPriceSat), true,
	}
	var out struct {
		TxID string `json:"txid"`
	}
	if err := c.tokenCall(ctx, wallet, "qrc20approve", params, &out); err != nil {
		return "", err
	}
	if out.TxID == "" {
		return "", errors.New("node returned an empty txid")
	}
	return out.TxID, nil
}

func (c *Client) KRC20TransferFrom(ctx context.Context, wallet, contract, owner, spender, receiver, tokenAmount string, gasLimit uint64, gasPriceSat int64) (string, error) {
	params := []any{
		contract, owner, spender, receiver, tokenAmount,
		gasLimit, amount.FormatKSC(gasPriceSat), true,
	}
	var out struct {
		TxID string `json:"txid"`
	}
	if err := c.tokenCall(ctx, wallet, "qrc20transferfrom", params, &out); err != nil {
		return "", err
	}
	if out.TxID == "" {
		return "", errors.New("node returned an empty txid")
	}
	return out.TxID, nil
}

func (c *Client) KRC20Burn(ctx context.Context, wallet, contract, owner, tokenAmount string, gasLimit uint64, gasPriceSat int64) (string, error) {
	params := []any{
		contract, owner, tokenAmount,
		gasLimit, amount.FormatKSC(gasPriceSat), true,
	}
	var out struct {
		TxID string `json:"txid"`
	}
	if err := c.tokenCall(ctx, wallet, "qrc20burn", params, &out); err != nil {
		return "", err
	}
	if out.TxID == "" {
		return "", errors.New("node returned an empty txid")
	}
	return out.TxID, nil
}

func (c *Client) KRC20BurnFrom(ctx context.Context, wallet, contract, owner, spender, tokenAmount string, gasLimit uint64, gasPriceSat int64) (string, error) {
	params := []any{
		contract, owner, spender, tokenAmount,
		gasLimit, amount.FormatKSC(gasPriceSat), true,
	}
	var out struct {
		TxID string `json:"txid"`
	}
	if err := c.tokenCall(ctx, wallet, "qrc20burnfrom", params, &out); err != nil {
		return "", err
	}
	if out.TxID == "" {
		return "", errors.New("node returned an empty txid")
	}
	return out.TxID, nil
}
