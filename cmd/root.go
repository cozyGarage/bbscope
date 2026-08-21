package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/validate"
	"github.com/cozyGarage/bbscope/v2/pkg/whttp"

	"github.com/spf13/viper"
)

var cfgFile string

// Version information - set via ldflags during build
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

const (
	LOGO = `	 _     _                              
	| |__ | |__  ___  ___ ___  _ __   ___ 
	| '_ \| '_ \/ __|/ __/ _ \| '_ \ / _ \
	| |_) | |_) \__ \ (_| (_) | |_) |  __/
	|_.__/|_.__/|___/\___\___/| .__/ \___|
	                          |_|           v2
							  
`
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "bbscope",
	Short: "A powerful scope aggregator for bug bounty hunters.",
	Long: LOGO + `bbscope helps you manage bug bounty program scopes from HackerOne, Bugcrowd,
Intigriti, YesWeHack, and Immunefi, right from your command line.

Visit https://bbscope.com for an hourly-updated list of public scopes!`,
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	ExecuteContext(context.Background())
}

// ExecuteContext runs the root command with the given context for graceful shutdown support.
func ExecuteContext(ctx context.Context) {
	// Legacy support: redirect "bbscope h1" -> "bbscope poll h1" (and others)
	if len(os.Args) > 1 {
		legacyCmds := map[string]bool{
			"h1": true, "bc": true, "it": true, "ywh": true, "immunefi": true,
		}
		if legacyCmds[os.Args[1]] {
			utils.Log.Warnf("The '%s' command is deprecated. We are automatically executing 'poll %s' for you.", os.Args[1], os.Args[1])
			utils.Log.Warnf("Please switch to 'poll %s' as flags may change in future versions.", os.Args[1])

			// Inject "poll" before the subcommand
			newArgs := append([]string{"poll"}, os.Args[1:]...)
			rootCmd.SetArgs(newArgs)
		}
	}

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if ctx.Err() != nil {
			utils.Log.Info("Shutting down gracefully...")
			os.Exit(0)
		}
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.bbscope.yaml)")
	rootCmd.SetHelpCommand(&cobra.Command{
		Use:    "no-help",
		Hidden: true,
	})

	// Global flags
	rootCmd.PersistentFlags().StringP("proxy", "", "", "HTTP Proxy (Useful for debugging. Example: http://127.0.0.1:8080)")
	rootCmd.PersistentFlags().StringP("loglevel", "l", "info", "Set log level. Available: debug, info, warn, error, fatal")
	rootCmd.PersistentFlags().Bool("debug-http", false, "Debug HTTP requests and responses")

}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		viper.AddConfigPath(home)
		viper.SetConfigName(".bbscope")
		viper.SetConfigType("yaml")
	}

	viper.AutomaticEnv()

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		var cfgNotFound viper.ConfigFileNotFoundError
		if errors.As(err, &cfgNotFound) {
			// Config file not found; create it with defaults.
			home, _ := os.UserHomeDir()
			configPath := filepath.Join(home, ".bbscope.yaml")
			if err := viper.SafeWriteConfigAs(configPath); err != nil {
				fmt.Printf("Error creating config file: %s", err)
			} else {
				// Set secure permissions on newly created config file
				if err := os.Chmod(configPath, 0600); err != nil {
					utils.Log.Warnf("Could not set secure permissions on config file: %v", err)
				}
			}
		}
	} else {
		// Config file found - check permissions
		checkConfigPermissions(viper.ConfigFileUsed())
	}

	// Set default empty values for all keys
	viper.SetDefault("hackerone.username", "")
	viper.SetDefault("hackerone.token", "")
	viper.SetDefault("bugcrowd.email", "")
	viper.SetDefault("bugcrowd.password", "")
	viper.SetDefault("bugcrowd.otpsecret", "")
	viper.SetDefault("intigriti.token", "")
	viper.SetDefault("yeswehack.email", "")
	viper.SetDefault("yeswehack.password", "")
	viper.SetDefault("yeswehack.otpsecret", "")
	viper.SetDefault("ai.provider", "openai")
	viper.SetDefault("ai.model", "gpt-4o-mini")
	viper.SetDefault("ai.api_key", "")
	viper.SetDefault("ai.endpoint", "")
	viper.SetDefault("ai.max_batch", 25)
	viper.SetDefault("ai.max_concurrency", 3)
	viper.SetDefault("ai.insecure_skip_verify", false)
	viper.SetDefault("db_url", "")

	// Init log library
	levelString, _ := rootCmd.PersistentFlags().GetString("loglevel")
	utils.SetLogLevel(levelString)

	// Init HTTP debug
	debugHTTP, _ := rootCmd.PersistentFlags().GetBool("debug-http")
	whttp.GlobalDebug = debugHTTP

}

func GetDBConnectionString() (string, error) {
	url := viper.GetString("db_url")
	if url == "" {
		return "", fmt.Errorf("db_url not set in config. Please set it in ~/.bbscope.yaml")
	}
	if _, err := validate.DatabaseURL(url); err != nil {
		return "", fmt.Errorf("invalid db_url: %w", err)
	}
	return url, nil
}

// checkConfigPermissions warns if config file has overly permissive permissions
func checkConfigPermissions(path string) {
	if path == "" {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		return
	}

	// On Unix-like systems, check if group or others have read permissions
	// Mode().Perm() returns the permission bits (e.g., 0644)
	mode := info.Mode().Perm()

	// Check if group or others have any permissions (bits 0077)
	if mode&0077 != 0 {
		utils.Log.Warnf("Config file %s has insecure permissions (%04o). It contains sensitive credentials.", path, mode)
		utils.Log.Warnf("Consider running: chmod 600 %s", path)
	}
}
