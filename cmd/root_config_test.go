package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestLoadViperConfigMissingExplicitFile(t *testing.T) {
	orig := cfgFile
	t.Cleanup(func() {
		cfgFile = orig
		viper.Reset()
	})
	viper.Reset()
	cfgFile = filepath.Join(t.TempDir(), "missing.yaml")
	viper.SetConfigFile(cfgFile)
	if err := loadViperConfig(); err == nil {
		t.Fatal("expected an error for --config pointing at a missing file")
	}
}

func TestLoadViperConfigInvalidYAML(t *testing.T) {
	orig := cfgFile
	t.Cleanup(func() {
		cfgFile = orig
		viper.Reset()
	})
	viper.Reset()
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte(":\n  - this is not: [valid yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgFile = path
	viper.SetConfigFile(path)
	if err := loadViperConfig(); err == nil {
		t.Fatal("expected an error for an unreadable config file")
	}
}

func TestLoadViperConfigValidFile(t *testing.T) {
	orig := cfgFile
	t.Cleanup(func() {
		cfgFile = orig
		viper.Reset()
	})
	viper.Reset()
	path := filepath.Join(t.TempDir(), "ok.yaml")
	if err := os.WriteFile(path, []byte("db_url: postgres://bbscope:devpass@127.0.0.1:5432/bbscope?sslmode=disable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgFile = path
	viper.SetConfigFile(path)
	if err := loadViperConfig(); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	if got := viper.GetString("db_url"); got == "" {
		t.Fatal("db_url was not loaded from the explicit --config file")
	}
}
