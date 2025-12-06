// Command dendrite starts and manages the dendrite-pulse API server.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/thorstenkramm/dendrite-pulse/internal/server"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "dendrite",
		Short: "dendrite-pulse API server",
	}

	rootCmd.PersistentFlags().Int("port", 3000, "Port to listen on")
	if err := viper.BindPFlag("port", rootCmd.PersistentFlags().Lookup("port")); err != nil {
		log.Fatalf("bind port flag: %v", err)
	}
	viper.SetDefault("port", 3000)

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Start the dendrite API server",
		RunE: func(_ *cobra.Command, _ []string) error {
			port := viper.GetInt("port")
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			addr := fmt.Sprintf(":%d", port)
			return server.Run(ctx, addr)
		},
	}

	rootCmd.AddCommand(runCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
