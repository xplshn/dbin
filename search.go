package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
)

func searchCommand() *cli.Command {
	return &cli.Command{
		Name:  "search",
		Usage: "Search for a binary by supplying one or more search terms",
		Flags: []cli.Flag{
			&cli.UintFlag{
				Name:    "limit",
				Aliases: []string{"l"},
				Usage:   "Set the limit of entries to be shown at once on the screen",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			config, err := loadConfig()
			if err != nil {
				return err
			}

			if uint(c.Uint("limit")) > 0 {
				config.Limit = uint(c.Uint("limit"))
			}

			uRepoIndex, err := fetchRepoIndex(config)
			if err != nil {
			    return err
			}
			return fSearch(config, c.Args().Slice(), uRepoIndex)
		},
	}
}

func fSearch(config *Config, searchTerms []string, uRepoIndex []binaryEntry) error {
	var results []binaryEntry
	for _, bin := range uRepoIndex {
		name, pkgId, version, description, rank := bin.Name, bin.PkgId, bin.Version, bin.Description, bin.Rank
		if name == "" || description == "" {
			continue
		}
		match := true
		for _, term := range searchTerms {
			if !strings.Contains(strings.ToLower(name), strings.ToLower(term)) &&
				!strings.Contains(strings.ToLower(description), strings.ToLower(term)) &&
				!strings.Contains(strings.ToLower(pkgId), strings.ToLower(term)) {
				match = false
				break
			}
		}
		if match {
			results = append(results, binaryEntry{
				Name:        name,
				PkgId:       pkgId,
				Version:     version,
				Description: description,
				Rank:        rank,
			})
		}
	}
	if len(results) == 0 {
		return fmt.Errorf("no matching binaries found for '%s'",
			strings.Join(searchTerms, " "))
	} else if uint(len(results)) > config.Limit {
		return fmt.Errorf("too many matching binaries (+%d. [Use --limit or -l before your query]) found for '%s'",
			len(results), strings.Join(searchTerms, " "))
	}
	disableTruncation := config.DisableTruncation
	for _, result := range results {
		prefix := "[-]"
		if bEntryOfinstalledBinary(filepath.Join(config.InstallDir, filepath.Base(result.Name))).PkgId == result.PkgId {
			prefix = "[i]"
		}
		truncatePrintf(disableTruncation, "%s %s - %s\n",
			prefix, parseBinaryEntry(result, true), result.Description)
	}
	return nil
}
