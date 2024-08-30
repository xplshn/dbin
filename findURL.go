// findURL.go // This file implements the findURL function //>
package main

import (
	"fmt"
	"net/http"
)

// findURL fetches the URL for the specified binary
func findURL(binaryName, trackerFile string, repositories []string) (string, error) {
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
		resp, _ := http.Head(url)

		if resp.StatusCode == http.StatusOK {
			fmt.Printf("\033[2K\r<%d/%d> | Found \"%s\" at %s\033[2K\r", iterations, len(repositories), binaryName, repository)
			return url, nil
		}
	}

	print("\033[2K\r")
	return "", fmt.Errorf("error: didn't find the SOURCE_URL for [%s]", binaryName)
}
