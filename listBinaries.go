package main

import (
	"fmt"
	"context"

	"github.com/urfave/cli/v3"
	"github.com/zeebo/errs"
)

var (
	errListBinariesFailed = errs.Class("list binaries failed")
)

func listCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List all available binaries",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "described",
				Usage: "List binaries with descriptions",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			config, err := loadConfig()
			if err != nil {
				return errListBinariesFailed.Wrap(err)
			}
			uRepoIndex, err := fetchRepoIndex(config)
            if err != nil {
                return errListBinariesFailed.Wrap(err)
            }
			if c.Bool("described") {
				return fSearch(config, []string{""}, uRepoIndex)
			}
			bEntries, err := listBinaries(uRepoIndex)
			if err != nil {
				return errListBinariesFailed.Wrap(err)
			}
			for _, binary := range binaryEntriesToArrString(bEntries, true) {
				fmt.Println(binary)
			}
			return nil
		},
	}
}

func listBinaries(uRepoIndex []binaryEntry) ([]binaryEntry, error) {
	var allBinaries []binaryEntry

	for _, bin := range uRepoIndex {
		name, pkgID, version, description, rank := bin.Name, bin.PkgID, bin.Version, bin.Description, bin.Rank

		if name != "" {
			allBinaries = append(allBinaries, binaryEntry{
				Name:        name,
				PkgID:       pkgID,
				Version:     version,
				Description: description,
				Rank:        rank,
			})
		}
	}

	if len(allBinaries) == 0 {
		return nil, errListBinariesFailed.New("no binaries found in the repository index")
	}

	return allBinaries, nil
}
