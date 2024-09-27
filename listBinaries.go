// listBinaries.go // This file implements the listBinaries function //>
package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Exclude specified file types and file names, these shall not appear in Lists nor in the Search Results
var excludedFileTypes = map[string]struct{}{
	".7z":       {},
	".bz2":      {},
	".json":     {},
	".gz":       {},
	".xz":       {},
	".md":       {},
	".txt":      {},
	".tar":      {},
	".zip":      {},
	".cfg":      {},
	".dir":      {},
	".test":     {},
	".AppImage": {}, // Majority doesn't work, NixAppImages do work however.
}

var excludedFileNames = map[string]struct{}{
	"TEST":                     {},
	"LICENSE":                  {},
	"experimentalBinaries_dir": {},
	"bundles_dir":              {},
	"blobs_dir":                {},
	"robotstxt":                {},
	"bdl.sh":                   {},
	// Because the repo contains duplicated files. And I do not manage the repo nor plan to implement sha256 filtering :
	"uroot":             {},
	"uroot-busybox":     {},
	"gobusybox":         {},
	"sysinfo-collector": {},
	"neofetch":          {},
	"sh":                {}, // Because in the repo, it is a duplicate of bash and not a POSIX implementation nor the original Thompshon Shell
}

// listBinariesCommand fetches and lists binary names from the given URL.
func listBinaries(metadataURLs []string) ([]string, error) {
	var allBinaries []string
	var metadata []struct {
		Name string `json:"name"`
	}
	// Fetch binaries from each metadata URL
	for _, url := range metadataURLs {

		// Fetch metadata from the given URL
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
	var filteredBinaries []string
	for _, binary := range allBinaries {
		ext := strings.ToLower(filepath.Ext(binary))
		base := filepath.Base(binary)
		if _, excluded := excludedFileTypes[ext]; !excluded {
			if _, excludedName := excludedFileNames[base]; !excludedName {
				filteredBinaries = append(filteredBinaries, binary)
			}
		}
	}

	// Remove duplicates based on their names
	uniqueBinaries := removeDuplicates(filteredBinaries)

	// Return the list of binaries
	return uniqueBinaries, nil
}
