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
	".test", ".appimage",
}

var excludedFileNames = []string{
	"TEST", "LICENSE", "experimentalBinaries_dir", "bundles_dir",
	"blobs_dir", "robotstxt", "bdl.sh",
	"uroot", "uroot-busybox", "gobusybox",
	"sysinfo-collector", "neofetch", "sh",
}

// listBinaries fetches and lists binary names from the given metadata URLs.
func listBinaries(config *Config) ([]string, error) {
	metadataURLs := config.MetadataURLs

	var allBinaries []string
	var metadata []struct {
		RealName string `json:"bin_name"`
	}

	// Fetch binaries from each metadata URL
	for _, url := range metadataURLs {
		if err := fetchJSON(url, &metadata); err != nil {
			return nil, fmt.Errorf("failed to fetch metadata from %s: %v", url, err)
		}

		// Extract binary RealNames
		for _, item := range metadata {
			if item.RealName != "" {
				allBinaries = append(allBinaries, item.RealName)
			}
		}
	}

	// Filter out excluded file types and file RealNames
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
