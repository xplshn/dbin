// info.go // This file implements binInfo, which `info` and `update` use //>
package main

import (
	"fmt"
	"path/filepath"
)

// BinaryInfo struct holds binary metadata used in main.go for the `info`, `update`, `list` functionality
type BinaryInfo struct {
	RealName    string `json:"pkg"`
	Name        string `json:"pkg_name"`
	PkgId       string `json:"pkg_id"`
	Description string `json:"description,omitempty"`
	Note        string `json:"note,omitempty"`
	Version     string `json:"version,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	Size        string `json:"size,omitempty"`
	Bsum        string `json:"bsum,omitempty"`   // BLAKE3
	Shasum      string `json:"shasum,omitempty"` // SHA256
	BuildDate   string `json:"build_date,omitempty"`
	SrcURL      string `json:"src_url,omitempty"`
	WebURL      string `json:"homepage,omitempty"`
	BuildScript string `json:"build_script,omitempty"`
	BuildLog    string `json:"build_log,omitempty"`
	Category    string `json:"category,omitempty"`
	ExtraBins   string `json:"provides,omitempty"`
}

// findBinaryInfo searches for binary metadata across multiple sections in the provided metadata map.
func findBinaryInfo(metadata map[string]interface{}, binaryName string) (BinaryInfo, bool) {
	for _, section := range metadata {
		// Each section is a list of binaries
		binaries, ok := section.([]interface{})
		if !ok {
			continue
		}

		// Iterate through each binary in the section
		for _, bin := range binaries {
			binMap, ok := bin.(map[string]interface{})
			if !ok {
				continue
			}

			name, nameOk := binMap["pkg_name"].(string)
			realName, realNameOk := binMap["pkg"].(string)

			if (nameOk && name == binaryName) || (realNameOk && realName == binaryName) {
				description, _ := binMap["description"].(string)
				note, _ := binMap["note"].(string)
				version, _ := binMap["version"].(string)
				downloadURL, _ := binMap["download_url"].(string)
				size, _ := binMap["size"].(string)
				bsum, _ := binMap["bsum"].(string)
				shasum, _ := binMap["shasum"].(string)
				buildDate, _ := binMap["build_date"].(string)
				srcURL, _ := binMap["src_url"].(string)
				webURL, _ := binMap["homepage"].(string)
				buildScript, _ := binMap["build_script"].(string)
				buildLog, _ := binMap["build_log"].(string)
				category, _ := binMap["category"].(string)
				extraBins, _ := binMap["provides"].(string)

				return BinaryInfo{
					RealName:    realName,
					Name:        name,
					Description: description,
					Note:        note,
					Version:     version,
					DownloadURL: downloadURL,
					Size:        size,
					Bsum:        bsum,
					Shasum:      shasum,
					BuildDate:   buildDate,
					SrcURL:      srcURL,
					WebURL:      webURL,
					BuildScript: buildScript,
					BuildLog:    buildLog,
					Category:    category,
					ExtraBins:   extraBins,
				}, true
			}
		}
	}
	return BinaryInfo{}, false
}

// getBinaryInfo retrieves binary metadata for the specified binary name by fetching and searching through the given metadata files
func getBinaryInfo(config *Config, binaryName string) (*BinaryInfo, error) {
	// Check the tracker file first
	realBinaryName, err := getFullName(filepath.Join(config.InstallDir, binaryName))
	if err == nil {
		binaryName = realBinaryName
	}

	var metadata map[string]interface{}
	for _, url := range config.MetadataURLs {
		var tempMetadata map[string]interface{}
		err := fetchJSON(url, &tempMetadata)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch and decode binary information from %s: %v", url, err)
		}
		metadata = tempMetadata

		binInfo, found := findBinaryInfo(metadata, binaryName)
		if found {
			return &binInfo, nil
		}
	}

	return nil, fmt.Errorf("error: info for the requested binary ('%s') not found in any of the metadata files", binaryName)
}
