package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/goccy/go-json"
	"github.com/goccy/go-yaml"
	"github.com/urfave/cli/v3"
	"github.com/zeebo/errs"
)

var (
	errBinaryInfoNotFound = errs.Class("binary info not found")
)

func infoCommand() *cli.Command {
	return &cli.Command{
		Name:  "info",
		Usage: "Show information about a specific binary OR display installed binaries",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Print output as JSON",
			},
			&cli.BoolFlag{
				Name:  "cbor",
				Usage: "Print output as CBOR",
			},
			&cli.BoolFlag{
				Name:  "yaml",
				Usage: "Print output as YAML",
			},
		},
		Action: func(_ context.Context, c *cli.Command) error {
			config, err := loadConfig()
			if err != nil {
				return err
			}
			var bEntry binaryEntry
			if c.Args().First() != "" {
				uRepoIndex, err := fetchRepoIndex(config)
				if err != nil {
					return err
				}
				bEntry = stringToBinaryEntry(c.Args().First())
				binaryInfo, err := getBinaryInfo(config, bEntry, uRepoIndex)
				if err != nil {
					return errBinaryInfoNotFound.Wrap(err)
				}

				if c.Bool("json") {
					jsonData, err := json.MarshalIndent(binaryInfo, "", "  ")
					if err != nil {
						return err
					}
					fmt.Println(string(jsonData))
					return nil
				}

				if c.Bool("cbor") {
					cborData, err := cbor.Marshal(binaryInfo)
					if err != nil {
						return err
					}
					fmt.Println(string(cborData))
					return nil
				}

				if c.Bool("yaml") {
					yamlData, err := yaml.Marshal(binaryInfo)
					if err != nil {
						return err
					}
					fmt.Println(string(yamlData))
					return nil
				}

				printBEntry(binaryInfo)

			} else {
				binaryEntries, err := validateProgramsFrom(config, nil, nil)
				if err != nil {
					return err
				}
				for _, program := range binaryEntries {
					fmt.Println(parseBinaryEntry(program, true))
				}
			}
			return nil
		},
	}
}

func findBinaryInfo(bEntry binaryEntry, uRepoIndex []binaryEntry) (binaryEntry, bool) {
	matchingBins := findMatchingBins(bEntry, uRepoIndex)

	if len(matchingBins) == 0 {
		return binaryEntry{}, false
	}

	return matchingBins[0], true
}

func getBinaryInfo(config *config, bEntry binaryEntry, uRepoIndex []binaryEntry) (*binaryEntry, error) {
	if instBEntry := bEntryOfinstalledBinary(filepath.Join(config.InstallDir, bEntry.Name)); bEntry.PkgID == "" && instBEntry.PkgID != "" {
		bEntry = instBEntry
	}

	binInfo, found := findBinaryInfo(bEntry, uRepoIndex)
	if found {
		return &binInfo, nil
	}

	return nil, errBinaryInfoNotFound.New("info for the requested binary ('%s') not found in any of the repository index files", parseBinaryEntry(bEntry, false))
}

func printBEntry(bEntry *binaryEntry) {
	fields := []struct {
		label string
		value any
	}{
		// Most important to the user
		{"Name", bEntry.Name + "#" + bEntry.PkgID},
		{"Pkg ID", bEntry.PkgID},

		{"Pretty Name", bEntry.PrettyName},
		{"Description", bEntry.Description},

		{"Version", bEntry.Version},
		{"Size", bEntry.Size},

		{"Categories", bEntry.Categories},

		{"Download URL", bEntry.DownloadURL},
		{"WebURLs", bEntry.WebURLs},
		{"SrcURLs", bEntry.SrcURLs},

		{"B3SUM", bEntry.Bsum},
		{"SHA256", bEntry.Shasum},
		{"Build Date", bEntry.BuildDate},
		{"Build Script", bEntry.BuildScript},
		{"Build Log", bEntry.BuildLog},

		// ------------------------------------

		// Clutter:
		// Useless in the context of `dbin`
		// These are shown only in the complete
		// repository index (non-lite version)
		{"Screenshots", bEntry.Screenshots},
		{"Icon URL", bEntry.Icon},
		{"Web Manifest", bEntry.WebManifest},
		{"Extra Bins", bEntry.ExtraBins},

		// Clutter, but useful:
		{"Snapshots", bEntry.Snapshots},

		// SBUILD meta
		{"Maintainers", bEntry.Maintainers},
		{"Notes", bEntry.Notes},
		{"License", bEntry.License},

		{"Rank", bEntry.Rank},
	}
	for _, field := range fields {
		switch v := field.value.(type) {
		case []string:
			for n, str := range v {
				prefixLength := len(field.label)
				prefix := blueBgWhiteFg + field.label + resetColor
				if n > 0 {
					prefix = strings.Repeat(" ", prefixLength)
				}
				fmt.Printf("%s: %s\n", prefix, str)
			}
		case []snapshot:
			for n, snap := range v {
				prefix := blueBgWhiteFg + field.label + resetColor
				if n > 0 {
					prefix = "         "
				}
				if snap.Commit != "" {
					fmt.Printf("%s: %s %s\n", prefix, snap.Commit, ternary(snap.Version != "", "["+cyanColor+snap.Version+resetColor+"]", ""))
				} else {
					fmt.Printf("%s: %s\n", prefix, "["+cyanColor+snap.Version+resetColor+"]")
				}
			}
		case uint16:
			if v != 0 {
				switch v {
					case 1:
						fmt.Printf("%s\x1b[0m: 🥇(%v)\n", blueBgWhiteFg+field.label+resetColor, v)
					case 2:
						fmt.Printf("%s\x1b[0m: 🥈(%v)\n", blueBgWhiteFg+field.label+resetColor, v)
					case 3:
						fmt.Printf("%s\x1b[0m: 🥉(%v)\n", blueBgWhiteFg+field.label+resetColor, v)
					default:
						fmt.Printf("%s\x1b[0m: %v\n", blueBgWhiteFg+field.label+resetColor, v)
				}
			}
		default:
			if v != "" && v != 0 {
				fmt.Printf("%s\x1b[0m: %v\n", blueBgWhiteFg+field.label+resetColor, v)
			}
		}
	}
}
