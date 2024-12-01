package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// update checks for updates to the valid programs and installs any that have changed.
func update(config *Config, programsToUpdate []string, verbosityLevel Verbosity, metadata map[string]interface{}) error {
	// Initialize counters
	var (
		skipped, updated, errors uint32
		checked                  uint32
		errorMessages            string
		errorMessagesMutex       sync.Mutex
	)

	// Call validateProgramsFrom with config and programsToUpdate
	programsToUpdate, err := validateProgramsFrom(config, programsToUpdate, metadata)
	if err != nil {
		return err
	}

	// Use a wait group to wait for all programs to finish updating
	var wg sync.WaitGroup

	// Separate slice to track programs that need updating
	var outdatedPrograms []string

	// Iterate over programsToUpdate and download/update each one concurrently
	installDir := config.InstallDir
	for _, program := range programsToUpdate {
		// Increment the WaitGroup counter
		wg.Add(1)

		// Launch a goroutine to update the program
		go func(program string) {
			defer wg.Done()

			installPath := filepath.Join(installDir, filepath.Base(program))

			if !fileExists(installPath) {
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				return
			}
			localB3sum, err := calculateChecksum(installPath)
			if err != nil {
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				return
			}

			binaryInfo, err := getBinaryInfo(config, program, metadata)
			if err != nil {
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				return
			}

			if binaryInfo.Bsum == "" {
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				return
			}

			if localB3sum != binaryInfo.Bsum {
				// Add to outdated programs for bulk installation
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&updated, 1)
				errorMessagesMutex.Lock()
				outdatedPrograms = append(outdatedPrograms, program)
				errorMessagesMutex.Unlock()
			} else {
				atomic.AddUint32(&checked, 1)
			}
		}(program)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Bulk install outdated programs
	if len(outdatedPrograms) > 0 {
		err := installCommand(config, outdatedPrograms, 1, metadata)
		if err != nil {
			atomic.AddUint32(&errors, 1)
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Printf("Failed to update programs: %v\n", outdatedPrograms)
			}
		}
	}

	// Print stats
	fmt.Printf("Skipped: %d\t Updated: %d\t Checked: %d\t Errors: %d\n",
		atomic.LoadUint32(&skipped),
		atomic.LoadUint32(&updated),
		atomic.LoadUint32(&checked),
		atomic.LoadUint32(&errors))

	// Print errors, if any
	if verbosityLevel >= silentVerbosityWithErrors && len(errorMessages) >= 1 {
		for _, errorMsg := range strings.Split(errorMessages, "\n") {
			fmt.Println(strings.TrimSpace(errorMsg))
		}
	}

	return nil
}
