package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"context"

	"github.com/urfave/cli/v3"
)

func removeCommand() *cli.Command {
	return &cli.Command{
		Name:    "remove",
		Aliases: []string{"del"},
		Usage:   "Remove binaries",
		Action: func(ctx context.Context, c *cli.Command) error {
			config, err := loadConfig()
			if err != nil {
				return err
			}
			uRepoIndex, err := fetchRepoIndex(config)
			if err != nil {
				return err
			}
			return removeBinaries(config, arrStringToArrBinaryEntry(c.Args().Slice()), getVerbosityLevel(c), uRepoIndex)
		},
	}
}

func removeBinaries(config *Config, bEntries []binaryEntry, verbosityLevel Verbosity, uRepoIndex []binaryEntry) error {
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
					fmt.Fprintf(os.Stderr, "Warning: Failed to retrieve full name for '%s#%s'. Skipping removal.\n", bEntry.Name, bEntry.PkgId)
				}
				return
			}

			if filepath.Base(bEntry.Name) != filepath.Base(trackedBEntry.Name) {
				installPath = filepath.Join(installDir, filepath.Base(trackedBEntry.Name))
			}

			if !fileExists(installPath) {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Warning: '%s' does not exist in %s. Skipping removal.\n", bEntry.Name, installDir)
				}
				return
			}

			if trackedBEntry.PkgId == "" {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Skipping '%s': it was not installed by dbin\n", bEntry.Name)
				}
				return
			}

			if err := runDeintegrationHooks(config, installPath, verbosityLevel, uRepoIndex); err != nil {
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
					fmt.Fprintf(os.Stderr, "error: failed to remove '%s' from %s. %v\n", bEntry.Name, installDir, err)
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
		return fmt.Errorf(strings.Join(removeErrors, "\n"))
	}

	return nil
}

func runDeintegrationHooks(config *Config, binaryPath string, verbosityLevel Verbosity, uRepoIndex []binaryEntry) error {
	if config.UseIntegrationHooks {
		ext := filepath.Ext(binaryPath)
		if hookCommands, exists := config.Hooks.Commands[ext]; exists {
			for _, cmd := range hookCommands.DeintegrationCommands {
				if err := executeHookCommand(config, cmd, binaryPath, ext, false, verbosityLevel, uRepoIndex); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
