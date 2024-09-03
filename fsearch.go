// fsearch.go // this file implements the search option
package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// fSearch searches for binaries based on the given search term
func fSearch(metadataURLs []string, installDir, tempDir, searchTerm string, disableTruncation bool, limit int, runCommandTrackerFile string) error {
	type tBinary struct {
		Architecture string `json:"architecture"`
		Name         string `json:"name"`
		Description  string `json:"description"`
	}

	// Fetch metadata
	var binaries []tBinary
	for _, url := range metadataURLs {
		var tempBinaries []tBinary
		err := fetchJSON(url, &tempBinaries)
		if err != nil {
			return fmt.Errorf("failed to fetch and decode binary information from %s: %v", url, err)
		}
		binaries = append(binaries, tempBinaries...)
	}

	// Filter binaries based on the search term and architecture
	searchResults := make([]string, 0)
	for _, binary := range binaries {
		if strings.Contains(strings.ToLower(binary.Name), strings.ToLower(searchTerm)) || strings.Contains(strings.ToLower(binary.Description), strings.ToLower(searchTerm)) {
			ext := strings.ToLower(filepath.Ext(binary.Name))
			base := filepath.Base(binary.Name)
			if _, excluded := excludedFileTypes[ext]; excluded {
				continue // Skip this binary if its extension is excluded
			}
			if _, excludedName := excludedFileNames[base]; excludedName {
				continue // Skip this binary if its name is excluded
			}
			entry := fmt.Sprintf("%s - %s", binary.Name, binary.Description)
			searchResults = append(searchResults, entry)
		}
	}

	// Check if no matching binaries found
	if len(searchResults) == 0 {
		return fmt.Errorf("no matching binaries found for '%s'", searchTerm)
	} else if len(searchResults) > limit {
		return fmt.Errorf("too many matching binaries (+%d. [Use --limit before your query]) found for '%s'", limit, searchTerm)
	}

	// Maps to track installed and cached binaries
	installedBinaries := make(map[string]bool)

	// Check if the binary exists in the INSTALL_DIR and print results with installation state indicators
	for _, line := range searchResults {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) < 2 {
			return fmt.Errorf("invalid search result format: %s", line)
		}
		name := parts[0]
		description := parts[1]
		baseName := filepath.Base(name)

		// Determine the prefix based on conditions
		prefix := "[-]"
		cachedLocation, trackedBinaryName := ReturnCachedFile(tempDir, name, runCommandTrackerFile)

		if installPath := filepath.Join(installDir, baseName); fileExists(installPath) && !installedBinaries[baseName] {
			prefix = "[i]"
			installedBinaries[baseName] = true
		} else if trackedBinaryName == name && fileExists(cachedLocation) {
			prefix = "[c]"
		}

		truncatePrintf(disableTruncation, true, "%s %s - %s ", prefix, name, description)
	}

	return nil
}
