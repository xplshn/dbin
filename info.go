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
    var highestRank uint16 = 0
    var selectedBin BinaryInfo
    var found bool

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

            name, _ := binMap["pkg"].(string)
            pkgId, _ := binMap["pkg_id"].(string)
            version, _ := binMap["version"].(string)

            // Check if binary matches name
            if req.Name != "" && name != req.Name {
                continue
            }

            // If ID specified, check ID match
            if req.PkgId != "" && pkgId != req.PkgId {
                continue
            }

            // If version specified, check version match
            if req.Version != "" && version != req.Version {
                continue
            }

            // Track highest rank
            if rank, ok := binMap["rank"].(uint16); ok && rank > highestRank {
                highestRank = rank
                selectedBin = BinaryInfo{
                    Name:        name,
                    PrettyName:  binMap["pkg_name"].(string),
                    PkgId:       pkgId,
                    Description: binMap["description"].(string),
                    Note:        binMap["note"].(string),
                    Version:     version,
                    DownloadURL: binMap["download_url"].(string),
                    Size:        binMap["size"].(string),
                    Bsum:        binMap["bsum"].(string),
                    Shasum:      binMap["shasum"].(string),
                    BuildDate:   binMap["build_date"].(string),
                    SrcURL:      binMap["src_url"].(string),
                    WebURL:      binMap["homepage"].(string),
                    BuildScript: binMap["build_script"].(string),
                    BuildLog:    binMap["build_log"].(string),
                    Categories:  binMap["categories"].(string),
                    ExtraBins:   binMap["provides"].(string),
                    GhcrURL:     binMap["ghcr_url"].(string),
                    Rank:        rank,
                }
                found = true
            }
        }
    }

    return selectedBin, found
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
