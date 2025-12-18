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

	"github.com/thorstenkramm/dendrite-pulse/internal/config"
	"github.com/thorstenkramm/dendrite-pulse/internal/files"
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
	rootCmd.PersistentFlags().String("listen", "127.0.0.1", "Listen address")
	if err := viper.BindPFlag("main.port", rootCmd.PersistentFlags().Lookup("port")); err != nil {
		log.Fatalf("bind port flag: %v", err)
	}
	if err := viper.BindPFlag("main.listen", rootCmd.PersistentFlags().Lookup("listen")); err != nil {
		log.Fatalf("bind listen flag: %v", err)
	}
	rootCmd.PersistentFlags().String("log-level", "info", "Log level: debug, info, warn, error")
	rootCmd.PersistentFlags().String("log-file", "", "Log file path, or '-' for stdout")
	rootCmd.PersistentFlags().String("log-format", "text", "Log format: text or json")
	rootCmd.PersistentFlags().String("config", "/etc/dendrite/dendrite.conf", "Path to config file")
	fileRootHelp := "File roots as /virtual:/source mapping (repeatable or comma-separated)"
	rootCmd.PersistentFlags().StringArray("file-root", nil, fileRootHelp)
	if err := viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config")); err != nil {
		log.Fatalf("bind config flag: %v", err)
	}
	if err := viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		log.Fatalf("bind log-level flag: %v", err)
	}
	if err := viper.BindPFlag("log.file", rootCmd.PersistentFlags().Lookup("log-file")); err != nil {
		log.Fatalf("bind log-file flag: %v", err)
	}
	if err := viper.BindPFlag("log.format", rootCmd.PersistentFlags().Lookup("log-format")); err != nil {
		log.Fatalf("bind log-format flag: %v", err)
	}
	if err := viper.BindPFlag("file-root", rootCmd.PersistentFlags().Lookup("file-root")); err != nil {
		log.Fatalf("bind file-root flag: %v", err)
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Start the dendrite API server",
		RunE:  runServer,
	}
	runCmd.Flags().Bool("config-check", false, "Validate configuration and exit")
	if err := viper.BindPFlag("config-check", runCmd.Flags().Lookup("config-check")); err != nil {
		log.Fatalf("bind config-check flag: %v", err)
	}

	rootCmd.AddCommand(runCmd)
	return rootCmd
}

func runServer(_ *cobra.Command, _ []string) error {
	cfgPath := viper.GetString("config")
	cfg, err := config.NewLoader(viper.GetViper()).Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if viper.GetBool("config-check") {
		fmt.Printf("Config OK: %s (file roots: %d)\n", cfgPath, len(cfg.FileRoots))
		return nil
	}

	port := cfg.Main.Port
	listen := cfg.Main.Listen
	logLevel := strings.ToLower(cfg.Log.Level)
	logFile := cfg.Log.File
	logFormat := strings.ToLower(cfg.Log.Format)

	appLogger, closeLog, err := setupLogger(logFile, logFormat, logLevel)
	if err != nil {
		return err
	}
	loggingEnabled := appLogger != nil
	if loggingEnabled {
		appLogger.Info("dendrite server started", "port", port)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if closeLog != nil {
		defer func() { _ = closeLog() }()
	}

	fileRoots := make([]files.Root, 0, len(cfg.FileRoots))
	for _, root := range cfg.FileRoots {
		fileRoots = append(fileRoots, files.Root{
			Virtual: root.Virtual,
			Source:  root.Source,
		})
	}
	fileSvc, err := files.NewService(fileRoots)
	if err != nil {
		return fmt.Errorf("init file service: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", listen, port)
	cfgSrv := server.Config{
		Logger:      appLogger,
		LogRequests: loggingEnabled,
		FileService: fileSvc,
	}
	if err := server.Run(ctx, addr, cfgSrv); err != nil {
		return fmt.Errorf("run server: %w", err)
	}
	return nil
}

func setupLogger(logFile, logFormat, logLevel string) (*slog.Logger, func() error, error) {
	if logFile == "" {
		return nil, nil, nil
	}

	logger, closer, err := logging.NewLogger(logFile, logFormat, logLevel)
	if err != nil {
		return nil, nil, fmt.Errorf("setup logger: %w", err)
	}

	return logger, closer, nil
}
