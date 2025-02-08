package main

import (
	"fmt"
	"net/url"
)

func matchesEntry(bEntry binaryEntry, binMap map[string]interface{}) bool {
	return (bEntry.Name == "" || binMap["pkg"].(string) == bEntry.Name) &&
		(bEntry.PkgId == "" || binMap["pkg_id"].(string) == bEntry.PkgId) &&
		(bEntry.Version == "" || binMap["version"].(string) == bEntry.Version)
}

// findURL fetches the URLs and BLAKE3sum for the specified binaries
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
				return nil, nil, fmt.Errorf("error: didn't find download URL for [%s]", bEntry.Name)
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
