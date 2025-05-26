package main

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/zeebo/errs"
)

func findMatchingBins(bEntry binaryEntry, uRepoIndex []binaryEntry) []binaryEntry {
	var matchingBins []binaryEntry
	seenNames := make(map[string]bool) // Track seen names to pick only the first match

	for _, bin := range uRepoIndex {
		// Skip if we've already seen this name
		if seenNames[bin.Name] {
			continue
		}
		// Match criteria: name, optional package ID, optional version, and optional repository
		if bin.Name == bEntry.Name &&
			(bEntry.PkgID == "" || bin.PkgID == bEntry.PkgID) &&
			(bEntry.Version == "" || bin.Version == bEntry.Version || hasMatchingSnapshot(bin, bEntry.Version)) &&
			(bEntry.Repository.Name == "" || bin.Repository.Name == bEntry.Repository.Name) {
			matchingBins = append(matchingBins, bin)
			seenNames[bin.Name] = true // Mark this name as seen
		}
	}

	return matchingBins
}

// Helper function to check if a snapshot matches the requested version or commit
func hasMatchingSnapshot(bin binaryEntry, version string) bool {
	for _, snap := range bin.Snapshots {
		if version == snap.Version || version == snap.Commit {
			// Modify the URL to use the snapshot's commit for OCI URLs
			if strings.HasPrefix(bin.DownloadURL, "oci://") {
				idx := strings.LastIndex(bin.DownloadURL, ":")
				if idx != -1 {
					bin.DownloadURL = bin.DownloadURL[:idx+1] + snap.Commit
					bin.Bsum = "!no_check"
					bin.Version = snap.Version
				}
			}
			return true
		}
	}
	return false
}

func findURL(config *config, bEntries []binaryEntry, uRepoIndex []binaryEntry) ([]binaryEntry, error) {
	// Check for duplicate names in bEntries
	nameCount := make(map[string]int)
	for _, bEntry := range bEntries {
		nameCount[bEntry.Name]++
		if nameCount[bEntry.Name] > 1 {
			return nil, errs.New("duplicate binary name '%s' provided in input", bEntry.Name)
		}
	}

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

		// Check if the binary is installed and update bEntry with installed metadata if available
		if instBEntry := bEntryOfinstalledBinary(filepath.Join(config.InstallDir, bEntry.Name)); instBEntry.Name != "" {
			if bEntry.PkgID == "" {
				bEntry.PkgID = instBEntry.PkgID
			}
			if bEntry.Version == "" {
				bEntry.Version = instBEntry.Version
			}
			if bEntry.Repository.Name == "" {
				bEntry.Repository.Name = instBEntry.Repository.Name
			}
		}

		matchingBins := findMatchingBins(bEntry, uRepoIndex)

		if len(matchingBins) == 0 {
			results = append(results, binaryEntry{
				Name:        bEntry.Name,
				DownloadURL: "!not_found",
				Bsum:        "!no_check",
			})
			allErrors = append(allErrors, fmt.Errorf("didn't find download URL for [%s]", parseBinaryEntry(bEntry, false)))
			continue
		}

		allFailed = false

		// If a repository is specified, select the binary from that repository
		if bEntry.Repository.Name != "" {
			for _, bin := range matchingBins {
				if bin.Repository.Name == bEntry.Repository.Name {
					results = append(results, bin)
					if verbosityLevel >= extraVerbose {
						fmt.Printf("\033[2K\rFound \"%s\" with id=%s version=%s repo=%s\n", bEntry.Name, bin.PkgID, bin.Version, bin.Repository.Name)
					}
					break
				}
			}
			// If no match with the specified repo, add an error
			if len(results) == len(bEntries)-1 {
				results = append(results, binaryEntry{
					Name:        bEntry.Name,
					DownloadURL: "!not_found",
					Bsum:        "!no_check",
				})
				allErrors = append(allErrors, fmt.Errorf("no binary found for [%s] in repository %s", parseBinaryEntry(bEntry, false), bEntry.Repository.Name))
			}
			continue
		}

		// If no repository is specified, include the first matching binary
		results = append(results, matchingBins[0])
		if verbosityLevel >= extraVerbose {
			fmt.Printf("\033[2K\rFound \"%s\" with id=%s version=%s repo=%s\n", bEntry.Name, matchingBins[0].PkgID, matchingBins[0].Version, matchingBins[0].Repository.Name)
		}
	}

	if allFailed {
		var errorMessages []string
		for _, e := range allErrors {
			errorMessages = append(errorMessages, e.Error())
		}
		return nil, errs.New(ternary(len(bEntries) != 1, "no valid download URLs found for any of the requested binaries.\n%s\n", "%s\n"), strings.Join(errorMessages, "\n"))
	}

	return results, nil
}
