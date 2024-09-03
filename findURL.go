// findURL.go // This file implements the findURL function //>
package main

import (
	"fmt"
	"net/http"
	"path/filepath"
)

// findURL fetches the URL for the specified binary
func findURL(binaryName, trackerFile string, repositories []string) (string, error) {
	// Check the tracker file first
	trackedBinaryName, err := getBinaryNameFromTrackerFile(trackerFile, filepath.Base(binaryName))
	if err == nil && trackedBinaryName == binaryName {
		// If the tracked binary name matches the requested binary name, use it.
		binaryName = trackedBinaryName
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
