package immunefi

import "strings"

// PLATFORM_URL is the Immunefi site root. It is a package variable so tests can
// point the poller at a local httptest server.
var PLATFORM_URL = "https://immunefi.com"

func getCategories(input string) []string {
	categories := map[string][]string{
		"web":        {"websites_and_applications"},
		"contracts":  {"smart_contract"},
		"blockchain": {"blockchain_dlt"},
		"all":        {"websites_and_applications", "smart_contract", "blockchain_dlt"},
	}

	selectedCategory, ok := categories[strings.ToLower(input)]
	if !ok {
		// Default to all if category is invalid
		return categories["all"]
	}
	return selectedCategory
}
