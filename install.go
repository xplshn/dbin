package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hedzr/progressbar"
	"github.com/hedzr/progressbar/cursor"
)

// installBinaries fetches multiple binaries concurrently, logging based on verbosity levels.
func installBinaries(ctx context.Context, config *Config, binaries []string, verbosityLevel Verbosity, metadata map[string]interface{}) error {
	cursor.Hide()
	defer cursor.Show()

	var wg sync.WaitGroup

	errChan := make(chan error, len(binaries))
	urls, checksums, err := findURL(config, binaries, verbosityLevel, metadata)
	if err != nil {
		return err
	}

	bar := progressbar.New()
	tasks := progressbar.NewTasks(bar)
	defer tasks.Close()

	var errors []string

	for i, binaryName := range binaries {
		wg.Add(1)
		url := urls[i]
		checksum := checksums[i]
		destination := filepath.Join(config.InstallDir, filepath.Base(url))

		tasks.Add(
			progressbar.WithTaskAddBarTitle(fmt.Sprintf("Downloading %s", binaryName)),
			progressbar.WithTaskAddOnTaskProgressing(func(bar progressbar.PB, exitCh <-chan struct{}) {
				defer wg.Done()
				// Fetch binary and place it at destination
				_, fetchErr := fetchBinaryFromURLToDest(ctx, bar, url, checksum, destination)
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
				if err := runIntegrationHooks(config, destination, verbosityLevel, metadata); err != nil {
					errChan <- fmt.Errorf("[%s] could not be handled by its default hooks: %v", binaryName, err)
					return
				}

				// Add full name to the binary's xattr
				if err := addFullName(destination, binaryName); err != nil {
					errChan <- fmt.Errorf("failed to add fullName property to the binary's xattr %s: %v", destination, err)
					return
				}
			}),
		)
	}

	go func() {
		tasks.Wait()
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
func runIntegrationHooks(config *Config, binaryPath string, verbosityLevel Verbosity, metadata map[string]interface{}) error {
	if config.UseIntegrationHooks {
		// Infer the file extension from the binaryPath
		ext := filepath.Ext(binaryPath)
		if hookCommands, exists := config.Hooks.Commands[ext]; exists {
			// Execute user-defined integration hooks
			for _, cmd := range hookCommands.IntegrationCommands {
				if err := executeHookCommand(config, cmd, binaryPath, ext, config.UseIntegrationHooks, verbosityLevel, metadata); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// installCommand installs one or more binaries based on the verbosity level.
func installCommand(config *Config, binaries []string, verbosityLevel Verbosity, metadata map[string]interface{}) error {
	return installBinaries(context.Background(), config, removeDuplicates(binaries), verbosityLevel, metadata)
}
