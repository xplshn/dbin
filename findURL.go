// findURL.go // This file implements the findURL function //>
package main

import (
	"fmt"
	"net/http"
)

// findURL fetches the URL and SHA256 for the specified binary
func findURL(binaryName, trackerFile string, repositories []string, metadataURLs []string) (string, string, error) {
	// First, try to get binary info from info.go logic
	binInfo, err := getBinaryInfo(trackerFile, binaryName, metadataURLs)
	if err == nil {
		if binInfo.Source != "" {
			// If the download_url (Source) is available, return it with SHA256
			fmt.Printf("\033[2K\rFound \"%s\" via the metadata files\n", binaryName)
			return binInfo.Source, binInfo.SHA256, nil
		}
	}

	// If no valid download_url found, proceed with HEAD requests on repositories
	iterations := 0
	for _, repository := range repositories {
		iterations++
		url := fmt.Sprintf("%s%s", repository, binaryName)
		fmt.Printf("\033[2K\r<%d/%d> | Working: Checking if \"%s\" is in the repos.", iterations, len(repositories), binaryName)
		resp, err := http.Head(url)

		if err == nil && resp.StatusCode == http.StatusOK {
			fmt.Printf("\033[2K\r<%d/%d> | Found \"%s\" at %s\n", iterations, len(repositories), binaryName, repository)
			return url, "", nil // No SHA256 if found this way
		}
	}

	fmt.Printf("\033[2K\r")
	return "", "", fmt.Errorf("error: didn't find the SOURCE_URL for [%s]", binaryName)
}
