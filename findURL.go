// findURL.go - This file implements the findURL function

package main

import (
	"fmt"
	"net/http"
)

// findURL fetches the URLs and BLAKE3sums for the specified binaries
func findURL(binaryNames []string, trackerFile string, repositories []string, metadataURLs []string, verbosityLevel Verbosity) ([]string, []string, error) {
	var foundURLs []string
	var foundB3sums []string

	for _, binaryName := range binaryNames {
		// Try to get binary info from info.go logic
		binInfo, err := getBinaryInfo(trackerFile, binaryName, metadataURLs)
		if err == nil && binInfo.DownloadURL != "" {
			// If the download_url (Source) is available, return it with BLAKE3sum
			if verbosityLevel >= extraVerbose {
				fmt.Printf("\033[2K\rFound \"%s\" via the metadata files\n", binaryName)
			}
			foundURLs = append(foundURLs, binInfo.SrcURL)
			foundB3sums = append(foundB3sums, binInfo.Bsum)
			continue
		}

		// If no valid download_url found, proceed with HEAD requests on repositories
		for i, repository := range repositories {
			url := fmt.Sprintf("%s%s", repository, binaryName)

			// Show progress only in verbose modes
			if verbosityLevel >= normalVerbosity {
				fmt.Printf("\033[2K\r<%d/%d> | Checking \"%s\" in repository \"%s\"\r", i+1, len(repositories), binaryName, repository)
			}

			resp, err := http.Head(url)
			if err == nil && resp.StatusCode == http.StatusOK {
				// If found, print message based on verbosity
				if verbosityLevel >= extraVerbose {
					fmt.Printf("\033[2K\r<%d/%d> | Found \"%s\" at %s\n", i+1, len(repositories), binaryName, repository)
				}
				foundURLs = append(foundURLs, url)
				foundB3sums = append(foundB3sums, "") // No SHA256 if found this way
				break
			}
		}

		// Cleanup last progress message if no URL was found
		if verbosityLevel >= normalVerbosity {
			fmt.Printf("\033[2K\r")
		}

		// Handle verbosity for error output
		if verbosityLevel >= silentVerbosityWithErrors {
			return nil, nil, fmt.Errorf("error: didn't find the SOURCE_URL for [%s]", binaryName)
		}
	}

	return foundURLs, foundB3sums, nil
}
