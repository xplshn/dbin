package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
)

// installBinary fetches and installs the binary, logging based on verbosity levels.
func installBinary(ctx context.Context, binaryName, installDir, trackerFile string, verbosityLevel Verbosity, repositories []string) error {
	url, err := findURL(binaryName, trackerFile, repositories)
	if err != nil {
		if verbosityLevel >= silentVerbosityWithErrors {
			return err
		}
	}

	destination := filepath.Join(installDir, filepath.Base(binaryName))
	_, err = fetchBinaryFromURLToDest(ctx, url, destination)
	if err != nil {
		if verbosityLevel >= silentVerbosityWithErrors {
			return fmt.Errorf("error fetching binary %s: %v", binaryName, err)
		}
	}

	if verbosityLevel >= normalVerbosity {
		fmt.Printf("Successfully downloaded %s and put it at %s\n", binaryName, destination)
	}

	if err := addToTrackerFile(trackerFile, binaryName, installDir); err != nil && verbosityLevel >= normalVerbosity {
		if verbosityLevel >= silentVerbosityWithErrors {
			return fmt.Errorf("failed to update tracker file for %s: %v", binaryName, err)
		}
	}

	return nil
}

// multipleInstall installs multiple binaries concurrently, respecting verbosity levels.
func multipleInstall(ctx context.Context, binaries []string, installDir, trackerFile string, verbosityLevel Verbosity, repositories []string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(binaries))

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for _, binaryName := range binaries {
		wg.Add(1)
		go func(binaryName string) {
			defer wg.Done()
			if err := installBinary(ctx, binaryName, installDir, trackerFile, verbosityLevel, repositories); err != nil {
				errChan <- err
			}
		}(binaryName)
	}

	var finalErr error
	for err := range errChan {
		if finalErr == nil {
			finalErr = err
		} else {
			finalErr = fmt.Errorf("%v; %v", finalErr, err)
		}
	}

	if verbosityLevel >= normalVerbosity && finalErr != nil {
		fmt.Printf("Final errors: %v\n", finalErr)
	}

	return finalErr
}

// installCommand installs one or more binaries based on the verbosity level.
func installCommand(binaries []string, installDir, trackerFile string, verbosityLevel Verbosity, repositories []string) error {
	if len(binaries) == 1 {
		return installBinary(context.Background(), binaries[0], installDir, trackerFile, verbosityLevel, repositories)
	} else if len(binaries) > 1 {
		// Remove duplicates before processing
		binaries = removeDuplicates(binaries)
		return multipleInstall(context.Background(), binaries, installDir, trackerFile, verbosityLevel, repositories)
	}
	return nil
}

// removeDuplicates removes duplicate binaries from the list.
func removeDuplicates(binaries []string) []string {
	seen := make(map[string]struct{})
	result := []string{}
	for _, binary := range binaries {
		if _, ok := seen[binary]; !ok {
			seen[binary] = struct{}{}
			result = append(result, binary)
		}
	}
	return result
}
