package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// installBinaries fetches multiple binaries concurrently, logging based on verbosity levels.
func installBinaries(ctx context.Context, binaries []string, installDir string, verbosityLevel Verbosity, repositories, metadataURLs []string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(binaries))
	urls, checksums, err := findURL(binaries, repositories, metadataURLs, installDir, verbosityLevel)
	if err != nil {
		return err
	}

	var errors []string

	for i, binaryName := range binaries {
		wg.Add(1)
		go func(i int, binaryName string) {
			defer wg.Done()
			url := urls[i]
			checksum := checksums[i]
			destination := performCorrections(filepath.Join(installDir, filepath.Base(binaryName)))

			// Ensure file isn't in use
			if isFileBusy(destination) {
				errChan <- fmt.Errorf("[%s] is busy and cannot be replaced", destination)
				return
			}

			// Fetch binary and place it at destination
			_, err := fetchBinaryFromURLToDest(ctx, url, checksum, destination)
			if err != nil {
				errChan <- fmt.Errorf("error fetching binary %s: %v", binaryName, err)
				return
			}

			// Add full name to the binary's xattr
			if err := addFullName(destination, binaryName); err != nil {
				errChan <- fmt.Errorf("failed to add fullName property to the binary's xattr %s: %v", destination, err)
				return
			}
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

	return nil
}

// performCorrections checks the binary name for specific extensions and handles them appropiately
func performCorrections(binaryPath string) string {
	if strings.HasSuffix(binaryPath, ".no_strip") {
		return strings.TrimSuffix(binaryPath, ".no_strip")
	}
	return binaryPath
}

// installCommand installs one or more binaries based on the verbosity level.
func installCommand(binaries []string, installDir string, verbosityLevel Verbosity, repositories, metadataURLs []string) error {
	return installBinaries(context.Background(), removeDuplicates(binaries), installDir, verbosityLevel, repositories, metadataURLs)
}
