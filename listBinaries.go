// listBinaries.go // This file implements the listBinaries function //>
package main

import (
	"path/filepath"
	"strings"
)

// Excluded file types and file names that shall not appear in Lists nor in Search Results
var excludedFileTypes = []string{
	".7z", ".bz2", ".json", ".gz", ".xz", ".md",
	".txt", ".tar", ".zip", ".cfg", ".dir",
	".test", //".appimage"
}

var excludedFileNames = []string{
	"TEST", "LICENSE", "experimentalBinaries_dir", "bundles_dir",
	"blobs_dir", "robotstxt", "bdl.sh",
	"uroot", "uroot-busybox", "gobusybox",
	"sysinfo-collector", "neofetch", "firefox.appimage",
	"firefox-esr.appimage", "firefox-dev.appimage",
	"firefox-nightly.appimage",
}

// listBinaries fetches and lists binary names from the given metadata URLs.
func listBinaries(metadata map[string]interface{}) ([]string, error) {
	var allBinaries []string

	// Iterate over all sections in the metadata
	for _, section := range metadata {
		binaries, ok := section.([]interface{})
		if !ok {
			continue
		}

		for _, item := range binaries {
			binMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			realName, _ := binMap["pkg"].(string)
			if realName != "" {
				allBinaries = append(allBinaries, realName)
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
