package wallet

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kron-smart-chain/webwallet/internal/config"
	"github.com/kron-smart-chain/webwallet/internal/db"
	"github.com/kron-smart-chain/webwallet/internal/models"
	"github.com/kron-smart-chain/webwallet/internal/rpc"
	"github.com/kron-smart-chain/webwallet/internal/secret"
)

func TestVersionOneSecretFallbackAndMigration(t *testing.T) {
	passphrases := map[string]string{}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode rpc: %v", err)
			return
		}
		walletName := strings.TrimPrefix(r.URL.Path, "/wallet/")
		var result any
		var rpcErr any
		switch req.Method {
		case "createwallet":
			name, _ := req.Params[0].(string)
			pass, _ := req.Params[3].(string)
			passphrases[name] = pass
		case "walletpassphrase":
			pass, _ := req.Params[0].(string)
			if passphrases[walletName] == "" || passphrases[walletName] != pass {
				rpcErr = map[string]any{"code": -14, "message": "wrong passphrase"}
			}
		case "getnewaddress":
			result = "KTestAddress1111111111111111111111111"
		case "listwallets":
			result = []string{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result, "error": rpcErr, "id": req.ID})
	}))
	defer fake.Close()

	dir := t.TempDir()
	cfg := &config.Config{
		WalletPrefix:     "u_",
		DataDir:          dir,
		DBPath:           filepath.Join(dir, "test.db"),
		WalletUnlockSecs: 60,
	}
	secrets, err := secret.LoadOrCreate("", "", dir)
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := db.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	client := rpc.New(fake.URL, "", "", 5*time.Second)
	svc := New(gdb, client, cfg, secrets)
	user := &models.User{ID: 1, Username: "alice"}
	w, err := svc.Provision(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}

	var sec models.WalletSecret
	if err := gdb.Where("wallet_id = ?", w.ID).First(&sec).Error; err != nil {
		t.Fatal(err)
	}
	if sec.KeyVersion != 2 {
		t.Fatalf("new secret version = %d", sec.KeyVersion)
	}
	passphrase, err := secrets.Decrypt(sec.Ciphertext, sec.Nonce, w.Name)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(string(passphrase))
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a wallet created by the old buggy version: raw bytes encrypted
	// with KeyVersion 1.
	oldCT, oldNonce, err := secrets.Encrypt(raw, w.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.Model(&sec).Updates(map[string]any{
		"ciphertext":  oldCT,
		"nonce":       oldNonce,
		"key_version": 1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	called := false
	if err := svc.Do(context.Background(), user.ID, func(ctx context.Context, walletName string) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("v1 fallback unlock failed: %v", err)
	}
	if !called {
		t.Fatal("operation callback was not called")
	}

	if err := svc.MigrateSecrets(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Where("wallet_id = ?", w.ID).First(&sec).Error; err != nil {
		t.Fatal(err)
	}
	if sec.KeyVersion != 2 {
		t.Fatalf("migrated secret version = %d", sec.KeyVersion)
	}
	migrated, err := secrets.Decrypt(sec.Ciphertext, sec.Nonce, w.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(migrated, passphrase) {
		t.Fatal("migrated passphrase does not match original")
	}
}
