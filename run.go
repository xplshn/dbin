package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
	"strings"

	"github.com/urfave/cli/v3"
	"github.com/zeebo/errs"
)

var (
	errRunFailed = errs.Class("run failed")
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
				return errRunFailed.New("no binary name provided for run command")
			}

			config, err := loadConfig()
			if err != nil {
				return errRunFailed.Wrap(err)
			}

			bEntry := stringToBinaryEntry(c.Args().First())
			return runFromCache(config, bEntry, c.Args().Tail(), c.Bool("transparent"), nil)
		},
	}
}

func runFromCache(config *Config, bEntry binaryEntry, args []string, transparentMode bool, env []string) error {
	if transparentMode {
		binaryPath, err := exec.LookPath(bEntry.Name)
		if err == nil {
			if verbosityLevel >= normalVerbosity {
				fmt.Printf("Running '%s' from PATH...\n", bEntry.Name)
			}
			return runBinary(binaryPath, args, env)
		}
	}

	cachedFile, err := isCached(config, bEntry)
	if err == nil {
		if verbosityLevel >= normalVerbosity {
			fmt.Printf("Running '%s' from cache...\n", parseBinaryEntry(bEntry, true))
		}
		if err := runBinary(cachedFile, args, env); err != nil {
			return errRunFailed.Wrap(err)
		}
		return cleanCache(config.CacheDir)
	}

	if verbosityLevel >= normalVerbosity {
		fmt.Printf("Couldn't find '%s' in the cache. Fetching a new one...\n", parseBinaryEntry(bEntry, true))
	}

	cacheConfig := *config
	cacheConfig.UseIntegrationHooks = false
	cacheConfig.InstallDir = config.CacheDir

	uRepoIndex, err := fetchRepoIndex(&cacheConfig)
	if err != nil {
		return errRunFailed.Wrap(err)
	}

	verbosityLevel = silentVerbosityWithErrors

	if err := installBinaries(context.Background(), &cacheConfig, []binaryEntry{bEntry}, uRepoIndex); err != nil {
		return errRunFailed.Wrap(err)
	}

	cachedFile, err = isCached(config, bEntry)
	if err != nil {
		return errRunFailed.New("failed to find binary after installation: %v", err)
	}

	if err := runBinary(cachedFile, args, env); err != nil {
		return errRunFailed.Wrap(err)
	}
	return cleanCache(config.CacheDir)
}

func isCached(config *Config, bEntry binaryEntry) (string, error) {
	cachedFile := filepath.Join(config.CacheDir, filepath.Base(bEntry.Name))

	if fileExists(cachedFile) && isExecutable(cachedFile) {
		trackedBEntry, err := readEmbeddedBEntry(cachedFile)
		if err == nil && (trackedBEntry.PkgID == bEntry.PkgID || bEntry.PkgID == "") {
			return cachedFile, nil
		}
		fmt.Println(trackedBEntry)
	}

	return "", errRunFailed.New("binary '%s' not found in cache or does not match the requested version", bEntry.Name)
}

func runBinary(binaryPath string, args []string, env []string) error {
	cmd := exec.Command(binaryPath, args...)
	if env == nil {
		cmd.Env = os.Environ()
	} else {
		cmd.Env = env
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil && verbosityLevel == extraVerbose {
		fmt.Printf("The program (%s) errored out with a non-zero exit code (%d).\n", binaryPath, cmd.ProcessState.ExitCode())
	}
	return errRunFailed.Wrap(err)
}

func cleanCache(cacheDir string) error {
	files, err := os.ReadDir(cacheDir)
	if err != nil {
		return errRunFailed.Wrap(err)
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
		// Skip files that start with "."
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

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
