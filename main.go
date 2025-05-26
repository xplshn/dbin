// PROGRAM: DBIN
// MAINTAINER: IDIOT (xplshn)
// PURPOSE: Package manager done right
// DESCRIPTION: A package manager that uses one-file packages (statically linked binaries, self-contained binaries)
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/urfave/cli/v3"
)

const (
	unsupportedArchMsg = "Unsupported architecture: "
	version            = 1.5
	maxCacheSize       = 15
	binariesToDelete   = 5
	// --------------------------------
	extraVerbose              int8 = 2
	normalVerbosity           int8 = 1
	silentVerbosityWithErrors int8 = -1
	extraSilent               int8 = -2
	// -------------------------------
)

var verbosityLevel = normalVerbosity

func main() {
	app := &cli.Command{
		Name:        "dbin",
		Usage:       "The easy to use, easy to get, software distribution system",
		Version:     strconv.FormatFloat(version, 'f', -1, 32),
		Description: "The easy to use, easy to get, software distribution system",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "Run in extra verbose mode",
			},
			&cli.BoolFlag{
				Name:  "silent",
				Usage: "Run in silent mode, only errors will be shown",
			},
			&cli.BoolFlag{
				Name:  "extra-silent",
				Usage: "Run in extra silent mode, suppressing almost all output",
			},
		},
		Commands: []*cli.Command{
			installCommand(),
			removeCommand(),
			listCommand(),
			searchCommand(),
			infoCommand(),
			runCommand(),
			updateCommand(),
			configCommand(),
		},
		EnableShellCompletion: true,
	}

	err := app.Run(context.Background(), os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func fetchRepoIndex(config *config) ([]binaryEntry, error) {
	uRepoIndex, err := decodeRepoIndex(config)
	if err != nil {
		return nil, fmt.Errorf("%v: Consider checking if DBIN_NOCONFIG=1 works, if so, consider modifying your config, your repository URLs may be outdated.\nAlso consider removing dbin's cache if the above fails", err)
	}
	return uRepoIndex, nil
}
