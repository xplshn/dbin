// run.go // This file implements the "run" functionality
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// ReturnCachedFile retrieves the cached file location. Returns an empty string and an error if not found.
func ReturnCachedFile(tempDir, binaryName string) (string, error) {
	cachedBinary := filepath.Join(tempDir, binaryName)
	if fileExists(cachedBinary) {
		return cachedBinary, nil
	}
	return "", errors.New("cached file not found")
}

// RunFromCache runs the binary from cache or fetches it if not found.
func RunFromCache(binaryName string, args []string, tempDir, trackerFile string, transparentMode bool, verbosityLevel Verbosity, repositories []string, metadataURLs []string) error {
	flagsAndBinaryName := append(strings.Fields(binaryName), args...)
	flag.CommandLine.Parse(flagsAndBinaryName)

	if binaryName == "" {
		return errors.New("binary name not provided")
	}

	binaryPath, err := exec.LookPath(binaryName)
	if err == nil && transparentMode {
		if verbosityLevel >= normalVerbosity {
			fmt.Printf("Running '%s' from PATH...\n", binaryName)
		}
		return runBinary(binaryPath, args, verbosityLevel)
	}

	cachedFile := filepath.Join(tempDir, filepath.Base(binaryName))
	if fileExists(cachedFile) && isExecutable(cachedFile) {
		if verbosityLevel >= normalVerbosity {
			fmt.Printf("Running '%s' from cache...\n", binaryName)
		}
		if err := runBinary(cachedFile, args, verbosityLevel); err != nil {
			return err
		}
		return cleanCache(tempDir, verbosityLevel)
	}

	if verbosityLevel >= normalVerbosity {
		fmt.Printf("Couldn't find '%s' in the cache. Fetching a new one...\n", binaryName)
	}

	if err := installCommand([]string{binaryName}, tempDir, trackerFile, silentVerbosityWithErrors, repositories, metadataURLs); err != nil {
		return fmt.Errorf("err: Could not cache the binary: %v", err)
	}

	if err := runBinary(cachedFile, args, verbosityLevel); err != nil {
		return err
	}
	return cleanCache(tempDir, verbosityLevel)
}

// runBinary executes the binary with the given arguments.
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

// cleanCache removes the oldest binaries when the cache size exceeds MaxCacheSize.
func cleanCache(tempDir string, verbosityLevel Verbosity) error {
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("error reading cache directory: %v", err)
	}

	if len(files) <= maxCacheSize {
		return nil
	}

	type fileWithAtime struct {
		info  os.FileInfo
		atime time.Time
	}

	var filesWithAtime []fileWithAtime
	for _, entry := range files {
		fileInfo, err := entry.Info()
		if err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "error getting file info: %v\n", err)
			}
			continue
		}

		var stat syscall.Stat_t
		if err := syscall.Stat(filepath.Join(tempDir, entry.Name()), &stat); err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "error getting file stat: %v\n", err)
			}
			continue
		}

		atime := time.Unix(int64(stat.Atim.Sec), int64(stat.Atim.Nsec))
		filesWithAtime = append(filesWithAtime, fileWithAtime{info: fileInfo, atime: atime})
	}

	sort.Slice(filesWithAtime, func(i, j int) bool {
		return filesWithAtime[i].atime.Before(filesWithAtime[j].atime)
	})

	for i := 0; i < binariesToDelete; i++ {
		if err := os.Remove(filepath.Join(tempDir, filesWithAtime[i].info.Name())); err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "error removing file: %v\n", err)
			}
		}
	}
	return nil
}
