// findURL.go - This file implements the findURL function

package main

import (
	"fmt"
	"net/http"
)

// findURL fetches the URL and SHA256 for the specified binary
func findURL(binaryName, trackerFile string, repositories []string, metadataURLs []string, verbosityLevel Verbosity) (string, string, error) {
	// Try to get binary info from info.go logic
	binInfo, err := getBinaryInfo(trackerFile, binaryName, metadataURLs)
	if err == nil && binInfo.Source != "" {
		// If the download_url (Source) is available, return it with SHA256
		if verbosityLevel >= extraVerbose {
			fmt.Printf("\033[2K\rFound \"%s\" via the metadata files\n", binaryName)
		}
		return binInfo.Source, binInfo.SHA256, nil
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
			return url, "", nil // No SHA256 if found this way
		}
	}

	// Cleanup last progress message if no URL was found
	if verbosityLevel >= normalVerbosity {
		fmt.Printf("\033[2K\r")
	}

	// Handle verbosity for error output
	if verbosityLevel >= silentVerbosityWithErrors {
		return "", "", fmt.Errorf("error: didn't find the SOURCE_URL for [%s]", binaryName)
	}

	return "", "", nil
}
