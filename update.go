package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/urfave/cli/v3"
	"github.com/zeebo/errs"
)

var (
	errUpdateFailed = errs.Class("update failed")
)

func updateCommand() *cli.Command {
	return &cli.Command{
		Name:  "update",
		Usage: "Update binaries, by checking their b3sum[:256] against the repo's",
		Action: func(_ context.Context, c *cli.Command) error {
			config, err := loadConfig()
			if err != nil {
				return errUpdateFailed.Wrap(err)
			}
			uRepoIndex, err := fetchRepoIndex(config)
			if err != nil {
				return errUpdateFailed.Wrap(err)
			}
			return update(config, arrStringToArrBinaryEntry(c.Args().Slice()), uRepoIndex)
		},
	}
}

func update(config *config, programsToUpdate []binaryEntry, uRepoIndex []binaryEntry) error {
	var (
		skipped, updated, errors uint32
		checked                  uint32
		errorMessages            string
		errorMessagesMutex       sync.Mutex
		padding                  = " "
	)

	programsToUpdate, err := validateProgramsFrom(config, programsToUpdate, uRepoIndex)
	if err != nil {
		return errUpdateFailed.Wrap(err)
	}

	toBeChecked := uint32(len(programsToUpdate))

	var progressMutex sync.Mutex
	var wg sync.WaitGroup

	var outdatedPrograms []binaryEntry

	installDir := config.InstallDir
	for _, program := range programsToUpdate {
		wg.Add(1)

		go func(program binaryEntry) {
			defer wg.Done()

			installPath := filepath.Join(installDir, filepath.Base(program.Name))
			trackedBEntry, err := readEmbeddedBEntry(installPath)
			if err != nil {
				return
			}

			if !fileExists(installPath) {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, "\033[2K\r<%d/%d> %s | Warning: Tried to update a non-existent program %s. Skipping.", atomic.LoadUint32(&checked), toBeChecked, padding, parseBinaryEntry(trackedBEntry, false))
				}
				progressMutex.Unlock()
				return
			}

			localB3sum, err := calculateChecksum(installPath)
			if err != nil {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, "\033[2K\r<%d/%d> %s | Warning: Failed to get B3sum for %s. Skipping.", atomic.LoadUint32(&checked), toBeChecked, padding, parseBinaryEntry(trackedBEntry, false))
				}
				progressMutex.Unlock()
				return
			}

			binInfo, err := getBinaryInfo(config, program, uRepoIndex)
			if err != nil {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, "\033[2K\r<%d/%d> %s | Warning: Failed to get metadata for %s. Skipping.", atomic.LoadUint32(&checked), toBeChecked, padding, parseBinaryEntry(trackedBEntry, false))
				}
				progressMutex.Unlock()
				return
			}

			if binInfo.Bsum == "" {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&skipped, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, "\033[2K\r<%d/%d> %s | Skipping %s because the B3sum field is null.", atomic.LoadUint32(&checked), toBeChecked, padding, parseBinaryEntry(trackedBEntry, false))
				}
				progressMutex.Unlock()
				return
			}

			if localB3sum != binInfo.Bsum {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				atomic.AddUint32(&updated, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, "\033[2K\r<%d/%d> %s | %s is outdated and will be updated.", atomic.LoadUint32(&checked), toBeChecked, padding, parseBinaryEntry(trackedBEntry, false))
				}
				errorMessagesMutex.Lock()
				outdatedPrograms = append(outdatedPrograms, program)
				errorMessagesMutex.Unlock()
				progressMutex.Unlock()
			} else {
				progressMutex.Lock()
				atomic.AddUint32(&checked, 1)
				if verbosityLevel >= normalVerbosity {
					truncatePrintf(false, "\033[2K\r<%d/%d> %s | No updates available for %s.", atomic.LoadUint32(&checked), toBeChecked, padding, parseBinaryEntry(trackedBEntry, false))
				}
				progressMutex.Unlock()
			}
		}(program)
	}

	wg.Wait()

	if len(outdatedPrograms) > 0 {
		fmt.Print("\033[2K\r")
		err := installBinaries(context.Background(), config, outdatedPrograms, uRepoIndex)
		if err != nil {
			atomic.AddUint32(&errors, 1)
		}
	
		var stillOutdated []string
		for _, program := range outdatedPrograms {
			installPath := filepath.Join(config.InstallDir, filepath.Base(program.Name))
			localB3sum, sumErr := calculateChecksum(installPath)
			repoBEntry, infoErr := getBinaryInfo(config, program, uRepoIndex)
			if sumErr != nil || infoErr != nil || localB3sum != repoBEntry.Bsum {
				stillOutdated = append(stillOutdated, program.Name)
			}
		}
	
		if len(stillOutdated) > 0 && verbosityLevel >= silentVerbosityWithErrors {
			fmt.Printf("Failed to update programs: %s\n", strings.Join(stillOutdated, ", "))
		}
	}

	finalCounts := fmt.Sprintf("\033[2K\rSkipped: %d\tUpdated: %d\tChecked: %d", atomic.LoadUint32(&skipped), atomic.LoadUint32(&updated), uint32(int(atomic.LoadUint32(&checked))))
	if errors > 0 && verbosityLevel >= silentVerbosityWithErrors {
		finalCounts += fmt.Sprintf("\tErrors: %d", atomic.LoadUint32(&errors))
	}

	if verbosityLevel >= normalVerbosity || (errors > 0 && verbosityLevel >= silentVerbosityWithErrors) {
		fmt.Print(finalCounts)
		for _, errorMsg := range strings.Split(errorMessages, "\n") {
			fmt.Println(strings.TrimSpace(errorMsg))
		}
	}

	return nil
}
