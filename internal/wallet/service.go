package wallet

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kron-smart-chain/webwallet/internal/amount"
	"github.com/kron-smart-chain/webwallet/internal/config"
	"github.com/kron-smart-chain/webwallet/internal/models"
	"github.com/kron-smart-chain/webwallet/internal/rpc"
	"github.com/kron-smart-chain/webwallet/internal/secret"
)

var ErrWalletNotFound = errors.New("wallet not found")

// currentSecretVersion 2 stores the node wallet passphrase (the base64 text)
// encrypted. Version 1 stored the raw 32 random bytes, which do not match the
// passphrase sent to createwallet. Version 1 is still supported for existing
// wallets and is migrated on startup.
const currentSecretVersion = 2

type Service struct {
	db      *gorm.DB
	rpc     *rpc.Client
	cfg     *config.Config
	secrets *secret.Manager

	mu     sync.Mutex
	locks  map[uint]*sync.Mutex
	loaded map[string]bool
}

func New(db *gorm.DB, client *rpc.Client, cfg *config.Config, secrets *secret.Manager) *Service {
	return &Service{
		db:      db,
		rpc:     client,
		cfg:     cfg,
		secrets: secrets,
		locks:   make(map[uint]*sync.Mutex),
		loaded:  make(map[string]bool),
	}
}

func (s *Service) lockFor(id uint) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locks[id] == nil {
		s.locks[id] = &sync.Mutex{}
	}
	return s.locks[id]
}

func (s *Service) Provision(ctx context.Context, user *models.User) (*models.Wallet, error) {
	name := s.cfg.WalletPrefix + uuid.NewString()
	passBytes, err := secret.RandomBytes(32)
	if err != nil {
		return nil, err
	}
	passphrase := base64.RawURLEncoding.EncodeToString(passBytes)
	clearBytes(passBytes)

	if err := s.rpc.CreateWallet(ctx, name, passphrase); err != nil {
		return nil, fmt.Errorf("create wallet in node: %w", err)
	}

	// The web wallet deliberately reuses its primary address; avoid_reuse=true
	// would hide used balances and can block consolidation/sending.
	_ = s.rpc.SetWalletFlag(ctx, name, "avoid_reuse", false)

	// Qtum contract operations require a P2PK/P2PKH sender. Descriptor
	// wallets may not have a legacy descriptor by default, so try to add it.
	_ = s.rpc.CreateWalletDescriptor(ctx, name, "legacy")
	address, err := s.rpc.GetNewAddress(ctx, name, "", "legacy")
	if err != nil {
		address, err = s.rpc.GetNewAddress(ctx, name, "", "")
	}
	if err != nil {
		return nil, fmt.Errorf("generate receiving address: %w", err)
	}

	ciphertext, nonce, err := s.secrets.Encrypt([]byte(passphrase), name)
	if err != nil {
		return nil, fmt.Errorf("encrypt wallet passphrase: %w", err)
	}

	wallet := &models.Wallet{
		UserID:  user.ID,
		Name:    name,
		Address: address,
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(wallet).Error; err != nil {
			return err
		}
		return tx.Create(&models.WalletSecret{
			WalletID:   wallet.ID,
			Ciphertext: ciphertext,
			Nonce:      nonce,
			KeyVersion: currentSecretVersion,
		}).Error
	})
	if err != nil {
		return nil, fmt.Errorf("persist wallet: %w", err)
	}

	s.mu.Lock()
	s.loaded[name] = true
	s.mu.Unlock()

	if s.cfg.BackupDir != "" {
		if err := os.MkdirAll(s.cfg.BackupDir, 0o700); err == nil {
			dest := filepath.Join(s.cfg.BackupDir, name+".dat")
			if err := s.rpc.BackupWallet(ctx, name, dest); err != nil {
				slog.Warn("initial wallet backup failed", "wallet", name, "error", err)
			}
		}
	}
	return wallet, nil
}

