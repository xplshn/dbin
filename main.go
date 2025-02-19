package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

type Verbosity int8

const (
	unsupportedArchMsg                  = "Unsupported architecture: "
	indicator                           = "...>"
	Version                             = "1.0"
	maxCacheSize                        = 15
	binariesToDelete                    = 5
	normalVerbosity           Verbosity = 1
	extraVerbose              Verbosity = 2
	silentVerbosityWithErrors Verbosity = -1
	extraSilent               Verbosity = -2
)

func main() {
	app := &cli.App{
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
			{
				Name:   "install",
				Aliases: []string{"add"},
				Usage:   "Install a binary",
				Action: func(c *cli.Context) error {
					if c.NArg() < 1 {
						return fmt.Errorf("no binary name provided for install command")
					}
					config, err := loadConfig()
					if err != nil {
						return err
					}
					uRepoIndex := fetchRepoIndex(config)
					return installCommand(config, arrStringToArrBinaryEntry(removeDuplicates(c.Args().Slice())), getVerbosityLevel(c), uRepoIndex)
				},
			},
			{
				Name:   "findurl",
				Usage:   "find the origin url of a binary",
				Action: func(c *cli.Context) error {
					if c.NArg() < 1 {
						return fmt.Errorf("no binary name provided for findurl command")
					}
					config, err := loadConfig()
					if err != nil {
						return err
					}
					uRepoIndex := fetchRepoIndex(config)
					fURLs, bsums, _ := findURL(config, arrStringToArrBinaryEntry(removeDuplicates(c.Args().Slice())), getVerbosityLevel(c), uRepoIndex)
					fmt.Println(fURLs, bsums)
					return nil
				},
			},
			{
				Name:   "remove",
				Aliases: []string{"del"},
				Usage:   "Remove a binary",
				Action: func(c *cli.Context) error {
					if c.NArg() < 1 {
						return fmt.Errorf("no binary name provided for remove command")
					}
					config, err := loadConfig()
					if err != nil {
						return err
					}
					uRepoIndex := fetchRepoIndex(config)
					return removeBinaries(config, arrStringToArrBinaryEntry(removeDuplicates(c.Args().Slice())), getVerbosityLevel(c), uRepoIndex)
				},
			},
			{
				Name:  "list",
				Usage: "List all available binaries",
				Action: func(c *cli.Context) error {
					config, err := loadConfig()
					if err != nil {
						return err
					}
					uRepoIndex := fetchRepoIndex(config)
					if c.NArg() == 1 && c.Args().First() == "--described" {
						return fSearch(config, []string{""}, uRepoIndex)
					}
					bEntries, err := listBinaries(uRepoIndex)
					if err != nil {
						return err
					}
					for _, binary := range binaryEntriesToArrString(bEntries, true) {
						fmt.Println(binary)
					}
					return nil
				},
			},
			{
				Name:  "search",
				Usage: "Search for a binary by supplying one or more search terms",
				Action: func(c *cli.Context) error {
					if c.NArg() < 1 {
						return fmt.Errorf("no search terms provided")
					}
					config, err := loadConfig()
					if err != nil {
						return err
					}
					uRepoIndex := fetchRepoIndex(config)
					query := c.Args().Slice()
					return fSearch(config, query, uRepoIndex)
				},
			},
			{
				Name:  "info",
				Usage: "Show information about a specific binary OR display installed binaries",
				Action: func(c *cli.Context) error {
					config, err := loadConfig()
					if err != nil {
						return err
					}
					uRepoIndex := fetchRepoIndex(config)
					var bEntry binaryEntry
					if c.NArg() == 1 {
						bEntry = stringToBinaryEntry(c.Args().First())
					}
					if bEntry.Name == "" {
						files, err := listFilesInDir(config.InstallDir)
						if err != nil {
							return err
						}
						installedPrograms := make([]string, 0)
						for _, file := range files {
							trackedBEntry := bEntryOfinstalledBinary(file)
							if trackedBEntry.Name != "" {
								installedPrograms = append(installedPrograms, parseBinaryEntry(trackedBEntry, true))
							}
						}
						for _, program := range installedPrograms {
							fmt.Println(program)
						}
					} else {
						binaryInfo, err := getBinaryInfo(config, bEntry, uRepoIndex)
						if err != nil {
							return err
						}
						fields := []struct {
							label string
							value interface{}
						}{
							{"Name", binaryInfo.Name + "#" + binaryInfo.PkgId},
							{"Pkg ID", binaryInfo.PkgId},
							{"Pretty Name", binaryInfo.PrettyName},
							{"Description", binaryInfo.Description},
							{"Version", binaryInfo.Version},
							{"Ghcr Blob", binaryInfo.GhcrBlob},
							{"Download URL", binaryInfo.DownloadURL},
							{"Size", binaryInfo.Size},
							{"B3SUM", binaryInfo.Bsum},
							{"SHA256", binaryInfo.Shasum},
							{"Build Date", binaryInfo.BuildDate},
							{"Build Script", binaryInfo.BuildScript},
							{"Build Log", binaryInfo.BuildLog},
							{"Categories", binaryInfo.Categories},
							{"Rank", binaryInfo.Rank},
							{"Extra Bins", binaryInfo.ExtraBins},
						}
						for _, field := range fields {
							switch v := field.value.(type) {
							case []string:
								for n, str := range v {
									prefix := "\033[48;5;4m" + field.label + "\033[0m"
									if n > 0 {
										prefix = "         "
									}
									fmt.Printf("%s: %s\n", prefix, str)
								}
							default:
								if v != "" && v != 0 {
									fmt.Printf("\033[48;5;4m%s\033[0m: %v\n", field.label, v)
								}
							}
						}
					}
					return nil
				},
			},
			{
				Name:  "run",
				Usage: "Run a specified binary from cache",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "transparent",
						Usage: "Run the binary from PATH if found",
					},
				},
				Action: func(c *cli.Context) error {
					if c.NArg() < 1 {
						return fmt.Errorf("no binary name provided for run command")
					}
					config, err := loadConfig()
					if err != nil {
						return err
					}
					uRepoIndex := fetchRepoIndex(config)
					return RunFromCache(config, stringToBinaryEntry(c.Args().First()), c.Args().Tail(), c.Bool("transparent"), getVerbosityLevel(c), uRepoIndex)
				},
			},
			{
				Name:  "tldr",
				Usage: "Equivalent to --silent run --transparent tlrc",
				Action: func(c *cli.Context) error {
					config, err := loadConfig()
					if err != nil {
						return err
					}
					uRepoIndex := fetchRepoIndex(config)
					return RunFromCache(config, stringToBinaryEntry("tlrc"), c.Args().Slice(), true, getVerbosityLevel(c), uRepoIndex)
				},
			},
			{
				Name:  "eget2",
				Usage: "Run eget2 from cache",
				Action: func(c *cli.Context) error {
					config, err := loadConfig()
					if err != nil {
						return err
					}
					uRepoIndex := fetchRepoIndex(config)
					return RunFromCache(config, stringToBinaryEntry("eget2"), c.Args().Slice(), true, getVerbosityLevel(c), uRepoIndex)
				},
			},
			{
				Name:  "update",
				Usage: "Update binaries, by checking their SHA against the repo's SHA",
				Action: func(c *cli.Context) error {
					config, err := loadConfig()
					if err != nil {
						return err
					}
					uRepoIndex := fetchRepoIndex(config)
					return update(config, arrStringToArrBinaryEntry(removeDuplicates(c.Args().Slice())), getVerbosityLevel(c), uRepoIndex)
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func getVerbosityLevel(c *cli.Context) Verbosity {
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
