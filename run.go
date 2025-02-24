package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/urfave/cli/v3"
)

func runCommand() *cli.Command {
	return &cli.Command{
		Name:            "run",
		Usage:           "Run a specified binary from cache",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "transparent",
				Usage: "Run the binary from PATH if found",
			},
		},
		SkipFlagParsing: true,
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.NArg() == 0 {
				return fmt.Errorf("no binary name provided for run command")
			}

			config, err := loadConfig()
			if err != nil {
				return err
			}
			
			bEntry := stringToBinaryEntry(c.Args().First())
			return runFromCache(config, bEntry, c.Args().Tail(), c.Bool("transparent"), getVerbosityLevel(c))
		},
	}
}

func runFromCache(config *Config, bEntry binaryEntry, args []string, transparentMode bool, verbosityLevel Verbosity) error {
	// Try running from PATH if transparent mode is enabled
	if transparentMode {
		binaryPath, err := exec.LookPath(bEntry.Name)
		if err == nil {
			if verbosityLevel >= normalVerbosity {
				fmt.Printf("Running '%s' from PATH...\n", bEntry.Name)
			}
			return runBinary(binaryPath, args, verbosityLevel)
		}
	}

	// Check if the binary exists in cache and matches the requested version
	baseName := filepath.Base(bEntry.Name)
	cachedFile := filepath.Join(config.CacheDir, baseName)
	
	if fileExists(cachedFile) && isExecutable(cachedFile) {
		trackedBEntry, err := readEmbeddedBEntry(cachedFile)
		if err == nil && (trackedBEntry.PkgId == bEntry.PkgId || bEntry.PkgId == "") {
			if verbosityLevel >= normalVerbosity {
				fmt.Printf("Running '%s' from cache...\n", bEntry.Name)
			}
			if err := runBinary(cachedFile, args, verbosityLevel); err != nil {
				return err
			}
			return cleanCache(config.CacheDir, verbosityLevel)
		}
		
		if verbosityLevel >= normalVerbosity {
			fmt.Printf("Cached binary '%s' does not match requested binary '%s'. Fetching a new one...\n", 
				parseBinaryEntry(trackedBEntry, false), parseBinaryEntry(bEntry, false))
		}
	} else if verbosityLevel >= normalVerbosity {
		fmt.Printf("Couldn't find '%s' in the cache. Fetching a new one...\n", bEntry.Name)
	}

	// Fetch and install the binary
	cacheConfig := *config
	cacheConfig.UseIntegrationHooks = false
	cacheConfig.InstallDir = config.CacheDir
	
	uRepoIndex := fetchRepoIndex(&cacheConfig)
	if err := installBinaries(context.Background(), &cacheConfig, []binaryEntry{bEntry}, silentVerbosityWithErrors, uRepoIndex); err != nil {
		return err
	}

	if err := runBinary(cachedFile, args, verbosityLevel); err != nil {
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

		filesWithAtime = append(filesWithAtime, fileWithAtime{info: entry, atime: fileInfo.ModTime()})
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
		} else if verbosityLevel >= extraVerbose {
			fmt.Printf("Removed old cached binary: %s\n", filePath)
		}
	}

	return nil
}
