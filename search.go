package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

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
		return fmt.Errorf("no matching binaries found for '%s'", strings.Join(searchTerms, " "))
	} else if uint(len(results)) > config.Limit {
		return fmt.Errorf("too many matching binaries (+%d. [Use --limit before your query]) found for '%s'",
			config.Limit, strings.Join(searchTerms, " "))
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
