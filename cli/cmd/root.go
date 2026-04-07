package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "kave",
	Short: "Kave is a control plane for AI agents",
	Long: `Kave intercepts, traces, and controls AI agent actions. 
It acts as a middleware layer between your agents and the outside world.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var cfgFile string

func init() {
	cobra.OnInitialize(initConfig)

	// Allow user to override config path
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.kave/config.json)")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, _ := os.UserHomeDir()
		kaveDir := filepath.Join(home, ".kave")

		// Create dir if missing
		os.MkdirAll(kaveDir, 0755)

		viper.AddConfigPath(kaveDir)
		viper.SetConfigName("config")
		viper.SetConfigType("json")
	}

	viper.SetEnvPrefix("KAVE")
	viper.AutomaticEnv() // Read KAVE_PORT, etc.

	// Set Defaults
	viper.SetDefault("proxy.port", 4000)
	viper.SetDefault("api.socket", "~/.kave/kave.sock")
	viper.SetDefault("db.path", "~/.kave/kave.db")

	if err := viper.ReadInConfig(); err != nil {
		// If file doesn't exist, we can write the defaults to a new file
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			viper.SafeWriteConfig()
		}
	}
}
