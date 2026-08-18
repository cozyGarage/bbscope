package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cozyGarage/bbscope/v2/internal/utils"
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// getCmd represents the parent `db get` command.
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Extract specific scope types from the database based on format",
}

func getAndPrintTargets(ctx context.Context, targetType, platform string, aggressive bool) error {
	dbURL, err := GetDBConnectionString()
	if err != nil {
		return err
	}

	db, err := storage.Open(dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	entries, err := db.ListEntries(ctx, storage.ListOptions{Platform: platform})
	if err != nil {
		return err
	}

	for _, target := range filterTargets(entries, targetType, aggressive) {
		fmt.Println(target)
	}

	return nil
}

// filterTargets extracts the target strings from entries that match targetType.
// It is kept free of I/O so the extraction rules can be unit-tested.
func filterTargets(entries []storage.Entry, targetType string, aggressive bool) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		target := e.TargetNormalized
		if aggressive {
			target = storage.AggressiveTransform(target)
		}

		switch targetType {
		case "urls":
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
				out = append(out, target)
			}
		case "ips":
			if utils.IsIP(target) {
				out = append(out, target)
				continue
			}
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
				if u, err := url.Parse(target); err == nil {
					host := strings.Trim(u.Hostname(), "[]")
					if utils.IsIP(host) {
						out = append(out, host)
					}
				}
			}
		case "cidrs":
			if utils.IsCIDR(target) || utils.IsIPRange(target) {
				out = append(out, target)
			}
		case "wildcards":
			if strings.HasPrefix(target, "*.") {
				out = append(out, target)
			}
		case "domains":
			if isDomainTarget(target) {
				out = append(out, target)
			}
		}
	}

	return out
}

// isDomainTarget reports whether target should be treated as a domain.
// URLs, IP addresses, CIDR ranges and IP ranges are intentionally excluded so
// that `db get domains` never emits non-domain targets such as 203.0.113.5 or
// 198.51.100.0/24.
func isDomainTarget(target string) bool {
	if target == "" {
		return false
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return false
	}
	if utils.IsIP(target) || utils.IsCIDR(target) || utils.IsIPRange(target) {
		return false
	}
	return strings.Contains(target, ".")
}

func init() {
	dbCmd.AddCommand(getCmd)
}
