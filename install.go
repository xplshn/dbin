package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// installBinaries fetches multiple binaries concurrently, logging based on verbosity levels.
func installBinaries(ctx context.Context, binaries []string, installDir, trackerFile string, verbosityLevel Verbosity, repositories, metadataURLs []string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(binaries))
	urls, checksums, err := findURL(binaries, trackerFile, repositories, metadataURLs, verbosityLevel)
	if err != nil {
		return err
	}

	var installedBinaries []string
	var errors []string

	for i, binaryName := range binaries {
		wg.Add(1)
		go func(i int, binaryName string) {
			defer wg.Done()
			url := urls[i]
			checksum := checksums[i]
			destination := filepath.Join(installDir, filepath.Base(binaryName))

			// Ensure file isn't in use
			if isFileBusy(destination) {
				errChan <- fmt.Errorf("[%s] is busy and cannot be replaced", destination)
				return
			}

			// Fetch binary and put it at destination
			_, err := fetchBinaryFromURLToDest(ctx, url, checksum, destination)
			if err != nil {
				errChan <- fmt.Errorf("error fetching binary %s: %v", binaryName, err)
				return
			}

			installedBinaries = append(installedBinaries, binaryName)
		}(i, binaryName)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	// Collect errors
	for err := range errChan {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		finalErr := strings.Join(errors, "\n")
		if verbosityLevel >= silentVerbosityWithErrors {
			return fmt.Errorf(finalErr)
		}
	}

	// Update tracker file
	if trackerFile != "" {
		err := addToTrackerFile(trackerFile, installedBinaries, installDir)
		if err != nil {
			return fmt.Errorf("failed to update tracker file: %v", err)
		}
	}

	return nil
}

// installCommand installs one or more binaries based on the verbosity level.
func installCommand(binaries []string, installDir, trackerFile string, verbosityLevel Verbosity, repositories, metadataURLs []string) error {
	return installBinaries(context.Background(), removeDuplicates(binaries), installDir, trackerFile, verbosityLevel, repositories, metadataURLs)
}
