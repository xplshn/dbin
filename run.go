// run.go // This file implements the "run" functionality
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ReturnCachedFile retrieves the cached file location and its corresponding tracked binary name. Returns an empty string and an error if not found.
func ReturnCachedFile(tempDir, binaryName, trackerFile string) (cachedBinary string, trackedBinaryName string) {
	baseName := filepath.Base(binaryName)
	cachedBinary = filepath.Join(tempDir, baseName)

	// Retrieve the tracked binary name from the tracker file
	trackedBinaryName, err := getBinaryNameFromTrackerFile(trackerFile, filepath.Base(binaryName))
	if err != nil {
		trackedBinaryName = ""
	}

	// Check if the cached binary exists
	if !fileExists(cachedBinary) {
		cachedBinary = ""
	}

	// Return empty strings if the cached binary is not found
	return cachedBinary, trackedBinaryName
}

// RunFromCache runs the binary from cache or fetches it if not found
func RunFromCache(binaryName string, args []string, tempDir, trackerFile string, transparentMode bool, verbosityLevel Verbosity, repositories, metadataURLs []string) error {
	flagsAndBinaryName := append(strings.Fields(binaryName), args...)
	flag.CommandLine.Parse(flagsAndBinaryName)

	if binaryName == "" {
		return errors.New("binary name not provided")
	}

	binaryPath, err := exec.LookPath(binaryName)
	if err == nil && transparentMode {
		if verbosityLevel >= normalVerbosity {
			fmt.Printf("Running '%s' from PATH...\n", binaryName)
		}
		return runBinary(binaryPath, args, verbosityLevel)
	}

	// Extract the base name of the binary
	baseName := filepath.Base(binaryName)

	// Check if the binary exists in the cache
	cachedFile := filepath.Join(tempDir, baseName)
	if fileExists(cachedFile) && isExecutable(cachedFile) {
		// Verify that the cached binary corresponds to the correct directory by checking the tracker file
		trackedBinaryName, err := getBinaryNameFromTrackerFile(trackerFile, baseName)
		if err != nil || trackedBinaryName != binaryName {
			// If the binary in the cache is from a different directory, log and re-fetch
			if verbosityLevel >= normalVerbosity {
				if trackedBinaryName != "" {
					fmt.Printf("The cached binary '%s' does not match the requested binary '%s'. Fetching the correct one...\n", trackedBinaryName, binaryName)
				}
			}

			// Fetch the correct binary
			if err := installCommand([]string{binaryName}, tempDir, trackerFile, silentVerbosityWithErrors, repositories, metadataURLs); err != nil {
				if verbosityLevel >= silentVerbosityWithErrors {
					fmt.Fprintf(os.Stderr, "error: could not cache the binary: %v\n", err)
				}
				return err
			}

			// Run the freshly fetched binary
			if err := runBinary(filepath.Join(tempDir, baseName), args, verbosityLevel); err != nil {
				return err
			}
			return cleanCache(tempDir, trackerFile, verbosityLevel)
		}
		// Run the binary from the cache if it matches the requested binary
		if verbosityLevel >= normalVerbosity {
			fmt.Printf("Running '%s' from cache...\n", binaryName)
		}
		if err := runBinary(filepath.Join(tempDir, baseName), args, verbosityLevel); err != nil {
			return err
		}
		return cleanCache(tempDir, trackerFile, verbosityLevel)
	}

	if verbosityLevel >= normalVerbosity {
		fmt.Printf("Couldn't find '%s' in the cache. Fetching a new one...\n", binaryName)
	}

	// Fetch the binary if it doesn't exist in the cache
	if err := installCommand([]string{binaryName}, tempDir, trackerFile, silentVerbosityWithErrors, repositories, metadataURLs); err != nil {
		if verbosityLevel >= silentVerbosityWithErrors {
			fmt.Fprintf(os.Stderr, "error: could not cache the binary: %v\n", err)
		}
		return err
	}

	// Run the freshly fetched binary
	if err := runBinary(filepath.Join(tempDir, baseName), args, verbosityLevel); err != nil {
		return err
	}
	return cleanCache(tempDir, trackerFile, verbosityLevel)
}

// runBinary executes the binary with the given arguments.
func runBinary(binaryPath string, args []string, verbosityLevel Verbosity) error {
	cmd := exec.Command(binaryPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil && verbosityLevel == extraVerbose {
		fmt.Printf("The program (%s) errored out with a non-zero exit code (%d).\n", binaryPath, cmd.ProcessState.ExitCode())
	}
	return err
}

// cleanCache removes old or untracked binaries from the cache directory.
func cleanCache(tempDir, trackerFile string, verbosityLevel Verbosity) error {
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("error reading cache directory, cannot proceed with cleanup: %v", err)
	}

	// Read the tracker file to get the list of tracked binaries
	tracker, err := readTrackerFile(trackerFile)
	if err != nil {
		return fmt.Errorf("could not read tracker file: %w", err)
	}

	// Print the contents of the tracker file for debugging
	if verbosityLevel >= extraVerbose {
		fmt.Printf("Tracker file contents: %v\n", tracker)
	}

	// Remove untracked files
	for _, entry := range files {
		filePath := filepath.Join(tempDir, entry.Name())

		// Check if the file is executable
		if !isExecutable(filePath) {
			continue // Skip this file if it's not executable
		}

		// Check if the file is in the tracker file
		baseName := entry.Name()
		if _, exists := tracker[baseName]; !exists {
			// Remove the file if it is not in the tracker file
			if err := os.Remove(filePath); err != nil {
				if verbosityLevel >= silentVerbosityWithErrors {
					fmt.Fprintf(os.Stderr, "error removing untracked cached binary file: %v\n", err)
				}
			} else {
				if verbosityLevel >= extraVerbose {
					fmt.Printf("Removed untracked cached executable: %s\n", filePath)
				}
			}
		}
	}

	// Check if the cache size exceeds the limit
	files, err = os.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("error reading cache directory, cannot proceed with cleanup: %v", err)
	}

	if len(files) <= maxCacheSize {
		return nil
	}

	type fileWithAtime struct {
		info  os.DirEntry
		atime time.Time
	}

	var filesWithAtime []fileWithAtime
	for _, entry := range files {
		filePath := filepath.Join(tempDir, entry.Name())

		// Check if the file is executable
		if !isExecutable(filePath) {
			continue // Skip this file if it's not executable
		}

		fileInfo, err := os.Stat(filePath)
		if err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "failed to read atime of old cached file: %v\n", err)
			}
			continue
		}

		// Get the access time
		atime := fileInfo.ModTime() // Use ModTime() as ATIME is not directly available

		filesWithAtime = append(filesWithAtime, fileWithAtime{info: entry, atime: atime})
	}

	// Sort files by access time
	sort.Slice(filesWithAtime, func(i, j int) bool {
		return filesWithAtime[i].atime.Before(filesWithAtime[j].atime)
	})

	// Remove the oldest executable binaries
	for i := 0; i < binariesToDelete && i < len(filesWithAtime); i++ {
		filePath := filepath.Join(tempDir, filesWithAtime[i].info.Name())
		if err := os.Remove(filePath); err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "error removing old cached binary file: %v\n", err)
			}
		} else {
			if verbosityLevel >= extraVerbose {
				fmt.Printf("Removed old cached executable: %s\n", filePath)
			}
		}
	}

	cleanupTrackerFile(trackerFile, tempDir)
	return nil
}
