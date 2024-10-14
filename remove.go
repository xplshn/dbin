package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// removeBinaries processes each binary, removing only those that pass validation.
func removeBinaries(config *Config, binaries []string, verbosityLevel Verbosity) error {
	var wg sync.WaitGroup
	var removeErrors []string
	var mutex sync.Mutex

	installDir := config.InstallDir

	// Loop over the binaries and remove the valid ones
	for _, binaryName := range binaries {
		wg.Add(1)
		go func(binaryName string) {
			defer wg.Done()

			installPath := filepath.Join(installDir, filepath.Base(binaryName))

			// Check if the binary exists before proceeding
			if !fileExists(installPath) {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Warning: '%s' does not exist in %s. Skipping removal.\n", binaryName, installDir)
				}
				return
			}

			// Get the full name of the binary installed
			fullBinaryName, err := getFullName(installPath)
			if err != nil {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Warning: Failed to retrieve full name for '%s'. Skipping removal.\n", binaryName)
				}
				return
			}

			// Compare the base name of the given binary and the full binary name
			if filepath.Base(binaryName) != filepath.Base(fullBinaryName) {
				// Use the base name of fullBinaryName for removal
				installPath = filepath.Join(installDir, filepath.Base(fullBinaryName))
			}

			// Validate if the binary was installed by checking the full binary name
			if fullBinaryName == "" {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Skipping '%s': it was not installed by dbin\n", binaryName)
				}
				return
			}

			// Run deintegration hooks before removing the binary
			if err := runDeintegrationHooks(config, installPath, verbosityLevel); err != nil {
				if verbosityLevel >= silentVerbosityWithErrors {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
				}
				// Add error to the list in a thread-safe way
				mutex.Lock()
				removeErrors = append(removeErrors, err.Error())
				mutex.Unlock()
				return
			}

			// Remove the binary
			err = os.Remove(installPath)
			if err != nil {
				if verbosityLevel >= silentVerbosityWithErrors {
					fmt.Fprintf(os.Stderr, "error: failed to remove '%s' from %s. %v\n", binaryName, installDir, err)
				}
				// Add error to the list in a thread-safe way
				mutex.Lock()
				removeErrors = append(removeErrors, fmt.Sprintf("failed to remove '%s' from %s: %v", binaryName, installDir, err))
				mutex.Unlock()
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

// runDeintegrationHooks runs the deintegration hooks for binaries which need to be deintegrated
func runDeintegrationHooks(config *Config, binaryPath string, verbosityLevel Verbosity) error {
	if config.IntegrateWithSystem {
		// Infer the file extension from the binaryPath
		ext := filepath.Ext(binaryPath)
		if hookCommands, exists := config.Hooks.Commands[ext]; exists {
			// Execute user-defined deintegration hooks
			for _, cmd := range hookCommands.DeintegrationCommands {
				if err := executeHookCommand(config, cmd, binaryPath, ext, config.IntegrateWithSystem, verbosityLevel); err != nil {
					return err
				}
			}
		} else {
			return fmt.Errorf("no deintegration commands found for extension: %s", ext)
		}
	}
	return nil
}
