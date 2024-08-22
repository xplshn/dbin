// findURL.go // This file implements the findURL function //>
package main

import (
	"fmt"
	"net/http"
)

// findURL fetches the URL for the specified binary.
func findURL(binaryName, trackerFile string, repositories, metadataURLs []string) (string, error) {
	// Check the tracker file first
	realBinaryName, err := getBinaryNameFromTrackerFile(trackerFile, binaryName)
	if err == nil {
		binaryName = realBinaryName
	}

	iterations := 0
	for _, repository := range repositories {
		iterations++
		url := fmt.Sprintf("%s%s", repository, binaryName)
		fmt.Printf("\033[2K\r<%d/%d> | Working: Checking if \"%s\" is in the repos.", iterations, len(repositories), binaryName)
		resp, err := http.Head(url)
		if err != nil {
			return "", err
		}

		if resp.StatusCode == http.StatusOK {
			fmt.Printf("\033[2K\r<%d/%d> | Found \"%s\" at %s", iterations, len(repositories), binaryName, repository)
			fmt.Printf("\033[2K\r")
			return url, nil
		}
	}

	return "", fmt.Errorf("\033[2K\rDidn't find the SOURCE_URL for [%s]", binaryName)
}
