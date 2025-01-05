// fsearch.go // this file implements the search option
package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// fSearch searches for binaries based on the given search term
func fSearch(config *Config, searchTerm string, metadata map[string]interface{}) error {
	type tBinary struct {
		Architecture string `json:"architecture"`
		RealName     string `json:"pkg"`
		Description  string `json:"description"`
	}

	// Fetch metadata
	var binaries []tBinary
	// Iterate over all sections and gather binaries
	for _, section := range metadata {
		binList, ok := section.([]interface{})
		if !ok {
			continue
		}

		for _, bin := range binList {
			binMap, ok := bin.(map[string]interface{})
			if !ok {
				continue
			}

			realName, _ := binMap["pkg"].(string)
			description, _ := binMap["description"].(string)
			binaries = append(binaries, tBinary{
				RealName:    realName,
				Description: description,
			})
		}
	}

	// Extract real names for filtering
	allBinaryNames := make([]string, len(binaries))
	for i, binary := range binaries {
		allBinaryNames[i] = binary.RealName
	}

	// Filter binaries using the filterBinaries function from list.go
	filteredBinaryNames := filterBinaries(allBinaryNames)

	// Filter binaries based on the search term and architecture
	searchResults := make([]string, 0)
	for _, binary := range binaries {
		if !contains(filteredBinaryNames, binary.RealName) || binary.Description == "" {
			continue
		}

		if strings.Contains(strings.ToLower(binary.RealName), strings.ToLower(searchTerm)) ||
			strings.Contains(strings.ToLower(binary.Description), strings.ToLower(searchTerm)) {
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

		truncatePrintf(disableTruncation, "%s %s - %s\n", prefix, RealName, description)
	}

	return nil
}
