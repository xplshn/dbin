package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"
	"github.com/zeebo/errs"
)

var (
	errListBinariesFailed = errs.Class("list binaries failed")
)

// filterBEntries applies a filter function to a []binaryEntry
func filterBEntries(entries *[]binaryEntry, filterFunc func(binaryEntry) bool) {
	if entries == nil {
		return
	}

	filtered := make([]binaryEntry, 0, len(*entries))
	for _, entry := range *entries {
		if filterFunc(entry) {
			filtered = append(filtered, entry)
		}
	}
	*entries = filtered
}

func listCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List all available binaries",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "detailed",
				Aliases: []string{"d"},
				Usage:   "List binaries with their descriptions",
			},
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"repos", "r"},
				Usage:   "Filter binaries by repository name",
			},
		},
		Action: func(_ context.Context, c *cli.Command) error {
			config, err := loadConfig()
			if err != nil {
				return errListBinariesFailed.Wrap(err)
			}
			uRepoIndex, err := fetchRepoIndex(config)
			if err != nil {
				return errListBinariesFailed.Wrap(err)
			}
			if c.Bool("detailed") {
				return fSearch(config, []string{""}, uRepoIndex)
			}
			bEntries, err := listBinaries(uRepoIndex)
			if err != nil {
				return errListBinariesFailed.Wrap(err)
			}

			// Apply repository filter if specified
			if repoNames := c.String("repo"); repoNames != "" {
				repoSet := make(map[string]struct{})
				for _, repo := range strings.Split(repoNames, ",") {
					repoSet[strings.TrimSpace(repo)] = struct{}{}
				}
				filterBEntries(&bEntries, func(entry binaryEntry) bool {
					_, ok := repoSet[entry.Repository.Name]
					return ok
				})
			}

			for _, binary := range binaryEntriesToArrString(bEntries, true) {
				fmt.Println(binary)
			}
			return nil
		},
	}
}

func listBinaries(uRepoIndex []binaryEntry) ([]binaryEntry, error) {
	filterBEntries(&uRepoIndex, func(entry binaryEntry) bool {
		return entry.Name != "" //&& entry.Description != ""
	})

	if len(uRepoIndex) == 0 {
		return nil, errListBinariesFailed.New("no binaries found in the repository index")
	}

	return uRepoIndex, nil
}
