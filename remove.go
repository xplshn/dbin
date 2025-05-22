package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"context"

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
		Action: func(ctx context.Context, c *cli.Command) error {
			config, err := loadConfig()
			if err != nil {
				return errRemoveFailed.Wrap(err)
			}
			return removeBinaries(config, arrStringToArrBinaryEntry(c.Args().Slice()))
		},
	}
}

func removeBinaries(config *Config, bEntries []binaryEntry) error {
	var wg sync.WaitGroup
	var removeErrors []string
	var mutex sync.Mutex

	installDir := config.InstallDir

	for _, bEntry := range bEntries {
		wg.Add(1)
		go func(bEntry binaryEntry) {
			defer wg.Done()

			installPath := filepath.Join(installDir, filepath.Base(bEntry.Name))

			trackedBEntry, err := readEmbeddedBEntry(installPath)
			if err != nil {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Warning: Failed to retrieve full name for '%s'. Skipping removal because this program may not have been installed by dbin.\n", parseBinaryEntry(bEntry, true))
				}
				return
			}

			if filepath.Base(bEntry.Name) != filepath.Base(trackedBEntry.Name) {
				installPath = filepath.Join(installDir, filepath.Base(trackedBEntry.Name))
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
			} else if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Printf("'%s' removed from %s\n", bEntry.Name, installDir)
			}
		}(bEntry)
	}

	wg.Wait()

	if len(removeErrors) > 0 {
		return errRemoveFailed.New(strings.Join(removeErrors, "\n"))
	}

	return nil
}

func runDeintegrationHooks(config *Config, binaryPath string) error {
	if config.UseIntegrationHooks {
		ext := filepath.Ext(binaryPath)
		if hookCommands, exists := config.Hooks.Commands[ext]; exists {
			if err := executeHookCommand(config, hookCommands.DeintegrationCommand, binaryPath, ext, false); err != nil {
				return errRemoveFailed.Wrap(err)
			}
		}
	}
	return nil
}
