// info.go // This file implements binInfo, which `info` and `update` use //>
package main

import (
	"fmt"
	"path/filepath"
)

// BinaryInfo struct holds binary metadata used in main.go for the `info`, `update`, `list` functionality
type BinaryInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Repo        string `json:"repo_url"`
	ModTime     string `json:"build_date"`
	Version     string `json:"repo_version"`
	Updated     string `json:"repo_updated"`
	Size        string `json:"size"`
	Extras      string `json:"extra_bins"`
	Note        string `json:"note"`
	Source      string `json:"download_url"`
	B3sum       string `json:"b3sum"`
	Sha256      string `json:"sha256"`
}

// findBinaryInfo searches for binary metadata in the provided metadata slice.
func findBinaryInfo(metadata []map[string]interface{}, binaryName string) (BinaryInfo, bool) {
	for _, binMap := range metadata {
		name, nameOk := binMap["name"].(string)
		if nameOk && name == binaryName {
			description, _ := binMap["description"].(string)
			repo, _ := binMap["repo_url"].(string)
			buildDate, _ := binMap["build_date"].(string)
			version, _ := binMap["repo_version"].(string)
			updated, _ := binMap["repo_updated"].(string)
			size, _ := binMap["size"].(string)
			extras, _ := binMap["extra_bins"].(string)
			note, _ := binMap["note"].(string)
			source, _ := binMap["download_url"].(string)
			b3s, _ := binMap["b3sum"].(string)
			sha256, _ := binMap["sha256"].(string)

			return BinaryInfo{
				Name:        name,
				Description: description,
				Repo:        repo,
				ModTime:     buildDate,
				Version:     version,
				Updated:     updated,
				Size:        size,
				Extras:      extras,
				Note:        note,
				Source:      source,
				B3sum:       b3s,
				Sha256:      sha256,
			}, true
		}
	}
	return BinaryInfo{}, false
}

// getBinaryInfo retrieves binary metadata for the specified binary name by fetching and searching through multiple JSON files.
func getBinaryInfo(trackerFile, binaryName string, metadataURLs []string) (*BinaryInfo, error) {
	// Check the tracker file first
	realBinaryName, err := getBinaryNameFromTrackerFile(trackerFile, filepath.Base(binaryName))
	if err == nil {
		binaryName = realBinaryName
	}

	var metadata []map[string]interface{}
	for _, url := range metadataURLs {
		var tempMetadata []map[string]interface{}
		err := fetchJSON(url, &tempMetadata)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch and decode binary information from %s: %v", url, err)
		}
		metadata = append(metadata, tempMetadata...)
	}

	binInfo, found := findBinaryInfo(metadata, binaryName)
	if !found {
		return nil, fmt.Errorf("error: info for the requested binary ('%s') not found in any of the metadata files", binaryName)
	}

	return &binInfo, nil
}
