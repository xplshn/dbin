package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// installBinary fetches and installs the binary, logging based on verbosity levels.
func installBinary(ctx context.Context, binaryName, installDir, trackerFile string, verbosityLevel Verbosity, repositories, metadataURLs []string) error {
	url, checksum, err := findURL(binaryName, trackerFile, repositories, metadataURLs)
	if err != nil {
		// Return the error directly without printing/logging
		return err
	}

	destination := filepath.Join(installDir, filepath.Base(binaryName))
	_, err = fetchBinaryFromURLToDest(ctx, url, checksum, destination)
	if err != nil {
		// Return the error directly without printing/logging
		return fmt.Errorf("error fetching binary %s: %v", binaryName, err)
	}

	if verbosityLevel >= normalVerbosity {
		fmt.Printf("Successfully downloaded %s and put it at %s\n", binaryName, destination)
	}

	if trackerFile != "" {
		err = addToTrackerFile(trackerFile, binaryName, installDir)
		if err != nil {
			// Return the error directly without printing/logging
			return fmt.Errorf("failed to update tracker file for %s: %v", binaryName, err)
		}
	}

	return nil
}

// multipleInstall installs multiple binaries concurrently, respecting verbosity levels.
func multipleInstall(ctx context.Context, binaries []string, installDir, trackerFile string, verbosityLevel Verbosity, repositories, metadataURLs []string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(binaries))

	for _, binaryName := range binaries {
		wg.Add(1)
		go func(binaryName string) {
			defer wg.Done()
			if err := installBinary(ctx, binaryName, installDir, trackerFile, verbosityLevel, repositories, metadataURLs); err != nil {
				errChan <- err
			}
		}(binaryName)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	var errors []string
	for err := range errChan {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		// Join errors with newline character
		finalErr := strings.Join(errors, "\n")
		if verbosityLevel >= silentVerbosityWithErrors {
			return fmt.Errorf(finalErr)
		}
	}

	return nil
}

// installCommand installs one or more binaries based on the verbosity level.
func installCommand(binaries []string, installDir, trackerFile string, verbosityLevel Verbosity, repositories, metadataURLs []string) error {
	if len(binaries) == 1 {
		return installBinary(context.Background(), binaries[0], installDir, trackerFile, verbosityLevel, repositories, metadataURLs)
	} else if len(binaries) > 1 {
		// Remove duplicates before processing
		binaries = removeDuplicates(binaries)
		return multipleInstall(context.Background(), binaries, installDir, trackerFile, verbosityLevel, repositories, metadataURLs)
	}
	return nil
}
