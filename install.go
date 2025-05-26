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
	"github.com/zeebo/errs"
)

var (
	errInstallFailed = errs.Class("installation failed")
)

func installCommand() *cli.Command {
	return &cli.Command{
		Name:    "install",
		Aliases: []string{"add"},
		Usage:   "Install binaries",
		Action: func(_ context.Context, c *cli.Command) error {
			config, err := loadConfig()
			if err != nil {
				return err
			}
			uRepoIndex, err := fetchRepoIndex(config)
			if err != nil {
				return err
			}
			return installBinaries(context.Background(), config, arrStringToArrBinaryEntry(c.Args().Slice()), uRepoIndex)
		},
	}
}

func installBinaries(ctx context.Context, config *config, bEntries []binaryEntry, uRepoIndex []binaryEntry) error {
	cursor.Hide()
	defer cursor.Show()

	var wg sync.WaitGroup
	var errors []string
	var errorsMu sync.Mutex

	binResults, err := findURL(config, bEntries, uRepoIndex)
	if err != nil {
		return errInstallFailed.Wrap(err)
	}

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
				progressbar.WithBarResumeable(true),
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
				progressbar.WithTaskAddOnTaskProgressing(func(bar progressbar.PB, _ <-chan struct{}) (stop bool) {
					defer wg.Done()
					err := fetchBinaryFromURLToDest(ctx, bar, &bEntry, destination, config)
					if err != nil {
						errorsMu.Lock()
						errors = append(errors, fmt.Sprintf("error fetching binary %s: %v\n", bEntry.Name, err))
						errorsMu.Unlock()
						return
					}

					if err := os.Chmod(destination, 0755); err != nil {
						errorsMu.Lock()
						errors = append(errors, fmt.Sprintf("error making binary executable %s: %v\n", destination, err))
						errorsMu.Unlock()
						return
					}

					if err := runIntegrationHooks(config, destination); err != nil {
						errorsMu.Lock()
						errors = append(errors, fmt.Sprintf("[%s] could not be handled by its default hooks: %v\n", bEntry.Name, err))
						errorsMu.Unlock()
						return
					}

					binInfo := &bEntry
					if err := embedBEntry(destination, *binInfo); err != nil {
						errorsMu.Lock()
						errors = append(errors, fmt.Sprintf("failed to add fullName property to the binary's xattr %s: %v\n", destination, err))
						errorsMu.Unlock()
						return
					}

					return
				}),
			)
		} else {
			go func(bEntry binaryEntry, destination string) {
				defer wg.Done()
				err := fetchBinaryFromURLToDest(ctx, nil, &bEntry, destination, config)
				if err != nil {
					errorsMu.Lock()
					errors = append(errors, fmt.Sprintf("error fetching binary %s: %v", bEntry.Name, err))
					errorsMu.Unlock()
					return
				}

				if err := os.Chmod(destination, 0755); err != nil {
					errorsMu.Lock()
					errors = append(errors, fmt.Sprintf("error making binary executable %s: %v", destination, err))
					errorsMu.Unlock()
					return
				}

				if err := runIntegrationHooks(config, destination); err != nil {
					errorsMu.Lock()
					errors = append(errors, fmt.Sprintf("[%s] could not be handled by its default hooks: %v", bEntry.Name, err))
					errorsMu.Unlock()
					return
				}

				binInfo := &bEntry
				if err := embedBEntry(destination, *binInfo); err != nil {
					errorsMu.Lock()
					errors = append(errors, fmt.Sprintf("failed to add fullName property to the binary's xattr %s: %v", destination, err))
					errorsMu.Unlock()
					return
				}

				if verbosityLevel >= normalVerbosity {
					fmt.Printf("Successfully installed [%s]\n", binInfo.Name+"#"+binInfo.PkgID)
				}
			}(bEntry, destination)
		}
	}

	wg.Wait()

	if len(errors) > 0 {
		var errN = uint8(0)
		for _, errMsg := range errors {
			errN++
			fmt.Printf("%d. %v\n", errN, errMsg)
		}
	}

	return nil
}

func runIntegrationHooks(config *config, binaryPath string) error {
	if config.UseIntegrationHooks {
		ext := filepath.Ext(binaryPath)
		if hookCommands, exists := config.Hooks.Commands[ext]; exists {
			if err := executeHookCommand(config, hookCommands.IntegrationCommand, binaryPath, ext, true); err != nil {
				return errInstallFailed.Wrap(err)
			}
		}
	}
	return nil
}


