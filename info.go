package main

import (
	"fmt"
	"path/filepath"
)

// BinaryInfo struct holds binary metadata used in main.go for the `info`, `update`, `list` functionality
type BinaryInfo struct {
	Name        string `json:"pkg"`
	PrettyName  string `json:"pkg_name"`
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
	Categories  string `json:"categories,omitempty"`
	ExtraBins   string `json:"provides,omitempty"`
	GhcrURL     string `json:"ghcr_url,omitempty"`
	Rank        uint16 `json:"rank,omitempty"`
}

// findBinaryInfo searches for binary metadata across multiple sections in the provided metadata map.
func findBinaryInfo(req binaryEntry, metadata map[string]interface{}) (BinaryInfo, bool) {
	for _, section := range metadata {
		binaries, ok := section.([]interface{})
		if !ok {
			continue
		}

		for _, bin := range binaries {
			binMap, ok := bin.(map[string]interface{})
			if !ok {
				continue
			}

			name, nameOk := binMap["pkg"].(string)
			pkgId, pkgIdOk := binMap["pkg_id"].(string)
			version, _ := binMap["version"].(string)

			if !nameOk || name != req.Name {
				continue
			}

			if req.PkgId != "" {
				if !pkgIdOk || pkgId != req.PkgId {
					continue
				}
			}

			if req.Version != "" && version != req.Version {
				continue
			}

			prettyName, _ := binMap["pkg_name"].(string)
			description, _ := binMap["description"].(string)
			note, _ := binMap["note"].(string)
			downloadURL, _ := binMap["download_url"].(string)
			size, _ := binMap["size"].(string)
			bsum, _ := binMap["bsum"].(string)
			shasum, _ := binMap["shasum"].(string)
			buildDate, _ := binMap["build_date"].(string)
			srcURL, _ := binMap["src_url"].(string)
			webURL, _ := binMap["homepage"].(string)
			buildScript, _ := binMap["build_script"].(string)
			buildLog, _ := binMap["build_log"].(string)
			categories, _ := binMap["categories"].(string)
			extraBins, _ := binMap["provides"].(string)
			ghcrURL, _ := binMap["ghcr_url"].(string)
			rank, _ := binMap["rank"].(uint16)

			return BinaryInfo{
				Name:        name,
				PrettyName:  prettyName,
				PkgId:       pkgId,
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
				Categories:  categories,
				ExtraBins:   extraBins,
				GhcrURL:     ghcrURL,
				Rank:        rank,
			}, true
		}
	}
	return BinaryInfo{}, false
}

// getBinaryInfo retrieves binary metadata for the specified binary name by fetching and searching through the given metadata files.
func getBinaryInfo(config *Config, bEntry binaryEntry, metadata map[string]interface{}) (*BinaryInfo, error) {
	realBinaryName, err := getFullName(filepath.Join(config.InstallDir, bEntry.Name))
	if err == nil {
		if filepath.Base(bEntry.Name) != realBinaryName {
			bEntry.Name = realBinaryName
		}
	}

	binInfo, found := findBinaryInfo(bEntry, metadata)
	if found {
		return &binInfo, nil
	}

	return nil, fmt.Errorf("error: info for the requested binary ('%s') not found in any of the metadata files", parseBinaryEntry(bEntry, false))
}
