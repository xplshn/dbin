package main

import (
	"fmt"
	"path/filepath"
)

type BinaryInfo struct {
	Name        string `json:"pkg"`
	PrettyName  string `json:"pkg_name"`
	PkgId       string `json:"pkg_id"`
	Description string `json:"description,omitempty"`
	Note        string `json:"note,omitempty"`
	Version     string `json:"version,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	Size        string `json:"size,omitempty"`
	Bsum        string `json:"bsum,omitempty"`
	Shasum      string `json:"shasum,omitempty"`
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

func findBinaryInfo(bEntry binaryEntry, metadata map[string]interface{}) (BinaryInfo, bool) {
	var matchingBins []map[string]interface{}
	var highestRank uint16 = 0

	// Search through all sections
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

			// Check if binary matches name
			if bEntry.Name != "" && binMap["pkg"].(string) != bEntry.Name {
				continue
			}

			// If ID specified, check ID match
			if bEntry.PkgId != "" && binMap["pkg_id"].(string) != bEntry.PkgId {
				continue
			}

			// If version specified, check version match
			if bEntry.Version != "" && binMap["version"].(string) != bEntry.Version {
				continue
			}

			matchingBins = append(matchingBins, binMap)

			// Track highest rank
			if rank, ok := binMap["rank"].(uint16); ok && rank > highestRank {
				highestRank = rank
			}
		}
	}

	// If matches found, select appropriate one
	if len(matchingBins) > 0 {
		var selectedBin map[string]interface{}

		if len(matchingBins) == 1 {
			selectedBin = matchingBins[0]
		} else {
			// Multiple matches - select highest rank
			for _, bin := range matchingBins {
				if rank, ok := bin["rank"].(uint16); ok && rank == highestRank {
					selectedBin = bin
					break
				}
			}
		}

		prettyName, _ := selectedBin["pkg_name"].(string)
		description, _ := selectedBin["description"].(string)
		note, _ := selectedBin["note"].(string)
		downloadURL, _ := selectedBin["download_url"].(string)
		size, _ := selectedBin["size"].(string)
		bsum, _ := selectedBin["bsum"].(string)
		shasum, _ := selectedBin["shasum"].(string)
		buildDate, _ := selectedBin["build_date"].(string)
		srcURL, _ := selectedBin["src_url"].(string)
		webURL, _ := selectedBin["homepage"].(string)
		buildScript, _ := selectedBin["build_script"].(string)
		buildLog, _ := selectedBin["build_log"].(string)
		categories, _ := selectedBin["categories"].(string)
		extraBins, _ := selectedBin["provides"].(string)
		ghcrURL, _ := selectedBin["ghcr_url"].(string)
		rank, _ := selectedBin["rank"].(uint16)

		return BinaryInfo{
			Name:        selectedBin["pkg"].(string),
			PrettyName:  prettyName,
			PkgId:       selectedBin["pkg_id"].(string),
			Description: description,
			Note:        note,
			Version:     selectedBin["version"].(string),
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

	return BinaryInfo{}, false
}

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
