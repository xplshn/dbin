package main

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

func findMatchingBins(bEntry binaryEntry, uRepoIndex []binaryEntry) ([]binaryEntry, uint16) {
	var matchingBins []binaryEntry
	var highestRank uint16

	for _, bin := range uRepoIndex {
		if bin.Name == bEntry.Name && (bEntry.PkgId == "" || bin.PkgId == bEntry.PkgId) && (bEntry.Version == "" || bin.Version == bEntry.Version) {
			matchingBins = append(matchingBins, bin)
			if bin.Rank > highestRank {
				highestRank = bin.Rank
			}
		}
	}

	return matchingBins, highestRank
}

func selectHighestRankedBin(matchingBins []binaryEntry, highestRank uint16) binaryEntry {
	if len(matchingBins) == 1 {
		return matchingBins[0]
	}

	var nonGlibcBins []binaryEntry
	for _, bin := range matchingBins {
		if !strings.Contains(bin.PkgId, "glibc") {
			nonGlibcBins = append(nonGlibcBins, bin)
		}
	}

	if len(nonGlibcBins) > 0 {
		var selectedBin binaryEntry
		var highestNonGlibcRank uint16
		for _, bin := range nonGlibcBins {
			if bin.Rank > highestNonGlibcRank {
				highestNonGlibcRank = bin.Rank
				selectedBin = bin
			}
		}
		return selectedBin
	}

	for _, bin := range matchingBins {
		if bin.Rank == highestRank {
			return bin
		}
	}

	if highestRank == 0 {
		return matchingBins[0]
	}

	return binaryEntry{}
}

func findURL(config *Config, bEntries []binaryEntry, verbosityLevel Verbosity, uRepoIndex []binaryEntry) ([]string, []string, error) {
	var foundURLs []string
	var foundB3sum []string
	var allErrors []error
	allFailed := true

	for _, bEntry := range bEntries {
		parsedURL, err := url.ParseRequestURI(bEntry.Name)
		if err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
			if verbosityLevel >= extraVerbose {
				fmt.Printf("\033[2K\rFound \"%s\" is already a valid URL", bEntry.Name)
			}
			foundURLs = append(foundURLs, bEntry.Name)
			foundB3sum = append(foundB3sum, "!no_check")
			allFailed = false
			continue
		}

		if instBEntry := bEntryOfinstalledBinary(filepath.Join(config.InstallDir, bEntry.Name)); instBEntry.Name != "" {
			bEntry = instBEntry
		}

		matchingBins, highestRank := findMatchingBins(bEntry, uRepoIndex)

		if len(matchingBins) == 0 {
			foundURLs = append(foundURLs, "!not_found")
			foundB3sum = append(foundB3sum, "!no_check")
			allErrors = append(allErrors, fmt.Errorf("didn't find download URL for [%s]", bEntry.Name + "#" + bEntry.PkgId))
			continue
		}

		allFailed = false
		selectedBin := selectHighestRankedBin(matchingBins, highestRank)

		url := ternary(selectedBin.GhcrPkg != "", selectedBin.GhcrPkg, selectedBin.DownloadURL)
		foundURLs = append(foundURLs, url)
		foundB3sum = append(foundB3sum, selectedBin.Bsum)

		if verbosityLevel >= extraVerbose {
			fmt.Printf("\033[2K\rFound \"%s\" with id=%s version=%s", bEntry.Name, selectedBin.PkgId, selectedBin.Version)
		}
	}

	if allFailed {
		var errorMessages []string
		for _, e := range allErrors {
			errorMessages = append(errorMessages, e.Error())
		}
		return nil, nil, fmt.Errorf(ternary(len(bEntries) != 1, "error: no valid download URLs found for any of the requested binaries.\n%s\n", "%s\n"), strings.Join(errorMessages, "\n"))
	}

	return foundURLs, foundB3sum, nil
}

