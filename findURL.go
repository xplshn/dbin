package main

import (
	"fmt"
	"net/url"
	"strings"
)

func findMatchingBins(bEntry binaryEntry, metadata map[string]interface{}) ([]map[string]interface{}, uint16) {
	var matchingBins []map[string]interface{}
	var highestRank uint16

	for _, section := range metadata {
		if binaries, ok := section.([]interface{}); ok {
			for _, bin := range binaries {
				if binMap, ok := bin.(map[string]interface{}); ok && matchesEntry(bEntry, binMap) {
					matchingBins = append(matchingBins, binMap)
					if rank, ok := binMap["rank"].(uint16); ok && rank > highestRank {
						highestRank = rank
					}
				}
			}
		}
	}

	return matchingBins, highestRank
}

func selectHighestRankedBin(matchingBins []map[string]interface{}, highestRank uint16) map[string]interface{} {
	if len(matchingBins) == 1 {
		return matchingBins[0]
	}

	var nonGlibcBins []map[string]interface{}

	// Collect all bins that do not contain "glibc" in their PkgId
	for _, bin := range matchingBins {
		if pkgId, ok := bin["PkgId"].(string); ok && !strings.Contains(pkgId, "glibc") {
			nonGlibcBins = append(nonGlibcBins, bin)
		}
	}

	// If there are non-glibc bins, select the one with the highest rank
	if len(nonGlibcBins) > 0 {
		var selectedBin map[string]interface{}
		var highestNonGlibcRank uint16
		for _, bin := range nonGlibcBins {
			if rank, ok := bin["rank"].(uint16); ok && rank > highestNonGlibcRank {
				highestNonGlibcRank = rank
				selectedBin = bin
			}
		}
		return selectedBin
	}

	// If no non-glibc bins, select the highest ranked bin overall
	for _, bin := range matchingBins {
		if rank, ok := bin["rank"].(uint16); ok && rank == highestRank {
			return bin
		}
	}

	if highestRank == 0 {
		return matchingBins[0]
	}

	return nil
}

func matchesEntry(bEntry binaryEntry, binMap map[string]interface{}) bool {
	return (binMap["pkg"].(string) == bEntry.Name) &&
		(bEntry.PkgId == "" || binMap["pkg_id"].(string) == bEntry.PkgId) &&
		(bEntry.Version == "" || binMap["version"].(string) == bEntry.Version)
}

func findURL(config *Config, bEntries []binaryEntry, verbosityLevel Verbosity, metadata map[string]interface{}) ([]string, []string, error) {
	var foundURLs []string
	var foundB3sum []string

	for _, bEntry := range bEntries {
		parsedURL, err := url.ParseRequestURI(bEntry.Name)
		if err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
			if verbosityLevel >= extraVerbose {
				fmt.Printf("\033[2K\rFound \"%s\" is already a valid URL", bEntry.Name)
			}
			foundURLs = append(foundURLs, bEntry.Name)
			foundB3sum = append(foundB3sum, "!no_check")
			continue
		}

		matchingBins, highestRank := findMatchingBins(bEntry, metadata)

		if len(matchingBins) == 0 {
			if verbosityLevel >= silentVerbosityWithErrors {
				return nil, nil, fmt.Errorf("error: didn't find download URL for [%s]\n", bEntry.Name)
			}
			continue
		}

		selectedBin := selectHighestRankedBin(matchingBins, highestRank)

		foundURLs = append(foundURLs, selectedBin["ghcr_pkg"].(string))
		foundB3sum = append(foundB3sum, selectedBin["bsum"].(string))

		if verbosityLevel >= extraVerbose {
			fmt.Printf("\033[2K\rFound \"%s\" with id=%s version=%s",
				bEntry.Name, selectedBin["pkg_id"], selectedBin["version"])
		}
	}

	return foundURLs, foundB3sum, nil
}
