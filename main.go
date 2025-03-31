package main

import (
	"fmt"
	"os"
	"context"

	"github.com/urfave/cli/v3"
)

type Verbosity int8

const (
	unsupportedArchMsg                  = "Unsupported architecture: "
	Indicator                           = "...>"
	Version                             = "1.2"
	maxCacheSize                        = 15
	binariesToDelete                    = 5
	extraVerbose              Verbosity = 2
	normalVerbosity           Verbosity = 1
	silentVerbosityWithErrors Verbosity = -1
	extraSilent               Verbosity = -2
)

func main() {
	app := &cli.Command{
		Name:        "dbin",
		Usage:       "The easy to use, easy to get, software distribution system",
		Version:     Version,
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
		},
		EnableShellCompletion: true,
	}

	err := app.Run(context.Background(), os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func getVerbosityLevel(c *cli.Command) Verbosity {
	if c.Bool("extra-silent") {
		return extraSilent
	} else if c.Bool("silent") {
		return silentVerbosityWithErrors
	} else if c.Bool("verbose") {
		return extraVerbose
	}
	return normalVerbosity
}

func fetchRepoIndex(config *Config) []binaryEntry {
	var uRepoIndex []binaryEntry
	for _, url := range config.RepoURLs {
		repoIndex, err := decodeRepoIndex(url)
		if err != nil {
			fmt.Printf("failed to fetch and decode binary information from %s: %v\n", url, err)
			continue
		}
		uRepoIndex = append(uRepoIndex, repoIndex...)
	}
	return uRepoIndex
}
