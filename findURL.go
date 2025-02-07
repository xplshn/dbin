package main

import (
	"fmt"
	"net/url"
)

// findURL fetches the URLs and BLAKE3sum for the specified binaries
func findURL(config *Config, bEntries []binaryEntry, verbosityLevel Verbosity, metadata map[string]interface{}) ([]string, []string, error) {
	var foundURLs []string
	var foundB3sum []string

	for _, bEntry := range bEntries {
		// Check if bEntry is a valid URL
		parsedURL, err := url.ParseRequestURI(bEntry.Name)
		if err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
			if verbosityLevel >= extraVerbose {
				fmt.Printf("\033[2K\rFound \"%s\" is already a valid URL", bEntry.Name)
			}
			foundURLs = append(foundURLs, bEntry.Name)
			foundB3sum = append(foundB3sum, "null")
			continue
		}

		// Find matching binaries
		var matchingBins []map[string]interface{}
		var highestRank uint16 = 0

		// Search through all sections
		for _, section := range metadata {
			binaries, ok := section.([]interface{})
			if !ok {
				continue
			}

			for _, bin := range binaries {
				binMap, ok := bin.(map[string]interface{})
				if !ok {
					continue
				}

				// Check if binary matches name
				if bEntry.Name != "" && binMap["pkg"].(string) != bEntry.Name {
					continue
				}

				// If ID specified, check ID match
				if bEntry.PkgId != "" && binMap["pkg_id"].(string) != bEntry.PkgId {
					continue
				}

				// If version specified, check version match
				if bEntry.Version != "" && binMap["version"].(string) != bEntry.Version {
					continue
				}

				matchingBins = append(matchingBins, binMap)

				// Track highest rank
				if rank, ok := binMap["rank"].(uint16); ok && rank > highestRank {
					highestRank = rank
				}
			}
		}

		// If matches found, select appropriate one
		if len(matchingBins) > 0 {
			var selectedBin map[string]interface{}

			if len(matchingBins) == 1 {
				selectedBin = matchingBins[0]
			} else {
				// Multiple matches - select highest rank
				for _, bin := range matchingBins {
					if rank, ok := bin["rank"].(uint16); ok && rank == highestRank {
						selectedBin = bin
						break
					}
				}
			}

			foundURLs = append(foundURLs, selectedBin["ghcr_blob"].(string))
			foundB3sum = append(foundB3sum, selectedBin["shasum"].(string))

			if verbosityLevel >= extraVerbose {
				fmt.Printf("\033[2K\rFound \"%s\" with id=%s version=%s",
					bEntry.Name, selectedBin["pkg_id"], selectedBin["version"])
			}
			continue
		}

		// If no matches found
		if verbosityLevel >= silentVerbosityWithErrors {
			return nil, nil, fmt.Errorf("error: didn't find download URL for [%s]", bEntry.Name)
		}
	}

	return foundURLs, foundB3sum, nil
}
