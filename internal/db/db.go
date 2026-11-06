package db

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kron-smart-chain/webwallet/internal/models"
)

func Open(path string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	gormLogger := logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
		SlowThreshold:             time.Second,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	})
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormLogger})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	// SQLite has a single writer. Keeping one connection avoids "database is
	// locked" errors while still allowing WAL readers through the same pool.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if err := migrate(gdb); err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		// Not fatal on filesystems that do not support chmod.
		_ = err
	}
	return gdb, nil
}

func migrate(gdb *gorm.DB) error {
	return gdb.AutoMigrate(
		&models.User{},
		&models.Session{},
		&models.Wallet{},
		&models.WalletSecret{},
		&models.Token{},
		&models.UserToken{},
		&models.Contract{},
		&models.AddressBookEntry{},
		&models.TxIntent{},
		&models.AuditEvent{},
		&models.RecoveryCode{},
		&models.DailyUsage{},
		&models.APIKey{},
	)
}
