package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/xattr"
)

// removeCommand handles the overall flow, passing binaries to removeBinaries and ensuring tracker cleanup.
func removeCommand(binaries []string, installDir, trackerFile string, verbosityLevel Verbosity) error {
	// Call removeBinaries to handle the actual removal process
	err := removeBinaries(removeDuplicates(binaries), installDir, verbosityLevel)
	if err != nil {
		return err
	}
	// Cleanup the tracker file after all binaries are removed
	err = cleanupTrackerFile(trackerFile, installDir)
	if err != nil {
		return fmt.Errorf("error cleaning up tracker file: %w", err)
	}

	return nil
}

// removeBinaries processes each binary, removing only those that pass validation.
func removeBinaries(binaries []string, installDir string, verbosityLevel Verbosity) error {
	var wg sync.WaitGroup
	var removeErrors []string
	var mutex sync.Mutex

	// Inline function to check if a binary is ours
	validate := func(file string) bool {
		if isSymlink(file) {
			return false
		}
		managedBy, err := xattr.Get(file, "user.ManagedBy")
		if err != nil || string(managedBy) != "dbin" {
			return false
		}
		return true
	}

	// Loop over the binaries and remove the valid ones
	for _, binaryName := range binaries {
		wg.Add(1)
		go func(binaryName string) {
			defer wg.Done()

			installPath := filepath.Join(installDir, filepath.Base(binaryName))

			if !validate(installPath) {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Skipping '%s': it was not installed by dbin\n", binaryName)
				}
				return
			}

			err := os.Remove(installPath)
			if err != nil {
				if os.IsNotExist(err) {
					if verbosityLevel >= normalVerbosity {
						fmt.Fprintf(os.Stderr, "Warning: '%s' does not exist in %s\n", binaryName, installDir)
					}
				} else {
					if verbosityLevel >= silentVerbosityWithErrors {
						fmt.Fprintf(os.Stderr, "error: failed to remove '%s' from %s. %v\n", binaryName, installDir, err)
					}
					// Add error to the list in a thread-safe way, i guess
					mutex.Lock()
					removeErrors = append(removeErrors, fmt.Sprintf("failed to remove '%s' from %s: %v", binaryName, installDir, err))
					mutex.Unlock()
				}
			} else if verbosityLevel <= extraVerbose {
				fmt.Printf("'%s' removed from %s\n", binaryName, installDir)
			}
		}(binaryName)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Return concatenated errors if any occurred
	if len(removeErrors) > 0 {
		return fmt.Errorf(strings.Join(removeErrors, "\n"))
	}

	return nil
}
