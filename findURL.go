package main

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

func findMatchingBins(bEntry binaryEntry, uRepoIndex []binaryEntry) ([]binaryEntry, uint16) {
	var matchingBins []binaryEntry
	var highestRank uint16

	for _, bin := range uRepoIndex {
		// Basic match criteria (name and optional package ID)
		if bin.Name == bEntry.Name && (bEntry.PkgId == "" || bin.PkgId == bEntry.PkgId) {
			if bEntry.Version != "" {
				// Handle snapshot request: check if this binary has the requested snapshot
				for _, snap := range bin.Snapshots {
					// Match by commit or version
					if bEntry.Version == snap.Version || bEntry.Version == snap.Commit {
						// Modify the URL to use the snapshot's commit
						if strings.HasPrefix(bin.DownloadURL, "oci://") {
							// For OCI URLs, replace the tag part by locating the last colon.
							idx := strings.LastIndex(bin.DownloadURL, ":")
							if idx != -1 {
								// Everything before the tag remains intact; the new tag is the snapshot commit.
								bin.DownloadURL = bin.DownloadURL[:idx+1] + snap.Commit
								// Disable checksum verification
								bin.Bsum = "!no_check"
								bin.Version = snap.Version
							}
						}
					}

				}
			}
			matchingBins = append(matchingBins, bin)
			if bin.Rank > highestRank {
				highestRank = bin.Rank
			}
			break
		}
	}

	return matchingBins, highestRank
}

func selectHighestRankedBin(matchingBins []binaryEntry, highestRank uint16) binaryEntry {
	if len(matchingBins) == 1 {
		return matchingBins[0]
	}

	var nonGlibcBins []binaryEntry
	for _, bin := range matchingBins {
		if !strings.Contains(bin.PkgId, "glibc") {
			nonGlibcBins = append(nonGlibcBins, bin)
		}
	}

	if len(nonGlibcBins) > 0 {
		var selectedBin binaryEntry
		var highestNonGlibcRank uint16
		for _, bin := range nonGlibcBins {
			if bin.Rank > highestNonGlibcRank {
				highestNonGlibcRank = bin.Rank
				selectedBin = bin
			}
		}
		return selectedBin
	}

	for _, bin := range matchingBins {
		if bin.Rank == highestRank {
			return bin
		}
	}

	if highestRank == 0 {
		return matchingBins[0]
	}

	return binaryEntry{}
}

func findURL(config *Config, bEntries []binaryEntry, verbosityLevel Verbosity, uRepoIndex []binaryEntry) ([]binaryEntry, error) {
	var results []binaryEntry
	var allErrors []error
	allFailed := true

	for _, bEntry := range bEntries {
		parsedURL, err := url.ParseRequestURI(bEntry.Name)
		if err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
			if verbosityLevel >= extraVerbose {
				fmt.Printf("\033[2K\rFound \"%s\" is already a valid URL\n", bEntry.Name)
			}
			results = append(results, bEntry)
			allFailed = false
			continue
		}

		if instBEntry := bEntryOfinstalledBinary(filepath.Join(config.InstallDir, bEntry.Name)); instBEntry.Name != "" {
			bEntry = instBEntry
		}

		matchingBins, highestRank := findMatchingBins(bEntry, uRepoIndex)

		if len(matchingBins) == 0 {
			results = append(results, binaryEntry{
				Name:        bEntry.Name,
				DownloadURL: "!not_found",
				Bsum:        "!no_check",
			})
			allErrors = append(allErrors, fmt.Errorf("didn't find download URL for [%s]\n", parseBinaryEntry(bEntry, false)))
			continue
		}

		allFailed = false
		selectedBin := selectHighestRankedBin(matchingBins, highestRank)

		results = append(results, selectedBin)

		if verbosityLevel >= extraVerbose {
			fmt.Printf("\033[2K\rFound \"%s\" with id=%s version=%s\n", bEntry.Name, selectedBin.PkgId, selectedBin.Version)
		}
	}

	if allFailed {
		var errorMessages []string
		for _, e := range allErrors {
			errorMessages = append(errorMessages, e.Error())
		}
		return nil, fmt.Errorf(ternary(len(bEntries) != 1, "error: no valid download URLs found for any of the requested binaries.\n%s\n", "%s\n"), strings.Join(errorMessages, "\n"))
	}

	return results, nil
}