// MigrateSecrets upgrades version-1 wallet secrets in place. Version 1
// encrypted the raw random bytes, but the node wallet was created with their
// base64 text. No wallet file or key is changed, only the representation of
// the stored passphrase.
func (s *Service) MigrateSecrets(ctx context.Context) error {
	var secrets []models.WalletSecret
	if err := s.db.WithContext(ctx).Where("key_version < ?", currentSecretVersion).Find(&secrets).Error; err != nil {
		return err
	}
	for i := range secrets {
		var w models.Wallet
		if err := s.db.WithContext(ctx).First(&w, secrets[i].WalletID).Error; err != nil {
			slog.Warn("wallet secret migration: wallet not found", "wallet_id", secrets[i].WalletID, "error", err)
			continue
		}
		raw, err := s.secrets.Decrypt(secrets[i].Ciphertext, secrets[i].Nonce, w.Name)
		if err != nil {
			slog.Warn("wallet secret migration: decrypt failed", "wallet", w.Name, "error", err)
			continue
		}
		passphrase := base64.RawURLEncoding.EncodeToString(raw)
		ciphertext, nonce, err := s.secrets.Encrypt([]byte(passphrase), w.Name)
		clearBytes(raw)
		if err != nil {
			slog.Warn("wallet secret migration: encrypt failed", "wallet", w.Name, "error", err)
			continue
		}
		if err := s.db.WithContext(ctx).Model(&secrets[i]).Updates(map[string]any{
			"ciphertext":  ciphertext,
			"nonce":       nonce,
			"key_version": currentSecretVersion,
		}).Error; err != nil {
			slog.Warn("wallet secret migration: update failed", "wallet", w.Name, "error", err)
			continue
		}
		slog.Info("wallet secret migrated", "wallet", w.Name)
	}
	return nil
}

// SendGeneral spends from any wallet UTXO and returns the change to the
// user's primary address. This avoids scattering KSC across automatic change
// addresses and keeps the contract/token sender funded.
func (s *Service) SendGeneral(ctx context.Context, userID uint, to string, sat int64) (string, error) {
	w, err := s.GetByUser(ctx, userID)
	if err != nil {
		return "", err
	}
	var txid string
	err = s.Do(ctx, userID, func(ctx context.Context, walletName string) error {
		raw, err := s.rpc.CreateRawTransaction(ctx, map[string]string{to: amount.FormatKSC(sat)})
		if err != nil {
			return fmt.Errorf("createrawtransaction: %w", err)
		}
		funded, err := s.rpc.FundRawTransaction(ctx, walletName, raw, w.Address)
		if err != nil {
			return fmt.Errorf("fundrawtransaction: %w", err)
		}
		signed, err := s.rpc.SignRawTransactionWithWallet(ctx, walletName, funded.Hex)
		if err != nil {
			return fmt.Errorf("signrawtransactionwithwallet: %w", err)
		}
		if !signed.Complete {
			return errors.New("wallet returned an incomplete transaction")
		}
		txid, err = s.rpc.SendRawTransaction(ctx, signed.Hex)
		return err
	})
	return txid, err
}

// Consolidate moves all spendable KSC to the primary address. It is used when
// a token operation needs gas but the primary address has no UTXO.
func (s *Service) Consolidate(ctx context.Context, userID uint, target string) (string, error) {
	w, err := s.GetByUser(ctx, userID)
	if err != nil {
		return "", err
	}
	if target == "" {
		target = w.Address
	}
	var txid string
	err = s.Do(ctx, userID, func(ctx context.Context, walletName string) error {
		var err error
		txid, err = s.rpc.SendAll(ctx, walletName, target)
		return err
	})
	return txid, err
}

func (s *Service) PrimaryUTXOs(ctx context.Context, userID uint) ([]rpc.Unspent, error) {
	w, err := s.GetByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.UTXOsForAddress(ctx, userID, w.Address)
}

func (s *Service) UTXOsForAddress(ctx context.Context, userID uint, address string) ([]rpc.Unspent, error) {
	var out []rpc.Unspent
	err := s.Read(ctx, userID, func(ctx context.Context, walletName string) error {
		var err error
		out, err = s.rpc.ListUnspent(ctx, walletName, 0, 9_999_999, []string{address})
		return err
	})
	return out, err
}

func (s *Service) Addresses(ctx context.Context, userID uint) ([]string, error) {
	w, err := s.GetByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	var labeled []string
	_ = s.Read(ctx, userID, func(ctx context.Context, walletName string) error {
		var err error
		labeled, err = s.rpc.GetAddressesByLabel(ctx, walletName, "")
		return err
	})
	groups, _ := s.AddressGroupings(ctx, userID)

	seen := map[string]bool{w.Address: true}
	result := []string{w.Address}
	add := func(address string) {
		if address != "" && !seen[address] {
			seen[address] = true
			result = append(result, address)
		}
	}
	for _, address := range labeled {
		add(address)
	}
	for _, group := range groups {
		for _, entry := range group {
			add(entry.Address)
		}
	}
	return result, nil
}

