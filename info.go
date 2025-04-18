package main

import (
	"fmt"
	"path/filepath"
	"context"

	"github.com/urfave/cli/v3"
)

func infoCommand() *cli.Command {
    return &cli.Command{
        Name:  "info",
        Usage: "Show information about a specific binary OR display installed binaries",
        Action: func(ctx context.Context, c *cli.Command) error {
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
                    {"Size", binaryInfo.Size},
                    {"Categories", binaryInfo.Categories},
                    {"WebURLs", binaryInfo.WebURLs},
                    {"SrcURLs", binaryInfo.SrcURLs},
                    {"Download URL", binaryInfo.DownloadURL},
                    {"B3SUM", binaryInfo.Bsum},
                    {"SHA256", binaryInfo.Shasum},
                    {"Build Date", binaryInfo.BuildDate},
                    {"Build Script", binaryInfo.BuildScript},
                    {"Build Log", binaryInfo.BuildLog},
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
                            prefix := "\033[48;5;4m" + field.label + "\033[0m"
                            if n > 0 {
                                prefix = "         "
                            }
                            fmt.Printf("%s: %s\n", prefix, str)
                        }
                    case []snapshot:
                        for n, snap := range v {
                            prefix := "\033[48;5;4m" + field.label + "\033[0m"
                            if n > 0 {
                                prefix = "         "
                            }
                            fmt.Printf("%s: %s %s\n", prefix, snap.Commit, ternary(snap.Version != "", "["+snap.Version+"]", ""))
                        }
                    default:
                        if v != "" && v != 0 {
                            fmt.Printf("\033[48;5;4m%s\033[0m: %v\n", field.label, v)
                        }
                    }
                }
            } else {
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
            }
            return nil
        },
    }
}

func findBinaryInfo(bEntry binaryEntry, uRepoIndex []binaryEntry) (binaryEntry, bool) {
	matchingBins, highestRank := findMatchingBins(bEntry, uRepoIndex)

	if len(matchingBins) == 0 {
		return binaryEntry{}, false
	}

	selectedBin := selectHighestRankedBin(matchingBins, highestRank)

	return selectedBin, true
}

func getBinaryInfo(config *Config, bEntry binaryEntry, uRepoIndex []binaryEntry) (*binaryEntry, error) {
	if instBEntry := bEntryOfinstalledBinary(filepath.Join(config.InstallDir, bEntry.Name)); bEntry.PkgId == "" && instBEntry.PkgId != "" {
		bEntry = instBEntry
	}

	binInfo, found := findBinaryInfo(bEntry, uRepoIndex)
	if found {
		return &binInfo, nil
	}

	return nil, fmt.Errorf("error: info for the requested binary ('%s') not found in any of the repository index files", parseBinaryEntry(bEntry, false))
}
