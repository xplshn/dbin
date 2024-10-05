// update.go // This file holds the implementation for the "update" functionality - (parallel) //>
package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/zeebo/blake3"
)

// update checks for updates to the valid programs and installs any that have changed.
func update(programsToUpdate []string, installDir, trackerFile string, verbosityLevel Verbosity, repositories, metadataURLs []string) error {
	// Initialize counters
	var (
		skipped, updated, errors, toBeChecked uint32
		checked                               uint32
		errorMessages                         string
		padding                               = " "
	)

	// Call validateProgramsFrom with InstallDir and programsToUpdate
	programsToUpdate, err := validateProgramsFrom(installDir, trackerFile, metadataURLs, programsToUpdate)
	if err != nil {
		return err
	}

	// Calculate toBeChecked
	toBeChecked = uint32(len(programsToUpdate))

	// Use a mutex for thread-safe updates to the progress
	var progressMutex sync.Mutex

	// Use a wait group to wait for all programs to finish updating
	var wg sync.WaitGroup

	// Iterate over programsToUpdate and download/update each one concurrently
	for _, program := range programsToUpdate {
		// Increment the WaitGroup counter
		wg.Add(1)

		// Launch a goroutine to update the program
		go func(program string) {
			defer wg.Done()

			installPath := filepath.Join(installDir, filepath.Base(program))
			if !fileExists(installPath) {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, false, "\033[2K\r<%d/%d> %s | Warning: Tried to update a non-existent program %s. Skipping.", atomic.LoadUint32(&checked), toBeChecked, padding, program)
				}
				progressMutex.Unlock()
				return
			}
			localB3sum, err := getLocalB3sum(installPath)
			if err != nil {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, false, "\033[2K\r<%d/%d> %s | Warning: Failed to get B3sum for %s. Skipping.", atomic.LoadUint32(&checked), toBeChecked, padding, program)
				}
				progressMutex.Unlock()
				return
			}

			binaryInfo, err := getBinaryInfo(trackerFile, program, metadataURLs)
			if err != nil {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, false, "\033[2K\r<%d/%d> %s | Warning: Failed to get metadata for %s. Skipping.", atomic.LoadUint32(&checked), toBeChecked, padding, program)
				}
				progressMutex.Unlock()
				return
			}

			if binaryInfo.B3sum == "" {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, false, "\033[2K\r<%d/%d> %s | Skipping %s because the B3sum field is null.", atomic.LoadUint32(&checked), toBeChecked, padding, program)
				}
				progressMutex.Unlock()
				return
			}

			if checkDifferences(localB3sum, binaryInfo.B3sum) == 1 {
				err := installCommand([]string{program}, installDir, trackerFile, verbosityLevel, repositories, metadataURLs)
				if err != nil {
					progressMutex.Lock()
					atomic.AddUint32(&errors, 1)
					if verbosityLevel >= silentVerbosityWithErrors {
						errorMessages += fmt.Sprintf("Failed to update '%s', please check this file's properties, etc.\n", program)
					}
					progressMutex.Unlock()
					return
				}
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&updated, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, false, "\033[2K\r<%d/%d> %s | Successfully updated %s.", atomic.LoadUint32(&checked), toBeChecked, padding, program)
				}
				progressMutex.Unlock()
			} else {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, false, "\033[2K\r<%d/%d> %s | No updates available for %s.", atomic.LoadUint32(&checked), toBeChecked, padding, program)
				}
				progressMutex.Unlock()
			}
		}(program)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Prepare final counts
	finalCounts := fmt.Sprintf("\033[2K\rSkipped: %d\tUpdated: %d\tChecked: %d", atomic.LoadUint32(&skipped), atomic.LoadUint32(&updated), uint32(int(atomic.LoadUint32(&checked))))
	if errors > 0 && verbosityLevel >= silentVerbosityWithErrors {
		finalCounts += fmt.Sprintf("\tErrors: %d", atomic.LoadUint32(&errors))
	}
	// Print final counts
	if verbosityLevel >= normalVerbosity || (errors > 0 && verbosityLevel >= silentVerbosityWithErrors) {
		fmt.Printf(finalCounts)
		for _, error := range strings.Split(errorMessages, "\n") {
			fmt.Println(strings.TrimSpace(error))
		}
	}

	return nil
}

// getLocalB3sum calculates the B3sum checksum of the local file.
func getLocalB3sum(filePath string) (checksum string, err error) {
	// Open the file for reading
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %v", filePath, err)
	}
	// Ensure the file is closed when the function returns
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close file %s: %v", filePath, cerr)
		}
	}()

	// Calculate B3sum checksum
	hasher := blake3.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to compute B3SUM of %s: %v", filePath, err)
	}

	// Return checksum as a hex string
	checksum = hex.EncodeToString(hasher.Sum(nil))
	return checksum, nil
}

// checkDifferences compares local and remote B3sum checksums.
func checkDifferences(localB3sum, remoteB3sum string) int {
	if localB3sum != remoteB3sum {
		return 1
	}
	return 0
}
