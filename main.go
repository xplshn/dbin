// PROGRAM: DBIN
// MAINTAINER: IDIOT (xplshn)
// PURPOSE: Package manager done right
// DESCRIPTION: A package manager that uses one-file packages (statically linked binaries, self-contained binaries)
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"
)

const (
	version            = 1.7
	maxCacheSize       = 15
	binariesToDelete   = 5
	// --------------------------------
	extraVerbose              uint8 = 4
	normalVerbosity           uint8 = 3
	silentVerbosityWithErrors uint8 = 2
	extraSilent               uint8 = 1
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
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			switch {
			case c.Bool("extra-silent"):
				verbosityLevel = extraSilent
			case c.Bool("silent"):
				verbosityLevel = silentVerbosityWithErrors
			case c.Bool("verbose"):
				verbosityLevel = extraVerbose
			}
			return nil, nil
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

	pathDirs := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))
	found := false
	for _, dir := range pathDirs {
		if !found || len(pathDirs) == 0 {
			if dir == "." || dir == ".." {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, filepath.Base(os.Args[0]))); err == nil {
				found = true
				break
			}
		}
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	if !found {
		fmt.Fprintf(os.Stderr, "\n%swarning%s: dbin not in $PATH\n", yellowColor, resetColor)
	}
}

func fetchRepoIndex(config *config) ([]binaryEntry, error) {
	uRepoIndex, err := decodeRepoIndex(config)
	if err != nil {
		return nil, fmt.Errorf("%v: Consider checking if DBIN_NOCONFIG=1 works, if so, consider modifying your config, your repository URLs may be outdated.\nAlso consider removing dbin's cache if the above fails", err)
	}
	return uRepoIndex, nil
}
