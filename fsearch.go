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
	Rank        uint16
}

// fSearch searches for binaries based on the given search term
func fSearch(config *Config, searchTerm string, metadata map[string]interface{}) error {
	var results []binaryEntry

	// Gather all matching binaries across sections
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

			// Skip if missing required fields
			if name == "" || description == "" {
				continue
			}

			// Check if matches search term
			if strings.Contains(strings.ToLower(name), strings.ToLower(searchTerm)) ||
				strings.Contains(strings.ToLower(description), strings.ToLower(searchTerm)) {

				results = append(results, binaryEntry{
					Name:        name,
					PkgId:       pkgId,
					Version:     version,
					Description: description,
					Rank:        rank,
				})
			}
		}
	}

	// Filter results
	filteredResults := make([]binaryEntry, 0)
	for _, result := range results {
		ext := strings.ToLower(filepath.Ext(result.Name))
		base := filepath.Base(result.Name)

		if !contains(excludedFileTypes, ext) && !contains(excludedFileNames, base) {
			filteredResults = append(filteredResults, result)
		}
	}

	// Check result count
	if len(filteredResults) == 0 {
		return fmt.Errorf("no matching binaries found for '%s'", searchTerm)
	} else if uint(len(filteredResults)) > config.Limit {
		return fmt.Errorf("too many matching binaries (+%d. [Use --limit before your query]) found for '%s'",
			config.Limit, searchTerm)
	}

	// Print results
	installDir := config.InstallDir
	disableTruncation := config.DisableTruncation

	for _, result := range filteredResults {
		// Determine installation status
		prefix := "[-]"
		if fileExists(filepath.Join(installDir, filepath.Base(result.Name))) {
			prefix = "[i]"
		}

		// Format version info in gray
		versionInfo := ""
		if result.PkgId != "" {
			versionInfo = fmt.Sprintf("\033[94m#%s", result.PkgId)
			if result.Version != "" {
				versionInfo += fmt.Sprintf("\033[90m:%s", result.Version)
			}
			versionInfo += "\033[0m"
		}

		truncatePrintf(disableTruncation, "%s %s%s - %s\n",
			prefix, result.Name, versionInfo, result.Description)
	}

	return nil
}
