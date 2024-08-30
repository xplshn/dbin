// fsearch.go // this file implements the search option
package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// fSearch searches for binaries based on the given search term.
func fSearch(metadataURLs []string, installDir, tempDir, searchTerm string, disableTruncation bool, limit int) error {
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
	searchResultsSet := make(map[string]struct{})
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
			if binary.Description != "" {
				entry := fmt.Sprintf("%s - %s", binary.Name, binary.Description)
				searchResultsSet[entry] = struct{}{}
			}
		}
	}

	// Check if no matching binaries found
	if len(searchResultsSet) == 0 {
		return fmt.Errorf("no matching binaries found for '%s'", searchTerm)
	} else if len(searchResultsSet) > limit {
		return fmt.Errorf("too many matching binaries (+%d. [Use --limit before your query]) found for '%s'", limit, searchTerm)
	}

	// Convert set to slice for sorting
	var searchResults []string
	for entry := range searchResultsSet {
		searchResults = append(searchResults, entry)
	}

	// Check if the binary exists in the INSTALL_DIR and print results with installation state indicators
	for _, line := range searchResults {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) < 2 {
			return fmt.Errorf("invalid search result format: %s", line)
		}
		name := parts[0]
		description := parts[1]

		// Determine the prefix based on conditions
		var prefix string
		if installPath := filepath.Join(installDir, name); fileExists(installPath) {
			prefix = "[i]"
		} else if path, err := exec.LookPath(name); err == nil && path != "" {
			prefix = "[\033[4mi\033[0m]" // Print [i], 'i' is underlined
		} else if cachedLocation, _ := ReturnCachedFile(tempDir, filepath.Base(name)); cachedLocation != "" {
			prefix = "[c]"
		} else {
			prefix = "[-]"
		}

		truncatePrintf(disableTruncation, true, "%s %s - %s ", prefix, name, description)
	}

	return nil
}
