package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
	"context"

	"github.com/urfave/cli/v3"
)

func runCommand() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Run a specified binary from cache",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "transparent",
				Usage: "Run the binary from PATH if found",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.NArg() == 0 {
				return fmt.Errorf("no binary name provided for run command")
			}

			config, err := loadConfig()
			if err != nil {
				return err
			}
			uRepoIndex := fetchRepoIndex(config)

			// The first argument is the binary name
			binaryName := c.Args().First()
			bEntry := stringToBinaryEntry(binaryName)

			// The rest of the arguments are passed to the binary
			args := c.Args().Tail()

			return runFromCache(config, bEntry, args, c.Bool("transparent"), getVerbosityLevel(c), uRepoIndex)
		},
	}
}

func returnCachedFile(config *Config, binaryName string) (cachedBinary string, trackedBEntry binaryEntry, err error) {
	cachedBinary = filepath.Join(config.CacheDir, filepath.Base(binaryName))

	trackedBEntry, err = readEmbeddedBEntry(cachedBinary)
	if err != nil {
		return "", trackedBEntry, err
	}

	if !fileExists(cachedBinary) {
		return "", trackedBEntry, fmt.Errorf("cached binary not found")
	}

	return cachedBinary, trackedBEntry, nil
}

func runFromCache(config *Config, bEntry binaryEntry, args []string, transparentMode bool, verbosityLevel Verbosity, uRepoIndex []binaryEntry) error {
	binaryPath, err := exec.LookPath(bEntry.Name)
	if err == nil && transparentMode {
		if verbosityLevel >= normalVerbosity {
			fmt.Printf("Running '%s' from PATH...\n", bEntry.Name)
		}
		return runBinary(binaryPath, args, verbosityLevel)
	}

	baseName := filepath.Base(bEntry.Name)
	cachedFile := filepath.Join(config.CacheDir, baseName)
	if fileExists(cachedFile) && isExecutable(cachedFile) {
		trackedBEntry, err := readEmbeddedBEntry(cachedFile)
		if err != nil || trackedBEntry.PkgId != bEntry.PkgId {
			if verbosityLevel >= normalVerbosity {
				if trackedBEntry.Name != "" {
					fmt.Printf("Cached binary '%s' does not match requested binary '%s'. Fetching a new one...\n", parseBinaryEntry(trackedBEntry, false), parseBinaryEntry(bEntry, false))
				}
			}

			config.UseIntegrationHooks = false
			config.InstallDir = config.CacheDir
			if err := installBinaries(context.Background(), config, []binaryEntry{bEntry}, silentVerbosityWithErrors, uRepoIndex); err != nil {
				if verbosityLevel >= silentVerbosityWithErrors {
					fmt.Fprintf(os.Stderr, "Error: could not fetch and cache the binary: %v\n", err)
				}
				return err
			}

			if err := runBinary(filepath.Join(config.CacheDir, baseName), args, verbosityLevel); err != nil {
				return err
			}
			return cleanCache(config.CacheDir, verbosityLevel)
		}

		if verbosityLevel >= normalVerbosity {
			fmt.Printf("Running '%s' from cache...\n", bEntry.Name)
		}
		if err := runBinary(filepath.Join(config.CacheDir, baseName), args, verbosityLevel); err != nil {
			return err
		}
		return cleanCache(config.CacheDir, verbosityLevel)
	}

	if verbosityLevel >= normalVerbosity {
		fmt.Printf("Couldn't find '%s' in the cache. Fetching a new one...\n", bEntry.Name)
	}

	config.UseIntegrationHooks = false
	config.InstallDir = config.CacheDir
	if err := installBinaries(context.Background(), config, []binaryEntry{bEntry}, silentVerbosityWithErrors, uRepoIndex); err != nil {
		if verbosityLevel >= silentVerbosityWithErrors {
			fmt.Fprintf(os.Stderr, "error: could not cache the binary: %v\n", err)
		}
		return err
	}

	if err := runBinary(filepath.Join(config.CacheDir, baseName), args, verbosityLevel); err != nil {
		return err
	}
	return cleanCache(config.CacheDir, verbosityLevel)
}

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
		return nil
	}

	type fileWithAtime struct {
		info  os.DirEntry
		atime time.Time
	}

	var filesWithAtime []fileWithAtime
	for _, entry := range files {
		filePath := filepath.Join(cacheDir, entry.Name())

		if !isExecutable(filePath) {
			continue
		}

		fileInfo, err := os.Stat(filePath)
		if err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "failed to read file info: %v\n", err)
			}
			continue
		}

		atime := fileInfo.ModTime()

		filesWithAtime = append(filesWithAtime, fileWithAtime{info: entry, atime: atime})
	}

	sort.Slice(filesWithAtime, func(i, j int) bool {
		return filesWithAtime[i].atime.Before(filesWithAtime[j].atime)
	})

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