func (s *Service) AddressGroupings(ctx context.Context, userID uint) ([][]rpc.GroupingEntry, error) {
	var out [][]rpc.GroupingEntry
	err := s.Read(ctx, userID, func(ctx context.Context, walletName string) error {
		var err error
		out, err = s.rpc.ListAddressGroupings(ctx, walletName)
		return err
	})
	return out, err
}

func (s *Service) GetByUser(ctx context.Context, userID uint) (*models.Wallet, error) {
	var w models.Wallet
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&w).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWalletNotFound
		}
		return nil, err
	}
	return &w, nil
}

func (s *Service) ensureLoaded(ctx context.Context, name string) error {
	s.mu.Lock()
	if s.loaded[name] {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	wallets, err := s.rpc.ListWallets(ctx)
	if err != nil {
		return fmt.Errorf("list node wallets: %w", err)
	}
	for _, loaded := range wallets {
		if loaded == name {
			_ = s.rpc.SetWalletFlag(ctx, name, "avoid_reuse", false)
			s.mu.Lock()
			s.loaded[name] = true
			s.mu.Unlock()
			return nil
		}
	}
	if err := s.rpc.LoadWallet(ctx, name); err != nil {
		return fmt.Errorf("load wallet: %w", err)
	}
	// Existing wallets created by older versions may have avoid_reuse=true.
	_ = s.rpc.SetWalletFlag(ctx, name, "avoid_reuse", false)
	s.mu.Lock()
	s.loaded[name] = true
	s.mu.Unlock()
	return nil
}

func (s *Service) Read(ctx context.Context, userID uint, fn func(ctx context.Context, walletName string) error) error {
	w, err := s.GetByUser(ctx, userID)
	if err != nil {
		return err
	}
	mu := s.lockFor(w.ID)
	mu.Lock()
	defer mu.Unlock()
	if err := s.ensureLoaded(ctx, w.Name); err != nil {
		return err
	}
	return fn(ctx, w.Name)
}

// Do unlocks the user's wallet for a single operation and always locks it
// again, including on panic or error.
func (s *Service) Do(ctx context.Context, userID uint, fn func(ctx context.Context, walletName string) error) error {
	w, err := s.GetByUser(ctx, userID)
	if err != nil {
		return err
	}
	mu := s.lockFor(w.ID)
	mu.Lock()
	defer mu.Unlock()

	if err := s.ensureLoaded(ctx, w.Name); err != nil {
		return err
	}

	var sec models.WalletSecret
	if err := s.db.WithContext(ctx).Where("wallet_id = ?", w.ID).First(&sec).Error; err != nil {
		return fmt.Errorf("load wallet secret: %w", err)
	}
	pass, err := s.secrets.Decrypt(sec.Ciphertext, sec.Nonce, w.Name)
	if err != nil {
		return fmt.Errorf("decrypt wallet passphrase: %w", err)
	}
	defer clearBytes(pass)

	passphrase := string(pass)
	if sec.KeyVersion < currentSecretVersion {
		// Compatibility with wallets created by the buggy v1 code.
		passphrase = base64.RawURLEncoding.EncodeToString(pass)
	}

	// A previous process may have left the wallet unlocked, or the node may
	// have been restarted with a different state. Lock first, then unlock for
	// the shortest possible window.
	_ = s.rpc.WalletLock(ctx, w.Name)
	if err := s.rpc.WalletPassphrase(ctx, w.Name, passphrase, s.cfg.WalletUnlockSecs); err != nil {
		return fmt.Errorf("unlock wallet: %w", err)
	}
	defer func() {
		lockCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.rpc.WalletLock(lockCtx, w.Name)
	}()

	return fn(ctx, w.Name)
}

func (s *Service) Backup(ctx context.Context, userID uint, destination string) error {
	return s.Read(ctx, userID, func(ctx context.Context, walletName string) error {
		return s.rpc.BackupWallet(ctx, walletName, destination)
	})
}

func clearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
