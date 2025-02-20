package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/urfave/cli/v3"
	"github.com/hedzr/progressbar"
	"github.com/hedzr/progressbar/cursor"
)

func installCommand() *cli.Command {
	return &cli.Command{
		Name:    "install",
		Aliases: []string{"add"},
		Usage:   "Install binaries",
		Action: func(ctx context.Context, c *cli.Command) error {
			config, err := loadConfig()
			if err != nil {
				return err
			}
			uRepoIndex := fetchRepoIndex(config)
			return installBinaries(context.Background(), config, arrStringToArrBinaryEntry(c.Args().Slice()), getVerbosityLevel(c), uRepoIndex)
		},
	}
}

func installBinaries(ctx context.Context, config *Config, bEntries []binaryEntry, verbosityLevel Verbosity, uRepoIndex []binaryEntry) error {
	cursor.Hide()
	defer cursor.Show()

	var wg sync.WaitGroup
	errChan := make(chan error, len(bEntries))
	urls, checksums, err := findURL(config, bEntries, verbosityLevel, uRepoIndex)
	if err != nil {
		return err
	}

	// Only create the progress bar if not in silent mode
	var bar progressbar.MultiPB
	var tasks *progressbar.Tasks
	if verbosityLevel > silentVerbosityWithErrors {
		bar = progressbar.New()
		tasks = progressbar.NewTasks(bar)
		defer tasks.Close()
	}

	var errors []string

	binaryNameMaxlen := 0
	for _, bEntry := range bEntries {
		if binaryNameMaxlen < len(bEntry.Name) {
			binaryNameMaxlen = len(bEntry.Name)
		}
	}

	termWidth := getTerminalWidth()

	for i, bEntry := range bEntries {
		wg.Add(1)
		url := urls[i]
		checksum := checksums[i]
		destination := filepath.Join(config.InstallDir, filepath.Base(bEntry.Name))

		if verbosityLevel > silentVerbosityWithErrors {
			barTitle := fmt.Sprintf("Installing %s", bEntry.Name)
			pbarOpts := []progressbar.Opt{
				progressbar.WithBarStepper(config.ProgressbarStyle),
			}

			if termWidth < 120 {
				barTitle = bEntry.Name
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
						errChan <- fmt.Errorf("error fetching binary %s: %v", bEntry.Name, fetchErr)
						return
					}

					if err := os.Chmod(destination, 0755); err != nil {
						errChan <- fmt.Errorf("error making binary executable %s: %v", destination, err)
						return
					}

					if err := runIntegrationHooks(config, destination, verbosityLevel, uRepoIndex); err != nil {
						errChan <- fmt.Errorf("[%s] could not be handled by its default hooks: %v", bEntry.Name, err)
						return
					}

					binInfo, _ := getBinaryInfo(config, bEntry, uRepoIndex)
					if err := embedBEntry(destination, binInfo.Name+"#"+binInfo.PkgId); err != nil {
						errChan <- fmt.Errorf("failed to add fullName property to the binary's xattr %s: %v", destination, err)
						return
					}
				}),
			)
		} else {
			// Perform the download without the progress bar
			go func() {
				defer wg.Done()
				_, fetchErr := fetchBinaryFromURLToDest(ctx, nil, url, checksum, destination)
				if fetchErr != nil {
					errChan <- fmt.Errorf("error fetching binary %s: %v", bEntry.Name, fetchErr)
					return
				}

				if err := os.Chmod(destination, 0755); err != nil {
					errChan <- fmt.Errorf("error making binary executable %s: %v", destination, err)
					return
				}

				if err := runIntegrationHooks(config, destination, verbosityLevel, uRepoIndex); err != nil {
					errChan <- fmt.Errorf("[%s] could not be handled by its default hooks: %v", bEntry.Name, err)
					return
				}

				binInfo, _ := getBinaryInfo(config, bEntry, uRepoIndex)
				if err := embedBEntry(destination, binInfo.Name+"#"+binInfo.PkgId); err != nil {
					errChan <- fmt.Errorf("failed to add fullName property to the binary's xattr %s: %v", destination, err)
					return
				}
			}()
		}
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

func runIntegrationHooks(config *Config, binaryPath string, verbosityLevel Verbosity, uRepoIndex []binaryEntry) error {
	if config.UseIntegrationHooks {
		ext := filepath.Ext(binaryPath)
		if hookCommands, exists := config.Hooks.Commands[ext]; exists {
			for _, cmd := range hookCommands.IntegrationCommands {
				if err := executeHookCommand(config, cmd, binaryPath, ext, config.UseIntegrationHooks, verbosityLevel, uRepoIndex); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
