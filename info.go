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

				fields := []struct {
					label string
					value any
				}{
					{"Name", binaryInfo.Name + "#" + binaryInfo.PkgID},
					{"Pkg ID", binaryInfo.PkgID},
					{"Pretty Name", binaryInfo.PrettyName},
					{"Description", binaryInfo.Description},
					{"Version", binaryInfo.Version},
					{"Size", binaryInfo.Size},
					{"Categories", binaryInfo.Categories},
					{"WebURLs", binaryInfo.WebURLs},
					{"SrcURLs", binaryInfo.SrcURLs},
					{"Download URL", binaryInfo.DownloadURL},
					{"Icon URL", binaryInfo.Icon},
					{"B3SUM", binaryInfo.Bsum},
					{"SHA256", binaryInfo.Shasum},
					{"Build Date", binaryInfo.BuildDate},
					{"Build Script", binaryInfo.BuildScript},
					{"Build Log", binaryInfo.BuildLog},
					{"Screenshots", binaryInfo.Screenshots},
					{"Extra Bins", binaryInfo.ExtraBins},
					{"Snapshots", binaryInfo.Snapshots},
					{"Notes", binaryInfo.Notes},
					{"License", binaryInfo.License},
					{"Rank", binaryInfo.Rank},
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
							fmt.Printf("%s: %s %s\n", prefix, snap.Commit, ternary(snap.Version != "", "["+cyanColor+snap.Version+resetColor+"]", ""))
						}
					default:
						if v != "" && v != 0 {
							fmt.Printf("%s\x1b[0m: %v\n", blueBgWhiteFg+field.label+resetColor, v)
						}
					}
				}
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
