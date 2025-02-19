package main

import (
	"fmt"
	"path/filepath"
)

func findBinaryInfo(bEntry binaryEntry, uRepoIndex []binaryEntry) (binaryEntry, bool) {
	matchingBins, highestRank := findMatchingBins(bEntry, uRepoIndex)

	if len(matchingBins) == 0 {
		return binaryEntry{}, false
	}

	selectedBin := selectHighestRankedBin(matchingBins, highestRank)

	return selectedBin, true
}

func getBinaryInfo(config *Config, bEntry binaryEntry, uRepoIndex []binaryEntry) (*binaryEntry, error) {
	if instBEntry := bEntryOfinstalledBinary(filepath.Join(config.InstallDir, bEntry.Name)); bEntry.PkgId == "" && instBEntry.PkgId != "" {
		bEntry = instBEntry
	}

	binInfo, found := findBinaryInfo(bEntry, uRepoIndex)
	if found {
		return &binInfo, nil
	}

	return nil, fmt.Errorf("error: info for the requested binary ('%s') not found in any of the repository index files", parseBinaryEntry(bEntry, false))
}
