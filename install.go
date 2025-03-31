package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/hedzr/progressbar"
	"github.com/hedzr/progressbar/cursor"
	"github.com/urfave/cli/v3"
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
	var errors []string

	// New version using the binaryEntryResult
	binResults, err := findURL(config, bEntries, verbosityLevel, uRepoIndex)
	if err != nil {
		return err
	}

	// Only create the progress bar if not in silent mode
	var bar progressbar.MultiPB
	var tasks *progressbar.Tasks
	if verbosityLevel >= normalVerbosity {
		bar = progressbar.New()
		tasks = progressbar.NewTasks(bar)
		defer tasks.Close()
	}

	binaryNameMaxlen := 0
	for _, result := range binResults {
		if binaryNameMaxlen < len(result.Name) {
			binaryNameMaxlen = len(result.Name)
		}
	}

	termWidth := getTerminalWidth()

	for _, result := range binResults {
		wg.Add(1)
		bEntry := result
		destination := filepath.Join(config.InstallDir, filepath.Base(bEntry.Name))

		if verbosityLevel >= normalVerbosity {
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
					_, fetchErr := fetchBinaryFromURLToDest(ctx, bar, bEntry, destination)
					if fetchErr != nil {
						errors = append(errors, fmt.Sprintf("error: error fetching binary %s: %v\n", bEntry.Name, fetchErr))
						return
					}

					if err := os.Chmod(destination, 0755); err != nil {
						errors = append(errors, fmt.Sprintf("error: error making binary executable %s: %v\n", destination, err))
						return
					}

					if err := runIntegrationHooks(config, destination, verbosityLevel, uRepoIndex); err != nil {
						errors = append(errors, fmt.Sprintf("error: [%s] could not be handled by its default hooks: %v\n", bEntry.Name, err))
						return
					}

					// Use the binary entry from the result directly since it already has the correct version info
					binInfo := &bEntry
					if err := embedBEntry(destination, *binInfo); err != nil {
						errors = append(errors, fmt.Sprintf("error: failed to add fullName property to the binary's xattr %s: %v\n", destination, err))
						return
					}
				}),
			)
		} else {
			go func(bEntry binaryEntry, destination string) {
				defer wg.Done()
				_, fetchErr := fetchBinaryFromURLToDest(ctx, nil, bEntry, destination)
				if fetchErr != nil {
					errors = append(errors, fmt.Sprintf("error: error fetching binary %s: %v", bEntry.Name, fetchErr))
					return
				}

				if err := os.Chmod(destination, 0755); err != nil {
					errors = append(errors, fmt.Sprintf("error: error making binary executable %s: %v", destination, err))
					return
				}

				if err := runIntegrationHooks(config, destination, verbosityLevel, uRepoIndex); err != nil {
					errors = append(errors, fmt.Sprintf("error: [%s] could not be handled by its default hooks: %v", bEntry.Name, err))
					return
				}

				// Use the binaryEntry directly with its correct version info
				binInfo := &bEntry
				if err := embedBEntry(destination, *binInfo); err != nil {
					errors = append(errors, fmt.Sprintf("error: failed to add fullName property to the binary's xattr %s: %v", destination, err))
					return
				}

				if verbosityLevel >= normalVerbosity {
					fmt.Printf("Successfully installed [%s]\n", binInfo.Name+"#"+binInfo.PkgId)
				}
			}(bEntry, destination)
		}
	}

	wg.Wait()

	if len(errors) > 0 {
		var errN = uint8(0)
		for _, errMsg := range errors {
			errN += 1
			fmt.Printf("%d. %v\n", errN, errMsg)
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
