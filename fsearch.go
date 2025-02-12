package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

type binaryEntry struct {
	Name        string
	PkgId       string
	Version     string
	Description string
	Path        string
	Rank        uint16
}

func fSearch(config *Config, searchTerms []string, metadata map[string]interface{}) error {
	var results []binaryEntry

	for _, section := range metadata {
		binList, ok := section.([]interface{})
		if !ok {
			continue
		}

		for _, bin := range binList {
			binMap, ok := bin.(map[string]interface{})
			if !ok {
				continue
			}

			name, _ := binMap["pkg"].(string)
			pkgId, _ := binMap["pkg_id"].(string)
			version, _ := binMap["version"].(string)
			description, _ := binMap["description"].(string)
			rank, _ := binMap["rank"].(uint16)

			if name == "" || description == "" {
				continue
			}

			// Check if all search terms are contained in either name or description
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
					Path:        filepath.Join(config.InstallDir, filepath.Base(name)),
					Rank:        rank,
				})
			}
		}
	}

	filteredResults := make([]binaryEntry, 0)
	for _, result := range results {
		ext := strings.ToLower(filepath.Ext(result.Name))
		base := filepath.Base(result.Name)

		if !contains(excludedFileTypes, ext) && !contains(excludedFileNames, base) {
			filteredResults = append(filteredResults, result)
		}
	}

	if len(filteredResults) == 0 {
		return fmt.Errorf("no matching binaries found for '%s'", strings.Join(searchTerms, " "))
	} else if uint(len(filteredResults)) > config.Limit {
		return fmt.Errorf("too many matching binaries (+%d. [Use --limit before your query]) found for '%s'",
			config.Limit, strings.Join(searchTerms, " "))
	}

	disableTruncation := config.DisableTruncation

	for _, result := range filteredResults {
		prefix := "[-]"
		if bEntryOfinstalledBinary(result.Path).PkgId == result.PkgId {
			prefix = "[i]"
		}

		truncatePrintf(disableTruncation, "%s %s - %s\n",
			prefix, parseBinaryEntry(result, true), result.Description)
	}

	return nil
}
