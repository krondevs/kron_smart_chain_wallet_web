package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/kron-smart-chain/webwallet/internal/api"
	"github.com/kron-smart-chain/webwallet/internal/auth"
	"github.com/kron-smart-chain/webwallet/internal/config"
	"github.com/kron-smart-chain/webwallet/internal/db"
	"github.com/kron-smart-chain/webwallet/internal/rpc"
	"github.com/kron-smart-chain/webwallet/internal/secret"
	"github.com/kron-smart-chain/webwallet/internal/wallet"
)

type rpcRequest struct {
	ID     any    `json:"id"`
	Method string `json:"method"`
	Params []any  `json:"params"`
}

func fakeRPC(t *testing.T) *httptest.Server {
	t.Helper()
	txid := strings.Repeat("a", 64)
	passphrases := map[string]string{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("fake rpc decode: %v", err)
			return
		}
		var result any
		var rpcError map[string]any
		walletName := strings.TrimPrefix(r.URL.Path, "/wallet/")
		switch req.Method {
		case "createwallet":
			name, _ := req.Params[0].(string)
			pass, _ := req.Params[3].(string)
			passphrases[name] = pass
			result = nil
		case "walletpassphrase":
			pass, _ := req.Params[0].(string)
			if passphrases[walletName] == "" || passphrases[walletName] != pass {
				rpcError = map[string]any{"code": -14, "message": "Error: The wallet passphrase entered was incorrect."}
			}
			result = nil
		case "loadwallet", "walletlock", "createwalletdescriptor":
			result = nil
		case "getnewaddress":
			result = "KTestAddress1111111111111111111111111"
		case "listwallets":
			result = []string{}
		case "getbalance":
			result = 1.0
		case "getbalances":
			result = map[string]any{"mine": map[string]any{"trusted": 1.0, "untrusted_pending": 0, "immature": 0, "stake": 0}}
		case "listunspent":
			result = []map[string]any{{"txid": txid, "vout": 0, "address": "KTestAddress1111111111111111111111111", "amount": 0.2, "confirmations": 1, "spendable": true, "solvable": true}}
		case "listaddressgroupings":
			result = []any{[]any{[]any{"KTestAddress1111111111111111111111111", 1.0, ""}}}
		case "getaddressinfo":
			result = map[string]any{"address": req.Params[0], "ismine": true, "iswatchonly": false, "solvable": true}
		case "getwalletinfo":
			result = map[string]any{"walletname": "u_test", "balance": 1.0}
		case "getblockchaininfo":
			result = map[string]any{
				"chain": "main", "blocks": 100, "headers": 100,
				"bestblockhash": strings.Repeat("b", 64), "difficulty": 1,
				"verificationprogress": 1, "initialblockdownload": false,
				"moneysupply": 100000000,
			}
		case "getdgpinfo":
			result = map[string]any{"maxblocksize": 1000000, "mingasprice": 40, "blockgaslimit": 2500000}
		case "validateaddress":
			result = map[string]any{"isvalid": true, "address": req.Params[0]}
		case "sendtoaddress":
			result = txid
		case "createrawtransaction":
			result = "rawhex"
		case "fundrawtransaction":
			result = map[string]any{"hex": "fundedhex", "fee": 0.0001, "changepos": 1}
		case "signrawtransactionwithwallet":
			result = map[string]any{"hex": "signedhex", "complete": true}
		case "sendrawtransaction":
			result = txid
		case "sendall":
			result = map[string]any{"txid": txid, "complete": true}
		case "listtransactions":
			result = []any{}
		case "estimatesmartfee":
			result = map[string]any{"feerate": 0.00001}
		case "qrc20name":
			result = "Litecoin"
		case "qrc20symbol":
			result = "LTC"
		case "qrc20decimals":
			result = 8
		case "qrc20totalsupply":
			result = "84000000.00000000"
		case "qrc20balanceof":
			result = "123.45"
		case "qrc20transfer":
			amountStr, _ := req.Params[3].(string)
			if amountStr != "0.0014" {
				rpcError = map[string]any{"code": -8, "message": "unexpected token amount: " + amountStr}
			} else {
				result = map[string]any{"txid": txid}
			}
		case "createcontract":
			result = map[string]any{"txid": txid, "sender": "KTestAddress1111111111111111111111111", "hash160": "", "address": strings.Repeat("e", 40)}
		case "getcontractcode":
			result = "6000"
		default:
			result = nil
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result, "error": rpcError, "id": req.ID})
	}))
}

