package main

import (
	"fmt"
	"context"

	"github.com/urfave/cli/v3"
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
				return err
			}
			uRepoIndex, err := fetchRepoIndex(config)
            if err != nil {
                return err
            }
			if c.Bool("described") {
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
	}
}

func listBinaries(uRepoIndex []binaryEntry) ([]binaryEntry, error) {
	var allBinaries []binaryEntry

	for _, bin := range uRepoIndex {
		name, pkgId, version, description, rank := bin.Name, bin.PkgId, bin.Version, bin.Description, bin.Rank

		if name != "" {
			allBinaries = append(allBinaries, binaryEntry{
				Name:        name,
				PkgId:       pkgId,
				Version:     version,
				Description: description,
				Rank:        rank,
			})
		}
	}

	return allBinaries, nil
}
