package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/clems4ever/ethereum-cache/internal/config"
	"github.com/clems4ever/ethereum-cache/internal/database"
	"github.com/clems4ever/ethereum-cache/internal/exporter"
	"github.com/clems4ever/ethereum-cache/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func main() {
	var cfgFile string
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	var rootCmd = &cobra.Command{
		Use:   "ethereum-cache",
		Short: "Ethereum RPC Cache Proxy",
		RunE: func(cmd *cobra.Command, args []string) error {
			var cfg config.Config
			if err := viper.Unmarshal(&cfg); err != nil {
				return fmt.Errorf("unable to decode into struct: %w", err)
			}

			if cfg.UpstreamURL == "" {
				return fmt.Errorf("upstream_url is required")
			}
			if cfg.DatabaseDSN == "" {
				return fmt.Errorf("database_dsn is required")
			}
			if cfg.Port == "" {
				cfg.Port = "8080"
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			db, err := database.NewDB(ctx, cfg.DatabaseDSN)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer db.Close()

			maxCacheSize, err := cfg.GetMaxCacheSizeBytes()
			if err != nil {
				return fmt.Errorf("invalid max_cache_size_bytes: %w", err)
			}

			exp := exporter.New(logger, db, 30*time.Second)
			go exp.Start(ctx)

			srv := server.New(logger, ":"+cfg.Port, cfg.UpstreamURL, db, cfg.AuthToken, maxCacheSize, cfg.CleanupSlackRatio, cfg.RateLimit)

			go func() {
				logger.Info("Starting server", zap.String("port", cfg.Port))
				if err := srv.Start(); err != nil {
					logger.Fatal("server error", zap.Error(err))
				}
			}()

			quit := make(chan os.Signal, 1)
			signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
			<-quit

			logger.Info("Shutting down server...")
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()

			if err := srv.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("server forced to shutdown: %w", err)
			}

			logger.Info("Server exited")
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.ethereum-cache.yaml)")

	cobra.OnInitialize(func() {
		if cfgFile != "" {
			viper.SetConfigFile(cfgFile)
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				logger.Fatal("failed to get user home dir", zap.Error(err))
			}
			viper.AddConfigPath(home)
			viper.SetConfigType("yaml")
			viper.SetConfigName(".ethereum-cache")
		}

		viper.AutomaticEnv()

		if err := viper.ReadInConfig(); err == nil {
			logger.Info("Using config file", zap.String("file", viper.ConfigFileUsed()))
		}
	})

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
