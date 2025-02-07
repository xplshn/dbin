package main

import (
	"fmt"
	"path/filepath"
)

type BinaryInfo struct {
	Name        string   `json:"pkg"`
	PrettyName  string   `json:"pkg_name"`
	PkgId       string   `json:"pkg_id"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version,omitempty"`
	DownloadURL string   `json:"download_url,omitempty"`
	Size        string   `json:"size,omitempty"`
	Bsum        string   `json:"bsum,omitempty"`
	Shasum      string   `json:"shasum,omitempty"`
	BuildDate   string   `json:"build_date,omitempty"`
	BuildScript string   `json:"build_script,omitempty"`
	BuildLog    string   `json:"build_log,omitempty"`
	Categories  string   `json:"categories,omitempty"`
	ExtraBins   string   `json:"provides,omitempty"`
	GhcrBlob    string   `json:"ghcr_blob,omitempty"`
	Rank        uint16   `json:"rank,omitempty"`
	Notes       []string `json:"notes,omitempty"`
	SrcURLs     []string `json:"src_urls,omitempty"`
	WebURLs     []string `json:"web_urls,omitempty"`
}

func findBinaryInfo(bEntry binaryEntry, metadata map[string]interface{}) (BinaryInfo, bool) {
	matchingBins, highestRank := findMatchingBins(bEntry, metadata)

	if len(matchingBins) == 0 {
		return BinaryInfo{}, false
	}

	selectedBin := selectHighestRankedBin(matchingBins, highestRank)

	return populateBinaryInfo(selectedBin), true
}

func findMatchingBins(bEntry binaryEntry, metadata map[string]interface{}) ([]map[string]interface{}, uint16) {
	var matchingBins []map[string]interface{}
	var highestRank uint16

	for _, section := range metadata {
		if binaries, ok := section.([]interface{}); ok {
			for _, bin := range binaries {
				if binMap, ok := bin.(map[string]interface{}); ok && matchesEntry(bEntry, binMap) {
					matchingBins = append(matchingBins, binMap)
					if rank, ok := binMap["rank"].(uint16); ok && rank > highestRank {
						highestRank = rank
					}
				}
			}
		}
	}

	return matchingBins, highestRank
}

func selectHighestRankedBin(matchingBins []map[string]interface{}, highestRank uint16) map[string]interface{} {
	if len(matchingBins) == 1 {
		return matchingBins[0]
	}

	for _, bin := range matchingBins {
		if rank, ok := bin["rank"].(uint16); ok && rank == highestRank {
			return bin
		}
	}

	if highestRank == 0 {
		return matchingBins[0]
	}

	return nil
}

func populateBinaryInfo(binMap map[string]interface{}) BinaryInfo {
	getString := func(key string) string {
		if val, ok := binMap[key].(string); ok {
			return val
		}
		return ""
	}

	getStringSlice := func(key string) []string {
		if val, ok := binMap[key]; ok {
			switch v := val.(type) {
				case []interface{}:
					// If the value is a slice of interfaces, convert each to a string
					strSlice := make([]string, len(v))
					for i, item := range v {
						if str, ok := item.(string); ok {
							strSlice[i] = str
						}
					}
					return strSlice
			}
		}
		return []string{}
	}

	getUint16 := func(key string) uint16 {
		if val, ok := binMap[key].(uint16); ok {
			return val
		}
		return 0
	}

	return BinaryInfo{
		Name:        getString("pkg"),
		PrettyName:  getString("pkg_name"),
		PkgId:       getString("pkg_id"),
		Description: getString("description"),
		Version:     getString("version"),
		DownloadURL: getString("download_url"),
		Size:        getString("size"),
		Bsum:        getString("bsum"),
		Shasum:      getString("shasum"),
		BuildDate:   getString("build_date"),
		BuildScript: getString("build_script"),
		BuildLog:    getString("build_log"),
		Categories:  getString("categories"),
		ExtraBins:   getString("provides"),
		GhcrBlob:    getString("ghcr_blob"),
		SrcURLs:     getStringSlice("src_urls"),
		WebURLs:     getStringSlice("web_urls"),
		Notes:       getStringSlice("notes"),
		Rank:        getUint16("rank"),
	}
}

func getBinaryInfo(config *Config, bEntry binaryEntry, metadata map[string]interface{}) (*BinaryInfo, error) {
	realBinaryName, err := getFullName(filepath.Join(config.InstallDir, bEntry.Name))
	if err == nil && filepath.Base(bEntry.Name) != realBinaryName {
		bEntry.Name = realBinaryName
	}

	binInfo, found := findBinaryInfo(bEntry, metadata)
	if found {
		return &binInfo, nil
	}

	return nil, fmt.Errorf("error: info for the requested binary ('%s') not found in any of the metadata files", parseBinaryEntry(bEntry, false))
}
