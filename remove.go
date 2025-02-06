package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func removeBinaries(config *Config, binaries []string, verbosityLevel Verbosity, metadata map[string]interface{}) error {
	var wg sync.WaitGroup
	var removeErrors []string
	var mutex sync.Mutex

	installDir := config.InstallDir

	for _, binaryName := range binaries {
		wg.Add(1)
		go func(binaryName string) {
			defer wg.Done()

			installPath := filepath.Join(installDir, filepath.Base(binaryName))

			fullBinaryName, err := getFullName(installPath)
			if err != nil {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Warning: Failed to retrieve full name for '%s'. Skipping removal.\n", binaryName)
				}
				return
			}

			if filepath.Base(binaryName) != filepath.Base(fullBinaryName) {
				installPath = filepath.Join(installDir, filepath.Base(fullBinaryName))
			}

			parts := strings.SplitN(installPath, "#", 2)
			installPath = parts[0]

			if !fileExists(installPath) {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Warning: '%s' does not exist in %s. Skipping removal.\n", binaryName, installDir)
				}
				return
			}

			if fullBinaryName == "" {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Skipping '%s': it was not installed by dbin\n", binaryName)
				}
				return
			}

			if err := runDeintegrationHooks(config, installPath, verbosityLevel, metadata); err != nil {
				if verbosityLevel >= silentVerbosityWithErrors {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
				}
				mutex.Lock()
				removeErrors = append(removeErrors, err.Error())
				mutex.Unlock()
				return
			}

			err = os.Remove(installPath)
			if err != nil {
				if verbosityLevel >= silentVerbosityWithErrors {
					fmt.Fprintf(os.Stderr, "error: failed to remove '%s' from %s. %v\n", binaryName, installDir, err)
				}
				mutex.Lock()
				removeErrors = append(removeErrors, fmt.Sprintf("failed to remove '%s' from %s: %v", binaryName, installDir, err))
				mutex.Unlock()
			} else if verbosityLevel <= extraVerbose {
				fmt.Printf("'%s' removed from %s\n", binaryName, installDir)
			}
		}(binaryName)
	}

	wg.Wait()

	if len(removeErrors) > 0 {
		return fmt.Errorf(strings.Join(removeErrors, "\n"))
	}

	return nil
}

func runDeintegrationHooks(config *Config, binaryPath string, verbosityLevel Verbosity, metadata map[string]interface{}) error {
	if config.UseIntegrationHooks {
		ext := filepath.Ext(binaryPath)
		if hookCommands, exists := config.Hooks.Commands[ext]; exists {
			for _, cmd := range hookCommands.DeintegrationCommands {
				if err := executeHookCommand(config, cmd, binaryPath, ext, config.UseIntegrationHooks, verbosityLevel, metadata); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
