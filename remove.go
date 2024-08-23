// remove.go // This file implements the functionality of "remove" //>
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func removeCommand(binaries []string, installDir, trackerFile string, silent Silent) error {
	if len(binaries) == 1 {
		err := removeBinary(binaries[0], installDir, silent)
		if err != nil {
			return err
		}
	} else if len(binaries) > 1 {
		// Remove duplicates before processing
		binaries = removeDuplicates(binaries)
		err := multipleRemove(binaries, installDir, silent)
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

func removeBinary(binaryName, installDir string, silent Silent) error {
	installPath := filepath.Join(installDir, filepath.Base(binaryName))
	err := os.Remove(installPath)
	if err != nil {
		if os.IsNotExist(err) {
			if silent != disabledVerbosity {
				fmt.Printf("Warning: '%s' does not exist in %s\n", binaryName, installDir)
			}
		} else {
			if silent != disabledVerbosity {
				fmt.Printf("Error: Failed to remove '%s' from %s. %v\n", binaryName, installDir, err)
			}
			return err
		}
	} else if silent == normalVerbosity {
		fmt.Printf("'%s' removed from %s\n", binaryName, installDir)
	}
	return nil
}

func multipleRemove(binaries []string, installDir string, silent Silent) error {
	for _, binaryName := range binaries {
		if err := removeBinary(binaryName, installDir, silent); err != nil {
			return err
		}
	}
	return nil
}
