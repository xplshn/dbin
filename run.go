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

// ReturnCachedFile retrieves the cached file location and its corresponding fullName. Returns an empty string and an error if not found.
func ReturnCachedFile(config *Config, binaryName string) (cachedBinary string, trackedBinaryName string, err error) {
	cachedBinary = filepath.Join(config.CacheDir, filepath.Base(binaryName))

	// Retrieve the fullName of the cachedBinary
	trackedBinaryName, err = getFullName(cachedBinary)
	if err != nil {
		return "", "", err
	}

	// Check if the cached binary exists
	if !fileExists(cachedBinary) {
		return "", "", errors.New("cached binary not found")
	}

	return cachedBinary, trackedBinaryName, nil
}

// RunFromCache runs the binary from cache or fetches it if not found
func RunFromCache(config *Config, binaryName string, args []string, transparentMode bool, verbosityLevel Verbosity, metadata map[string]interface{}) error {
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
	cachedFile := filepath.Join(config.CacheDir, baseName)
	if fileExists(cachedFile) && isExecutable(cachedFile) {
		// Verify that the cached binary corresponds to the correct binary by checking the fullName
		trackedBinaryName, err := getFullName(cachedFile)
		if err != nil || trackedBinaryName != binaryName {
			// If the cached binary is different, log and re-fetch
			if verbosityLevel >= normalVerbosity {
				if trackedBinaryName != "" {
					fmt.Printf("Cached binary '%s' does not match requested binary '%s'. Fetching a new one...\n", trackedBinaryName, binaryName)
				}
			}

			// Fetch the correct binary
			config.UseIntegrationHooks = false
			config.InstallDir = config.CacheDir
			if err := installCommand(config, []string{binaryName}, silentVerbosityWithErrors, metadata); err != nil {
				if verbosityLevel >= silentVerbosityWithErrors {
					fmt.Fprintf(os.Stderr, "Error: could not fetch and cache the binary: %v\n", err)
				}
				return err
			}

			// Run the newly fetched binary
			if err := runBinary(filepath.Join(config.CacheDir, baseName), args, verbosityLevel); err != nil {
				return err
			}
			return cleanCache(config.CacheDir, verbosityLevel)
		}

		// Run the binary from cache if fullName matches
		if verbosityLevel >= normalVerbosity {
			fmt.Printf("Running '%s' from cache...\n", binaryName)
		}
		if err := runBinary(filepath.Join(config.CacheDir, baseName), args, verbosityLevel); err != nil {
			return err
		}
		return cleanCache(config.CacheDir, verbosityLevel)
	}

	if verbosityLevel >= normalVerbosity {
		fmt.Printf("Couldn't find '%s' in the cache. Fetching a new one...\n", binaryName)
	}

	// Fetch the binary if it doesn't exist in the cache
	config.UseIntegrationHooks = false
	config.InstallDir = config.CacheDir
	if err := installCommand(config, []string{binaryName}, silentVerbosityWithErrors, metadata); err != nil {
		if verbosityLevel >= silentVerbosityWithErrors {
			fmt.Fprintf(os.Stderr, "error: could not cache the binary: %v\n", err)
		}
		return err
	}

	// Run the freshly fetched binary
	if err := runBinary(filepath.Join(config.CacheDir, baseName), args, verbosityLevel); err != nil {
		return err
	}
	return cleanCache(config.CacheDir, verbosityLevel)
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

func cleanCache(cacheDir string, verbosityLevel Verbosity) error {
	files, err := os.ReadDir(cacheDir)
	if err != nil {
		return fmt.Errorf("error reading cache directory, cannot proceed with cleanup: %v", err)
	}

	if len(files) <= maxCacheSize {
		// Cache size is within the limit, no need to remove binaries
		return nil
	}

	type fileWithAtime struct {
		info  os.DirEntry
		atime time.Time
	}

	var filesWithAtime []fileWithAtime
	for _, entry := range files {
		filePath := filepath.Join(cacheDir, entry.Name())

		// Check if the file is executable
		if !isExecutable(filePath) {
			continue // Skip this file if it's not executable
		}

		fileInfo, err := os.Stat(filePath)
		if err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "failed to read file info: %v\n", err)
			}
			continue
		}

		// Use ModTime as a substitute for access time (atime) since atime is not always supported
		atime := fileInfo.ModTime()

		filesWithAtime = append(filesWithAtime, fileWithAtime{info: entry, atime: atime})
	}

	// Sort files by access time, oldest first
	sort.Slice(filesWithAtime, func(i, j int) bool {
		return filesWithAtime[i].atime.Before(filesWithAtime[j].atime)
	})

	// Remove the oldest executable binaries until cache size is within the limit
	for i := 0; i < binariesToDelete && i < len(filesWithAtime); i++ {
		filePath := filepath.Join(cacheDir, filesWithAtime[i].info.Name())
		if err := os.Remove(filePath); err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "error removing old cached binary: %v\n", err)
			}
		} else {
			if verbosityLevel >= extraVerbose {
				fmt.Printf("Removed old cached binary: %s\n", filePath)
			}
		}
	}

	return nil
}
