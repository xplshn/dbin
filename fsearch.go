// fsearch.go // this file implements the search option
package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// fSearch searches for binaries based on the given search term
func fSearch(config *Config, searchTerm string) error {
	type tBinary struct {
		Architecture string `json:"architecture"`
		RealName     string `json:"bin_name"`
		Description  string `json:"description"`
	}

	// Fetch metadata
	var binaries []tBinary
	for _, url := range config.MetadataURLs {
		var tempBinaries []tBinary
		err := fetchJSON(url, &tempBinaries)
		if err != nil {
			return fmt.Errorf("failed to fetch and decode binary information from %s: %v", url, err)
		}
		binaries = append(binaries, tempBinaries...)
	}

	// Filter binaries based on exclusions
	var allBinaryRealNames []string
	for _, binary := range binaries {
		if binary.RealName != "" {
			allBinaryRealNames = append(allBinaryRealNames, binary.RealName)
		}
	}
	filteredBinaries := filterBinaries(allBinaryRealNames)

	// Filter binaries based on the search term and architecture
	searchResults := make([]string, 0)
	for _, binary := range binaries {
		if contains(filteredBinaries, binary.RealName) &&
			(strings.Contains(strings.ToLower(binary.RealName), strings.ToLower(searchTerm)) ||
				strings.Contains(strings.ToLower(binary.Description), strings.ToLower(searchTerm))) {

			entry := fmt.Sprintf("%s - %s", binary.RealName, binary.Description)
			searchResults = append(searchResults, entry)
		}
	}

	// Check if no matching binaries found
	limit := config.Limit
	if len(searchResults) == 0 {
		return fmt.Errorf("no matching binaries found for '%s'", searchTerm)
	} else if len(searchResults) > limit {
		return fmt.Errorf("too many matching binaries (+%d. [Use --limit before your query]) found for '%s'", limit, searchTerm)
	}

	// Maps to track installed and cached binaries
	installedBinaries := make(map[string]bool)

	// Check if the binary exists in the INSTALL_DIR and print results with installation state indicators
	installDir := config.InstallDir
	disableTruncation := config.DisableTruncation
	for _, line := range searchResults {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) < 2 {
			return fmt.Errorf("invalid search result format: %s", line)
		}
		RealName := parts[0]
		description := parts[1]
		baseRealName := filepath.Base(RealName)

		// Determine the prefix based on conditions
		prefix := "[-]"
		_, cachedBinaryRealName, _ := ReturnCachedFile(config, RealName)

		if installPath := filepath.Join(installDir, baseRealName); fileExists(installPath) && !installedBinaries[baseRealName] {
			prefix = "[i]"
			installedBinaries[baseRealName] = true
		} else if cachedBinaryRealName == RealName {
			prefix = "[c]"
		}

		truncatePrintf(disableTruncation, true, "%s %s - %s ", prefix, RealName, description)
	}

	return nil
}
