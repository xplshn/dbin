package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hedzr/progressbar"
	"github.com/hedzr/progressbar/cursor"
)

func installBinaries(ctx context.Context, config *Config, binaries []binaryEntry, verbosityLevel Verbosity, metadata map[string]interface{}) error {
	var outputDevice io.Writer
	if verbosityLevel <= silentVerbosityWithErrors {
		outputDevice = io.Discard
	} else {
		outputDevice = os.Stdout
		cursor.Hide()
		defer cursor.Show()
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(binaries))
	urls, checksums, err := findURL(config, binaries, verbosityLevel, metadata)
	if err != nil {
		return err
	}

	bar := progressbar.New(progressbar.WithOutputDevice(outputDevice))
	tasks := progressbar.NewTasks(bar)
	defer tasks.Close()

	var errors []string

	binaryNameMaxlen := 0
	for _, binaryEntry := range binaries {
		if binaryNameMaxlen < len(binaryEntry.Name) {
			binaryNameMaxlen = len(binaryEntry.Name)
		}
	}

	termWidth := getTerminalWidth()

	for i, binaryEntry := range binaries {
		wg.Add(1)
		url := urls[i]
		checksum := checksums[i]
		destination := filepath.Join(config.InstallDir, filepath.Base(binaryEntry.Name))

		barTitle := fmt.Sprintf("Installing %s", binaryEntry.Name)
		pbarOpts := []progressbar.Opt{
			progressbar.WithBarStepper(config.ProgressbarStyle),
		}

		if termWidth < 120 {
			barTitle = binaryEntry.Name
			pbarOpts = append(
				pbarOpts,
				progressbar.WithBarTextSchema(`{{.Bar}} {{.Percent}} | <font color="green">{{.Title}}</font>`),
				progressbar.WithBarWidth(termWidth-(binaryNameMaxlen+19)),
			)
		}

		tasks.Add(
			progressbar.WithTaskAddBarTitle(barTitle),
			progressbar.WithTaskAddBarOptions(pbarOpts...),
			progressbar.WithTaskAddOnTaskProgressing(func(bar progressbar.PB, exitCh <-chan struct{}) {
				defer wg.Done()
				_, fetchErr := fetchBinaryFromURLToDest(ctx, bar, url, checksum, destination)
				if fetchErr != nil {
					errChan <- fmt.Errorf("error fetching binary %s: %v", binaryEntry.Name, fetchErr)
					return
				}

				if err := os.Chmod(destination, 0755); err != nil {
					errChan <- fmt.Errorf("error making binary executable %s: %v", destination, err)
					return
				}

				if err := runIntegrationHooks(config, destination, verbosityLevel, metadata); err != nil {
					errChan <- fmt.Errorf("[%s] could not be handled by its default hooks: %v", binaryEntry.Name, err)
					return
				}

				binInfo, _ := getBinaryInfo(config, binaryEntry, metadata)
				if err := addFullName(destination, binInfo.PkgId); err != nil {
					errChan <- fmt.Errorf("failed to add fullName property to the binary's xattr %s: %v", destination, err)
					return
				}
			}),
		)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

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

func runIntegrationHooks(config *Config, binaryPath string, verbosityLevel Verbosity, metadata map[string]interface{}) error {
	if config.UseIntegrationHooks {
		ext := filepath.Ext(binaryPath)
		if hookCommands, exists := config.Hooks.Commands[ext]; exists {
			for _, cmd := range hookCommands.IntegrationCommands {
				if err := executeHookCommand(config, cmd, binaryPath, ext, config.UseIntegrationHooks, verbosityLevel, metadata); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func installCommand(config *Config, binaries []binaryEntry, verbosityLevel Verbosity, metadata map[string]interface{}) error {
	return installBinaries(context.Background(), config, removeDuplicates(binaries), verbosityLevel, metadata)
}
