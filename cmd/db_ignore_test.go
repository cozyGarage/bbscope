package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveProgramPattern(t *testing.T) {
	cmd := &cobra.Command{Use: "ignore"}
	cmd.Flags().String("program-url", "", "")

	if _, err := resolveProgramPattern(cmd); err == nil {
		t.Fatal("expected error when --program-url is empty")
	}

	if err := cmd.Flags().Set("program-url", "https://hackerone.com/acme"); err != nil {
		t.Fatal(err)
	}
	got, err := resolveProgramPattern(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "acme") {
		t.Fatalf("pattern = %q", got)
	}
}
