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

			// Try to find the binary by name or by matching user.FullName
			binaryPath, trackedBEntry, err := findBinaryByNameOrFullName(installDir, bEntry.Name)
			if err != nil {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Warning: '%s' does not exist or was not installed by dbin: %v\n", bEntry.Name, err)
				}
				return
			}

			licensePath := filepath.Join(config.LicenseDir, fmt.Sprintf("%s_LICENSE", filepath.Base(parseBinaryEntry(trackedBEntry, false))))

			if !fileExists(binaryPath) {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Warning: '%s' does not exist in %s\n", bEntry.Name, installDir)
				}
				return
			}

			if trackedBEntry.PkgID == "" {
				if verbosityLevel >= normalVerbosity {
					fmt.Fprintf(os.Stderr, "Warning: '%s' was not installed by dbin\n", bEntry.Name)
				}
				return
			}

			if err := runDeintegrationHooks(config, binaryPath); err != nil {
				if verbosityLevel >= silentVerbosityWithErrors {
					fmt.Fprintf(os.Stderr, "Error running deintegration hooks for '%s': %v\n", bEntry.Name, err)
				}
				mutex.Lock()
				removeErrors = append(removeErrors, err.Error())
				mutex.Unlock()
				return
			}

			err = os.Remove(binaryPath)
			if err != nil {
				if verbosityLevel >= silentVerbosityWithErrors {
					fmt.Fprintf(os.Stderr, "Failed to remove '%s' from %s: %v\n", bEntry.Name, installDir, err)
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

// findBinaryByNameOrFullName searches for a binary in installDir by its name or by matching the user.FullName xattr.
func findBinaryByNameOrFullName(installDir, name string) (string, binaryEntry, error) {
	// First, try direct path
	binaryPath := filepath.Join(installDir, filepath.Base(name))
	if fileExists(binaryPath) {
		trackedBEntry, err := readEmbeddedBEntry(binaryPath)
		if err == nil && trackedBEntry.Name != "" {
			return binaryPath, trackedBEntry, nil
		}
	}

	// If direct path fails, scan directory for matching user.FullName
	entries, err := os.ReadDir(installDir)
	if err != nil {
		return "", binaryEntry{}, errFileAccess.Wrap(err)
	}

	// Normalize the input name for comparison
	inputBEntry := stringToBinaryEntry(name)
	inputFullName := parseBinaryEntry(inputBEntry, false)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		binaryPath = filepath.Join(installDir, entry.Name())
		if !isExecutable(binaryPath) || isSymlink(binaryPath) {
			continue
		}

		trackedBEntry, err := readEmbeddedBEntry(binaryPath)
		if err != nil || trackedBEntry.Name == "" {
			continue
		}

		trackedFullName := parseBinaryEntry(trackedBEntry, false)
		if trackedFullName == inputFullName || trackedBEntry.Name == filepath.Base(name) {
			return binaryPath, trackedBEntry, nil
		}
	}

	return "", binaryEntry{}, errFileNotFound.New("binary '%s' not found in %s", name, installDir)
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
