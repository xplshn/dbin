package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/urfave/cli/v3"
	"github.com/zeebo/errs"
)

var (
	errRemoveFailed = errs.Class("removal failed")
)

func removeCommand() *cli.Command {
	return &cli.Command{
		Name:    "remove",
		Aliases: []string{"del"},
		Usage:   "Remove binaries",
		Action: func(_ context.Context, c *cli.Command) error {
			config, err := loadConfig()
			if err != nil {
				return errRemoveFailed.Wrap(err)
			}
			return removeBinaries(config, arrStringToArrBinaryEntry(c.Args().Slice()))
		},
	}
}

func removeBinaries(config *config, bEntries []binaryEntry) error {
	var wg sync.WaitGroup
	var removeErrors []string
	var mutex sync.Mutex

	installDir := config.InstallDir

	for _, bEntry := range bEntries {
		wg.Add(1)
		go func(bEntry binaryEntry) {
			defer wg.Done()

			installPath := filepath.Join(installDir, filepath.Base(bEntry.Name))
			licensePath := filepath.Join(config.LicenseDir, fmt.Sprintf("%s_LICENSE", parseBinaryEntry(bEntry, false)))

			trackedBEntry, err := readEmbeddedBEntry(installPath)
			if err != nil {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Warning: Failed to retrieve full name for '%s'. Skipping removal because this program may not have been installed by dbin.\n", parseBinaryEntry(bEntry, true))
				}
				return
			}

			if filepath.Base(bEntry.Name) != filepath.Base(trackedBEntry.Name) {
				installPath = filepath.Join(installDir, filepath.Base(trackedBEntry.Name))
				licensePath = filepath.Join(config.LicenseDir, fmt.Sprintf("%s_LICENSE", filepath.Base(parseBinaryEntry(trackedBEntry, false))))
			}

			if !fileExists(installPath) {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Warning: '%s' does not exist in %s\n", bEntry.Name, installDir)
				}
				return
			}

			if trackedBEntry.PkgID == "" {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Skipping '%s': it was not installed by dbin\n", bEntry.Name)
				}
				return
			}

			if err := runDeintegrationHooks(config, installPath); err != nil {
				if verbosityLevel >= silentVerbosityWithErrors {
					fmt.Fprintf(os.Stderr, "%s\n", err)
				}
				mutex.Lock()
				removeErrors = append(removeErrors, err.Error())
				mutex.Unlock()
				return
			}

			err = os.Remove(installPath)
			if err != nil {
				if verbosityLevel >= silentVerbosityWithErrors {
					fmt.Fprintf(os.Stderr, "failed to remove '%s' from %s. %v\n", bEntry.Name, installDir, err)
				}
				mutex.Lock()
				removeErrors = append(removeErrors, fmt.Sprintf("failed to remove '%s' from %s: %v", bEntry.Name, installDir, err))
				mutex.Unlock()
			} else {
				if verbosityLevel >= silentVerbosityWithErrors {
					fmt.Printf("'%s' removed from %s\n", bEntry.Name, installDir)
				}
				// Remove corresponding license file if it exists
				if config.CreateLicenses && fileExists(licensePath) {
					if err := os.Remove(licensePath); err != nil {
						if verbosityLevel >= silentVerbosityWithErrors {
							fmt.Fprintf(os.Stderr, "Warning: Failed to remove license file %s: %v\n", licensePath, err)
						}
						// Non-fatal error
					} else if verbosityLevel >= normalVerbosity {
						fmt.Printf("Removed license file %s\n", licensePath)
					}
				}
			}
		}(bEntry)
	}

	wg.Wait()

	if len(removeErrors) > 0 {
		return errRemoveFailed.New(strings.Join(removeErrors, "\n"))
	}

	return nil
}

func runDeintegrationHooks(config *config, binaryPath string) error {
	if config.UseIntegrationHooks {
		ext := filepath.Ext(binaryPath)
		if hookCommands, exists := config.Hooks.Commands[ext]; exists {
			if err := executeHookCommand(config, &hookCommands, ext, binaryPath, false); err != nil {
				return errRemoveFailed.Wrap(err)
			}
		} else if hookCommands, exists := config.Hooks.Commands["*"]; exists {
			if err := executeHookCommand(config, &hookCommands, ext, binaryPath, false); err != nil {
				return errRemoveFailed.Wrap(err)
			}
		}
	}
	return nil
}
