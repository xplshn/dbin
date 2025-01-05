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
		padding                  = " "
	)

	// Call validateProgramsFrom with config and programsToUpdate
	programsToUpdate, err := validateProgramsFrom(config, programsToUpdate, metadata)
	if err != nil {
		return err
	}

	// Calculate toBeChecked
	toBeChecked := uint32(len(programsToUpdate))

	// Use a mutex for thread-safe updates to the progress
	var progressMutex sync.Mutex

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
			fullName, err := getFullName(installPath)

			if !fileExists(installPath) {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, "\033[2K\r<%d/%d> %s | Warning: Tried to update a non-existent program %s. Skipping.", atomic.LoadUint32(&checked), toBeChecked, padding, fullName)
				}
				progressMutex.Unlock()
				return
			}
			localB3sum, err := calculateChecksum(installPath)
			if err != nil {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, "\033[2K\r<%d/%d> %s | Warning: Failed to get B3sum for %s. Skipping.", atomic.LoadUint32(&checked), toBeChecked, padding, fullName)
				}
				progressMutex.Unlock()
				return
			}

			binaryInfo, err := getBinaryInfo(config, program, metadata)
			if err != nil {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, "\033[2K\r<%d/%d> %s | Warning: Failed to get metadata for %s. Skipping.", atomic.LoadUint32(&checked), toBeChecked, padding, fullName)
				}
				progressMutex.Unlock()
				return
			}

			if binaryInfo.Bsum == "" {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, "\033[2K\r<%d/%d> %s | Skipping %s because the B3sum field is null.", atomic.LoadUint32(&checked), toBeChecked, padding, fullName)
				}
				progressMutex.Unlock()
				return
			}

			if localB3sum != binaryInfo.Bsum {
				// Add to outdated programs for bulk installation
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&updated, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, "\033[2K\r<%d/%d> %s | %s is outdated and will be updated.", atomic.LoadUint32(&checked), toBeChecked, padding, fullName)
				}
				errorMessagesMutex.Lock()
				outdatedPrograms = append(outdatedPrograms, program)
				errorMessagesMutex.Unlock()
				progressMutex.Unlock()
			} else {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, "\033[2K\r<%d/%d> %s | No updates available for %s.", atomic.LoadUint32(&checked), toBeChecked, padding, fullName)
				}
				progressMutex.Unlock()
			}
		}(program)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Bulk install outdated programs
	if len(outdatedPrograms) > 0 {
		fmt.Print("\033[2K\r") // Clear up any prior messages on this same line, prior to triggering installCommand()
		err := installCommand(config, outdatedPrograms, 1, metadata)
		if err != nil {
			atomic.AddUint32(&errors, 1)
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Printf("Failed to update programs: %v\n", outdatedPrograms)
			}
		}
	}

	// Prepare final counts
	finalCounts := fmt.Sprintf("\033[2K\rSkipped: %d\tUpdated: %d\tChecked: %d", atomic.LoadUint32(&skipped), atomic.LoadUint32(&updated), uint32(int(atomic.LoadUint32(&checked))))
	if errors > 0 && verbosityLevel >= silentVerbosityWithErrors {
		finalCounts += fmt.Sprintf("\tErrors: %d", atomic.LoadUint32(&errors))
	}

	// Print final counts
	if verbosityLevel >= normalVerbosity || (errors > 0 && verbosityLevel >= silentVerbosityWithErrors) {
		fmt.Printf(finalCounts)
		for _, errorMsg := range strings.Split(errorMessages, "\n") {
			fmt.Println(strings.TrimSpace(errorMsg))
		}
	}

	return nil
}
