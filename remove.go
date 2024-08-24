// remove.go // This file implements the functionality of "remove" //>
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// removeCommand handles the removal of one or more binaries based on the verbosity level.
func removeCommand(binaries []string, installDir, trackerFile string, verbosityLevel Verbosity) error {
	if len(binaries) == 1 {
		err := removeBinary(binaries[0], installDir, verbosityLevel)
		if err != nil {
			return err
		}
	} else if len(binaries) > 1 {
		// Remove duplicates before processing
		binaries = removeDuplicates(binaries)
		err := multipleRemove(binaries, installDir, verbosityLevel)
		if err != nil {
			return err
		}
	}

	// Cleanup the tracker file after removal
	err := cleanupTrackerFile(trackerFile, installDir)
	if err != nil {
		return fmt.Errorf("error cleaning up tracker file: %w", err)
	}

	return nil
}

// removeBinary removes a single binary and logs the operation based on verbosity level.
func removeBinary(binaryName, installDir string, verbosityLevel Verbosity) error {
	installPath := filepath.Join(installDir, filepath.Base(binaryName))
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
			return err
		}
	} else if verbosityLevel <= extraVerbose {
		fmt.Printf("'%s' removed from %s\n", binaryName, installDir)
	}
	return nil
}

// multipleRemove removes multiple binaries and logs the operations based on verbosity level.
func multipleRemove(binaries []string, installDir string, verbosityLevel Verbosity) error {
	for _, binaryName := range binaries {
		if err := removeBinary(binaryName, installDir, verbosityLevel); err != nil {
			return err
		}
	}
	return nil
}
