// Command dendrite starts and manages the dendrite-pulse API server.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/thorstenkramm/dendrite-pulse/internal/logging"
	"github.com/thorstenkramm/dendrite-pulse/internal/server"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "dendrite",
		Short: "dendrite-pulse API server",
	}

	rootCmd.PersistentFlags().Int("port", 3000, "Port to listen on")
	if err := viper.BindPFlag("port", rootCmd.PersistentFlags().Lookup("port")); err != nil {
		log.Fatalf("bind port flag: %v", err)
	}
	viper.SetDefault("port", 3000)
	rootCmd.PersistentFlags().String("log-level", "info", "Log level: debug, info, warn, error")
	rootCmd.PersistentFlags().String("log-file", "", "Log file path, or '-' for stdout")
	rootCmd.PersistentFlags().String("log-format", "text", "Log format: text or json")
	_ = viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))
	_ = viper.BindPFlag("log-file", rootCmd.PersistentFlags().Lookup("log-file"))
	_ = viper.BindPFlag("log-format", rootCmd.PersistentFlags().Lookup("log-format"))

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Start the dendrite API server",
		RunE:  runServer,
	}

	rootCmd.AddCommand(runCmd)
	return rootCmd
}

func runServer(_ *cobra.Command, _ []string) error {
	port := viper.GetInt("port")
	logLevel := strings.ToLower(viper.GetString("log-level"))
	logFile := viper.GetString("log-file")
	logFormat := strings.ToLower(viper.GetString("log-format"))

	var (
		appLogger *slog.Logger
		closeLog  func() error
	)

	loggingEnabled := logFile != ""
	if loggingEnabled {
		logger, closer, err := logging.NewLogger(logFile, logFormat, logLevel)
		if err != nil {
			return fmt.Errorf("setup logger: %w", err)
		}
		appLogger = logger
		closeLog = closer
		appLogger.Info("dendrite server started", "port", port)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if closeLog != nil {
		defer func() { _ = closeLog() }()
	}

	addr := fmt.Sprintf(":%d", port)
	cfg := server.Config{
		Logger:      appLogger,
		LogRequests: loggingEnabled,
	}
	if err := server.Run(ctx, addr, cfg); err != nil {
		return fmt.Errorf("run server: %w", err)
	}
	return nil
}
