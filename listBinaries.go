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

func listBinaries(uRepoIndex []binaryEntry) ([]binaryEntry, error) {
	var allBinaries []binaryEntry

	for _, bin := range uRepoIndex {
		name, pkgId, version, description, rank := bin.Name, bin.PkgId, bin.Version, bin.Description, bin.Rank

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

	return filterBinaries(allBinaries), nil
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
