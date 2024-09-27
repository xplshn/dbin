// listBinaries.go // This file implements the listBinaries function //>
package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Excluded file types and file names that shall not appear in Lists nor in Search Results
var excludedFileTypes = []string{
	".7z", ".bz2", ".json", ".gz", ".xz", ".md",
	".txt", ".tar", ".zip", ".cfg", ".dir",
	".test", ".AppImage",
}

var excludedFileNames = []string{
	"TEST", "LICENSE", "experimentalBinaries_dir", "bundles_dir",
	"blobs_dir", "robotstxt", "bdl.sh",
	"uroot", "uroot-busybox", "gobusybox",
	"sysinfo-collector", "neofetch", "sh",
}

// listBinaries fetches and lists binary names from the given metadata URLs.
func listBinaries(metadataURLs []string) ([]string, error) {
	var allBinaries []string
	var metadata []struct {
		Name string `json:"name"`
	}

	// Fetch binaries from each metadata URL
	for _, url := range metadataURLs {
		if err := fetchJSON(url, &metadata); err != nil {
			return nil, fmt.Errorf("failed to fetch metadata from %s: %v", url, err)
		}

		// Extract binary names
		for _, item := range metadata {
			if item.Name != "" {
				allBinaries = append(allBinaries, item.Name)
			}
		}
	}

	// Filter out excluded file types and file names
	filteredBinaries := filterBinaries(allBinaries)

	// Return unique binaries
	return removeDuplicates(filteredBinaries), nil
}

// filterBinaries filters the list of binaries based on exclusions.
func filterBinaries(binaries []string) []string {
	var filtered []string
	for _, binary := range binaries {
		ext := strings.ToLower(filepath.Ext(binary))
		base := filepath.Base(binary)

		// Check for exclusions
		if !contains(excludedFileTypes, ext) && !contains(excludedFileNames, base) {
			filtered = append(filtered, binary)
		}
	}
	return filtered
}
