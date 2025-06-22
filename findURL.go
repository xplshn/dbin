package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zeebo/errs"
)

func findMatchingBins(bEntry binaryEntry, uRepoIndex []binaryEntry) []binaryEntry {
	var matchingBins []binaryEntry

	for _, bin := range uRepoIndex {
		// Match based on the format hierarchy: name#id:version@repo, name#id@repo, name#id:version, name#id, name@repo, name
		matches := false
		if bin.Name == bEntry.Name {
			// name#id:version@repo
			if bEntry.PkgID != "" && bEntry.Version != "" && bEntry.Repository.Name != "" {
				if bin.PkgID == bEntry.PkgID && (bin.Version == bEntry.Version || hasMatchingSnapshot(&bin, bEntry.Version)) && bin.Repository.Name == bEntry.Repository.Name {
					matches = true
				}
				// name#id:version
			} else if bEntry.PkgID != "" && bEntry.Version != "" && bEntry.Repository.Name == "" {
				if bin.PkgID == bEntry.PkgID && (bin.Version == bEntry.Version || hasMatchingSnapshot(&bin, bEntry.Version)) {
					matches = true
				}
				// name#id@repo
			} else if bEntry.PkgID != "" && bEntry.Repository.Name != "" && bEntry.Version == "" {
				if bin.PkgID == bEntry.PkgID && bin.Repository.Name == bEntry.Repository.Name {
					matches = true
				}
				// name#id
			} else if bEntry.PkgID != "" && bEntry.Version == "" && bEntry.Repository.Name == "" {
				if bin.PkgID == bEntry.PkgID {
					matches = true
				}
				// name@repo
			} else if bEntry.PkgID == "" && bEntry.Repository.Name != "" && bEntry.Version == "" {
				if bin.Repository.Name == bEntry.Repository.Name {
					matches = true
				}
				// name
			} else if bEntry.PkgID == "" && bEntry.Version == "" && bEntry.Repository.Name == "" {
				matches = true
			}
		}

		if matches {
			matchingBins = append(matchingBins, bin)
		}
	}

	return matchingBins
}

// Helper function to check if a snapshot matches the requested version or commit, and if so, modify the DownloadURL
func hasMatchingSnapshot(bin *binaryEntry, version string) bool {
	if !strings.HasPrefix(bin.DownloadURL, "oci://") {
		return false
	}

	// First, check all snapshot commits
	for _, snap := range bin.Snapshots {
		if version == snap.Commit {
			// Modify the URL to use the snapshot's commit for OCI URLs
			idx := strings.LastIndex(bin.DownloadURL, ":")
			if idx != -1 {
				bin.DownloadURL = bin.DownloadURL[:idx+1] + snap.Commit
				bin.Bsum = "!no_check"
				bin.Version = snap.Version
			}
			return true
		}
	}

	// Then, check all snapshot versions
	for _, snap := range bin.Snapshots {
		if version == snap.Version {
			// Modify the URL to use the snapshot's commit for OCI URLs
			idx := strings.LastIndex(bin.DownloadURL, ":")
			if idx != -1 {
				bin.DownloadURL = bin.DownloadURL[:idx+1] + snap.Commit
				bin.Bsum = "!no_check"
				bin.Version = snap.Version
			}
			return true
		}
	}
	return false
}

func findURL(bEntries []binaryEntry, uRepoIndex []binaryEntry, config *config) ([]binaryEntry, error) {
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
		if bEntry.Bsum == "!no_check" {
			results = append(results, bEntry)
			if verbosityLevel >= extraVerbose {
				fmt.Printf("\033[2K\rSkipping resolution for \"%s\" (its Bsum was marked as \"!no_check\" by stringToBinaryEntry())\n", bEntry.Name)
			}
			allFailed = false
			continue
		} else {
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