func TestRegisterVerifyAndSend(t *testing.T) {
	fake := fakeRPC(t)
	defer fake.Close()

	dir := t.TempDir()
	cfg := &config.Config{
		Environment:         "test",
		HTTPAddr:            ":0",
		DataDir:             dir,
		DBPath:              filepath.Join(dir, "test.db"),
		RPCURL:              fake.URL,
		RPCTimeout:          5 * time.Second,
		SessionCookieName:   "ksc_session",
		CSRFCookieName:      "ksc_csrf",
		CookieSecure:        false,
		SessionTTL:          time.Hour,
		MFAReverifyAfter:    time.Hour,
		WalletUnlockSecs:    60,
		RegistrationEnabled: true,
		RequireTOTP:         true,
		DefaultPerTxLimit:   100_00000000,
		DefaultDailyLimit:   1000_00000000,
		MaxAccountsPerIP:    0,
		MaxLoadedWallets:    10,
		MaxContractGas:      2_500_000,
		MaxGasPriceSat:      10_000,
		DefaultGasLimit:     250_000,
		DefaultCreateGas:    2_500_000,
		KSCDecimals:         8,
		WalletPrefix:        "u_",
		TOTPIssuer:          "Kron Test",
	}
	secrets, err := secret.LoadOrCreate("", "", dir)
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := db.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	client := rpc.New(cfg.RPCURL, "", "", cfg.RPCTimeout)
	walletSvc := wallet.New(gdb, client, cfg, secrets)
	authSvc := auth.NewService(gdb, cfg, secrets, walletSvc)
	srv, err := api.New(cfg, gdb, client, secrets, authSvc, walletSvc)
	if err != nil {
		t.Fatal(err)
	}
	app := httptest.NewServer(srv.Router())
	defer app.Close()

	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{Jar: jar}

	csrf := getCSRF(t, httpClient, app.URL)

	var register struct {
		Data struct {
			User struct {
				Username    string `json:"username"`
				TOTPEnabled bool   `json:"totp_enabled"`
			} `json:"user"`
		} `json:"data"`
	}
	postJSON(t, httpClient, app.URL+"/api/v1/auth/register", csrf, map[string]any{
		"username": "alice", "password": "correct horse 1234",
	}, &register)
	if register.Data.User.Username != "alice" {
		t.Fatalf("register user = %#v", register.Data.User)
	}
	if register.Data.User.TOTPEnabled {
		t.Fatal("2FA must be disabled by default")
	}

	// API keys: max three active, Bearer auth, and revocation.
	var firstKey struct {
		Data struct {
			Key    string `json:"key"`
			APIKey struct {
				ID     uint   `json:"id"`
				Prefix string `json:"prefix"`
			} `json:"api_key"`
		} `json:"data"`
	}
	for i := 0; i < 3; i++ {
		var created struct {
			Data struct {
				Key    string `json:"key"`
				APIKey struct {
					ID     uint   `json:"id"`
					Prefix string `json:"prefix"`
				} `json:"api_key"`
			} `json:"data"`
		}
		postJSON(t, httpClient, app.URL+"/api/v1/api-keys", csrf, map[string]any{
			"name": "app-" + strconv.Itoa(i), "expires_in_days": 7,
		}, &created)
		if created.Data.Key == "" || created.Data.APIKey.Prefix == "" {
			t.Fatalf("api key %d = %#v", i, created.Data)
		}
		if i == 0 {
			firstKey = created
		}
	}
	status, _ := postJSONStatus(t, httpClient, app.URL+"/api/v1/api-keys", csrf, map[string]any{
		"name": "too-many", "expires_in_days": 7,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("fourth api key status = %d", status)
	}

	var appMe struct {
		Data struct {
			User struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"data"`
	}
	doAPIKey(t, http.MethodGet, app.URL+"/api/v1/app/me", firstKey.Data.Key, "", nil, &appMe, http.StatusOK)
	if appMe.Data.User.Username != "alice" {
		t.Fatalf("app me = %#v", appMe.Data)
	}
	var appWallet struct {
		Data struct {
			Address string `json:"address"`
		} `json:"data"`
	}
	doAPIKey(t, http.MethodGet, app.URL+"/api/v1/app/wallet", firstKey.Data.Key, "", nil, &appWallet, http.StatusOK)
	if appWallet.Data.Address == "" {
		t.Fatal("app wallet missing address")
	}
	var appSent struct {
		Data struct {
			TxID string `json:"txid"`
		} `json:"data"`
	}
	doAPIKey(t, http.MethodPost, app.URL+"/api/v1/app/wallet/send", firstKey.Data.Key, "api-key-send-1", map[string]any{
		"address": "KTestAddress1111111111111111111111111", "amount": "0.01",
	}, &appSent, http.StatusOK)
	if appSent.Data.TxID != strings.Repeat("a", 64) {
		t.Fatalf("app send txid = %q", appSent.Data.TxID)
	}
	if err := deleteWithCSRF(t, httpClient, app.URL+"/api/v1/api-keys/"+strconv.FormatUint(uint64(firstKey.Data.APIKey.ID), 10), csrf); err != nil {
		t.Fatal(err)
	}
	doAPIKey(t, http.MethodGet, app.URL+"/api/v1/app/me", firstKey.Data.Key, "", nil, nil, http.StatusUnauthorized)

	var walletData struct {
		Data struct {
			Address          string `json:"address"`
			Balance          string `json:"balance"`
			PrimaryBalance   string `json:"primary_balance"`
			PrimaryUTXOCount *int   `json:"primary_utxo_count"`
			Addresses        []struct {
				Address string `json:"address"`
				Amount  string `json:"amount"`
			} `json:"addresses"`
		} `json:"data"`
	}
	getJSON(t, httpClient, app.URL+"/api/v1/wallet", &walletData)
	if walletData.Data.Address == "" {
		t.Fatal("missing wallet address")
	}
	if walletData.Data.Balance != "1" {
		t.Fatalf("balance = %q", walletData.Data.Balance)
	}
	if walletData.Data.PrimaryUTXOCount == nil || *walletData.Data.PrimaryUTXOCount != 1 {
		t.Fatalf("primary utxo count = %#v", walletData.Data.PrimaryUTXOCount)
	}
	if len(walletData.Data.Addresses) == 0 || walletData.Data.PrimaryBalance == "" {
		t.Fatalf("wallet balance breakdown missing: %#v", walletData.Data)
	}

	var sent struct {
		Data struct {
			TxID string `json:"txid"`
		} `json:"data"`
	}
	postJSONWithKey(t, httpClient, app.URL+"/api/v1/wallet/send", csrf, "test-key-1", map[string]any{
		"address": "KTestAddress1111111111111111111111111",
		"amount":  "0.1",
	}, &sent)
	if sent.Data.TxID != strings.Repeat("a", 64) {
		t.Fatalf("txid = %q", sent.Data.TxID)
	}

	// Idempotent replay returns the same txid.
	var replay struct {
		Data struct {
			TxID     string `json:"txid"`
			Replayed bool   `json:"replayed"`
		} `json:"data"`
	}
	postJSONWithKey(t, httpClient, app.URL+"/api/v1/wallet/send", csrf, "test-key-1", map[string]any{
		"address": "KTestAddress1111111111111111111111111",
		"amount":  "0.1",
	}, &replay)
	if replay.Data.TxID != sent.Data.TxID || !replay.Data.Replayed {
		t.Fatalf("replay = %#v", replay.Data)
	}

	var setup struct {
		Data struct {
			Secret        string   `json:"secret"`
			RecoveryCodes []string `json:"recovery_codes"`
		} `json:"data"`
	}
	postJSON(t, httpClient, app.URL+"/api/v1/auth/totp/setup", csrf, map[string]any{}, &setup)
	if setup.Data.Secret == "" || len(setup.Data.RecoveryCodes) != auth.RecoveryCodeCount {
		t.Fatalf("totp setup = %#v", setup.Data)
	}
	code, err := totp.GenerateCode(setup.Data.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var verify map[string]any
	postJSON(t, httpClient, app.URL+"/api/v1/auth/totp/verify", csrf, map[string]any{"code": code}, &verify)

	var added struct {
		Data struct {
			Contract string `json:"contract"`
			Name     string `json:"name"`
			Symbol   string `json:"symbol"`
		} `json:"data"`
	}
	postJSON(t, httpClient, app.URL+"/api/v1/tokens", csrf, map[string]any{
		"contract": "f996498457a417fee934db11e10e814f1a866b8b",
	}, &added)
	if added.Data.Name != "Litecoin" || added.Data.Symbol != "LTC" {
		t.Fatalf("token add = %#v", added.Data)
	}

	var tokenSent struct {
		Data struct {
			TxID string `json:"txid"`
		} `json:"data"`
	}
	postJSONWithKey(t, httpClient, app.URL+"/api/v1/tokens/f996498457a417fee934db11e10e814f1a866b8b/transfer", csrf, "token-transfer-1", map[string]any{
		"to":     "KTestAddress1111111111111111111111111",
		"amount": "0.0014",
	}, &tokenSent)
	if tokenSent.Data.TxID != strings.Repeat("a", 64) {
		t.Fatalf("token transfer txid = %q", tokenSent.Data.TxID)
	}

	var consolidated struct {
		Data struct {
			TxID string `json:"txid"`
		} `json:"data"`
	}
	postJSONWithKey(t, httpClient, app.URL+"/api/v1/wallet/consolidate", csrf, "consolidate-1", map[string]any{}, &consolidated)
	if consolidated.Data.TxID != strings.Repeat("a", 64) {
		t.Fatalf("consolidate txid = %q", consolidated.Data.TxID)
	}

	var prepared struct {
		Data struct {
			TxID string `json:"txid"`
		} `json:"data"`
	}
	postJSONWithKey(t, httpClient, app.URL+"/api/v1/wallet/prepare-gas", csrf, "prepare-gas-1", map[string]any{
		"address": "KTestAddress1111111111111111111111111",
	}, &prepared)
	if prepared.Data.TxID != strings.Repeat("a", 64) {
		t.Fatalf("prepare gas txid = %q", prepared.Data.TxID)
	}

	var deployed struct {
		Data struct {
			TxID    string `json:"txid"`
			Address string `json:"address"`
		} `json:"data"`
	}
	postJSONWithKey(t, httpClient, app.URL+"/api/v1/contracts/deploy", csrf, "deploy-1", map[string]any{
		"name":     "Test Contract",
		"bytecode": "6000",
		"gaslimit": "2500000",
		"gasprice": "0.00000040",
	}, &deployed)
	if deployed.Data.TxID != strings.Repeat("a", 64) || deployed.Data.Address != strings.Repeat("e", 40) {
		t.Fatalf("deploy = %#v", deployed.Data)
	}

	// A deployed contract that is not imported in this account must still
	// return its address so the UI can copy it and offer an ABI import.
	var fallback struct {
		Data struct {
			Address  string `json:"address"`
			Imported *bool  `json:"imported"`
		} `json:"data"`
	}
	getJSON(t, httpClient, app.URL+"/api/v1/contracts/"+strings.Repeat("f", 40), &fallback)
	if fallback.Data.Address != strings.Repeat("f", 40) || fallback.Data.Imported == nil || *fallback.Data.Imported {
		t.Fatalf("contract fallback = %#v", fallback.Data)
	}

	code, err = totp.GenerateCode(setup.Data.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var disabled struct {
		Data struct {
			TOTPEnabled bool `json:"totp_enabled"`
		} `json:"data"`
	}
	postJSON(t, httpClient, app.URL+"/api/v1/auth/totp/disable", csrf, map[string]any{
		"password": "correct horse 1234",
		"code":     code,
	}, &disabled)
	if disabled.Data.TOTPEnabled {
		t.Fatal("2FA was not disabled")
	}

	var afterDisable struct {
		Data struct {
			TxID string `json:"txid"`
		} `json:"data"`
	}
	postJSONWithKey(t, httpClient, app.URL+"/api/v1/wallet/send", csrf, "after-disable", map[string]any{
		"address": "KTestAddress1111111111111111111111111",
		"amount":  "0.01",
	}, &afterDisable)
	if afterDisable.Data.TxID != strings.Repeat("a", 64) {
		t.Fatalf("send after disabling 2FA = %#v", afterDisable.Data)
	}
}

func getCSRF(t *testing.T, client *http.Client, base string) string {
	t.Helper()
	resp, err := client.Get(base + "/api/v1/auth/csrf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Data struct {
			Token string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Token == "" {
		t.Fatal("empty CSRF token")
	}
	return body.Data.Token
}

func postJSON(t *testing.T, client *http.Client, url, csrf string, payload any, out any) {
	t.Helper()
	postJSONWithKey(t, client, url, csrf, "", payload, out)
}

func postJSONWithKey(t *testing.T, client *http.Client, url, csrf, idempotency string, payload any, out any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	if idempotency != "" {
		req.Header.Set("Idempotency-Key", idempotency)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("POST %s: HTTP %d: %s", url, resp.StatusCode, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("decode response: %v: %s", err, data)
		}
	}
}

func getJSON(t *testing.T, client *http.Client, url string, out any) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: HTTP %d: %s", url, resp.StatusCode, data)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode response: %v: %s", err, data)
	}
}

func postJSONStatus(t *testing.T, client *http.Client, url, csrf string, payload any) (int, []byte) {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func doAPIKey(t *testing.T, method, url, apiKey, idempotency string, payload any, out any, wantStatus int) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotency != "" {
		req.Header.Set("Idempotency-Key", idempotency)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: HTTP %d: %s", method, url, resp.StatusCode, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("decode %s: %v: %s", url, err, data)
		}
	}
}

func deleteWithCSRF(t *testing.T, client *http.Client, url, csrf string) error {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s: HTTP %d: %s", url, resp.StatusCode, data)
	}
	return nil
}
