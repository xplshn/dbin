package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
	"github.com/zeebo/errs"
)

var (
	errSearchFailed = errs.Class("search failed")
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
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"repos", "r"},
				Usage:   "Filter binaries by repository name, comma-separated",
			},
		},
		Action: func(_ context.Context, c *cli.Command) error {
			config, err := loadConfig()
			if err != nil {
				return errSearchFailed.Wrap(err)
			}

			if uint(c.Uint("limit")) > 0 {
				config.Limit = uint(c.Uint("limit"))
			}

			uRepoIndex, err := fetchRepoIndex(config)
			if err != nil {
				return errSearchFailed.Wrap(err)
			}

			// Apply repository filter if specified
			if repoNames := c.String("repo"); repoNames != "" {
				repoSet := make(map[string]struct{})
				for _, repo := range strings.Split(repoNames, ",") {
					repoSet[strings.TrimSpace(repo)] = struct{}{}
				}
				// In-line filter to avoid dependency on list.go's filterBEntries
				filtered := make([]binaryEntry, 0, len(uRepoIndex))
				for _, entry := range uRepoIndex {
					if _, ok := repoSet[entry.Repository.Name]; ok {
						filtered = append(filtered, entry)
					}
				}
				uRepoIndex = filtered
			}

			return fSearch(config, c.Args().Slice(), uRepoIndex)
		},
	}
}

func fSearch(config *config, searchTerms []string, uRepoIndex []binaryEntry) error {
	var results []binaryEntry
	for _, bin := range uRepoIndex {
		name, pkgID, version, description, rank, repo := bin.Name, bin.PkgID, bin.Version, bin.Description, bin.Rank, bin.Repository
		if name == "" || description == "" {
			continue
		}
		match := true
		for _, term := range searchTerms {
			if !strings.Contains(strings.ToLower(name), strings.ToLower(term)) &&
				!strings.Contains(strings.ToLower(description), strings.ToLower(term)) &&
				!strings.Contains(strings.ToLower(pkgID), strings.ToLower(term)) {
				match = false
				break
			}
		}
		if match {
			results = append(results, binaryEntry{
				Name:        name,
				PkgID:       pkgID,
				Version:     version,
				Description: description,
				Rank:        rank,
				Repository:  repo,
			})
		}
	}
	if len(results) == 0 {
		return errSearchFailed.New("no matching binaries found for '%s'", strings.Join(searchTerms, " "))
	} else if uint(len(results)) > config.Limit {
		return errSearchFailed.New("too many matching binaries (+%d. [Use --limit or -l before your query]) found for '%s'", len(results), strings.Join(searchTerms, " "))
	}
	disableTruncation := config.DisableTruncation
	for _, result := range results {
		prefix := "[-]"
		if bEntryOfinstalledBinary(filepath.Join(config.InstallDir, filepath.Base(result.Name))).PkgID == result.PkgID {
			prefix = "[i]"
		} else if _, err := isCached(config, result); err == nil {
			prefix = "[c]"
		}
		truncatePrintf(disableTruncation, "%s %s - %s\n",
			prefix, parseBinaryEntry(result, true), result.Description)
	}
	return nil
}
