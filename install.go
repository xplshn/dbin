package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// installBinaries fetches multiple binaries concurrently, logging based on verbosity levels.
func installBinaries(ctx context.Context, config *Config, binaries []string, verbosityLevel Verbosity) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(binaries))
	urls, checksums, err := findURL(config, binaries, verbosityLevel)
	if err != nil {
		return err
	}

	var errors []string

	for i, binaryName := range binaries {
		wg.Add(1)
		go func(i int, binaryName string) {
			defer wg.Done()
			url := urls[i]
			checksum := checksums[i]
			destination := filepath.Join(config.InstallDir, filepath.Base(binaryName))

			if err != nil {
				errChan <- fmt.Errorf("[%s] could not be handled by its default hooks: %v", binaryName, err)
				return
			}

			// Fetch binary and place it at destination
			_, fetchErr := fetchBinaryFromURLToDest(ctx, url, checksum, destination)
			if fetchErr != nil {
				errChan <- fmt.Errorf("error fetching binary %s: %v", binaryName, fetchErr)
				return
			}

			// Make the binary executable
			if err := os.Chmod(destination, 0755); err != nil {
				errChan <- fmt.Errorf("error making binary executable %s: %v", destination, err)
				return
			}

			// Run hooks after the file is downloaded and chmod +x
			if err := runIntegrationHooks(config, destination, verbosityLevel); err != nil {
				errChan <- err
				return
			}

			// Add full name to the binary's xattr
			if err := addFullName(destination, binaryName); err != nil {
				errChan <- fmt.Errorf("failed to add fullName property to the binary's xattr %s: %v", destination, err)
				return
			}
		}(i, binaryName)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	// Collect errors
	for err := range errChan {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		finalErr := strings.Join(errors, "\n")
		if verbosityLevel >= silentVerbosityWithErrors {
			return fmt.Errorf(finalErr)
		}
	}

	return nil
}

// runIntegrationHooks runs the integration hooks for binaries which need to be integrated
func runIntegrationHooks(config *Config, binaryPath string, verbosityLevel Verbosity) error {
	if config.UseIntegrationHooks {
		// Infer the file extension from the binaryPath
		ext := filepath.Ext(binaryPath)
		if hookCommands, exists := config.Hooks.Commands[ext]; exists {
			// Execute user-defined integration hooks
			for _, cmd := range hookCommands.IntegrationCommands {
				if err := executeHookCommand(config, cmd, binaryPath, ext, config.UseIntegrationHooks, verbosityLevel); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// installCommand installs one or more binaries based on the verbosity level.
func installCommand(config *Config, binaries []string, verbosityLevel Verbosity) error {
	return installBinaries(context.Background(), config, removeDuplicates(binaries), verbosityLevel)
}
