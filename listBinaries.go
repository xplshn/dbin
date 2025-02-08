package main

import (
	"path/filepath"
	"strings"
)

var excludedFileTypes = []string{
	".7z", ".bz2", ".json", ".gz", ".xz", ".md",
	".txt", ".tar", ".zip", ".cfg", ".dir",
	".dynamic", ".test",
}

var excludedFileNames = []string{
	"TEST", "LICENSE", "experimentalBinaries_dir", "bundles_dir",
	"blobs_dir", "robotstxt", "bdl.sh",
	"uroot", "uroot-busybox", "gobusybox",
	"sysinfo-collector", "neofetch", "firefox.appimage",
	"firefox-esr.appimage", "firefox-dev.appimage",
	"firefox-nightly.appimage",
}

func listBinaries(metadata map[string]interface{}) ([]binaryEntry, error) {
	var allBinaries []binaryEntry

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

			name, _ := binMap["pkg"].(string)
			pkgId, _ := binMap["pkg_id"].(string)
			version, _ := binMap["version"].(string)
			description, _ := binMap["description"].(string)
			rank, _ := binMap["rank"].(uint16)

			if name != "" {
				allBinaries = append(allBinaries, binaryEntry{
					Name:        name,
					PkgId:       pkgId,
					Version:     version,
					Description: description,
					Rank:        rank,
				})
			}
		}
	}

	filteredBinaries := filterBinaries(allBinaries)
	return removeDuplicates(filteredBinaries), nil
}

func filterBinaries(binaries []binaryEntry) []binaryEntry {
	var filtered []binaryEntry
	for _, result := range binaries {
		ext := strings.ToLower(filepath.Ext(result.Name))
		base := filepath.Base(result.Name)

		if !contains(excludedFileTypes, ext) && !contains(excludedFileNames, base) {
			filtered = append(filtered, result)
		}
	}
	return filtered
}
