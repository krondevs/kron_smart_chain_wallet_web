package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kron-smart-chain/webwallet/internal/api"
	"github.com/kron-smart-chain/webwallet/internal/auth"
	"github.com/kron-smart-chain/webwallet/internal/config"
	"github.com/kron-smart-chain/webwallet/internal/db"
	"github.com/kron-smart-chain/webwallet/internal/rpc"
	"github.com/kron-smart-chain/webwallet/internal/secret"
	"github.com/kron-smart-chain/webwallet/internal/wallet"
)

func main() {
	configPath := flag.String("config", "settings.json", "path to settings.json")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("configuration error", "error", err, "path", *configPath)
		os.Exit(1)
	}
	level := slog.LevelInfo
	if cfg.Environment == "development" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	secrets, err := secret.LoadOrCreate(cfg.MasterKey, cfg.MasterKeyFile, cfg.DataDir)
	if err != nil {
		slog.Error("master key error", "error", err)
		os.Exit(1)
	}
	gdb, err := db.Open(cfg.DBPath)
	if err != nil {
		slog.Error("database error", "error", err)
		os.Exit(1)
	}
	client := rpc.New(cfg.RPCURL, cfg.RPCUser, cfg.RPCPassword, cfg.RPCTimeout)
	walletSvc := wallet.New(gdb, client, cfg, secrets)
	if err := walletSvc.MigrateSecrets(context.Background()); err != nil {
		// The v1 fallback in wallet.Do still works; do not prevent startup.
		slog.Warn("wallet secret migration failed", "error", err)
	}
	authSvc := auth.NewService(gdb, cfg, secrets, walletSvc)

	if err := authSvc.BootstrapAdmin(context.Background()); err != nil {
		slog.Error("admin bootstrap failed", "error", err)
		os.Exit(1)
	}

	server, err := api.New(cfg, gdb, client, secrets, authSvc, walletSvc)
	if err != nil {
		slog.Error("server setup failed", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("webwallet listening", "addr", cfg.HTTPAddr, "env", cfg.Environment, "rpc", cfg.RPCURL)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server stopped", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
	slog.Info("webwallet stopped")
}
