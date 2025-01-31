package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// findURL fetches the URLs and BLAKE3sum for the specified binaries using xattr instead of trackerFile.
func findURL(config *Config, binaryNames []string, verbosityLevel Verbosity, metadata map[string]interface{}) ([]string, []string, error) {
    var foundURLs []string
    var foundB3sum []string

    for _, binaryName := range binaryNames {
        // Check if binaryName is a valid URL
        parsedURL, err := url.ParseRequestURI(binaryName)
        if err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
            // If it's a valid URL, return it with the checksum set to "null"
            if verbosityLevel >= extraVerbose {
                fmt.Printf("\033[2K\rFound \"%s\" is already a valid URL", binaryName)
            }
            foundURLs = append(foundURLs, binaryName)
            foundB3sum = append(foundB3sum, "null")
            continue
        }

        // Check if the binaryName contains a section specifier
        parts := strings.Split(binaryName, "#")
        if len(parts) == 2 {
            section := parts[1]
            binaryName = parts[0]

            // Check if the section exists in the JSON data
            sectionData, ok := metadata[section]
            if !ok {
                return nil, nil, fmt.Errorf("error: section [%s] not found in JSON data", section)
            }

            // Search for the binary in the specified section
            binaries, ok := sectionData.([]interface{})
            if !ok {
                return nil, nil, fmt.Errorf("error: section [%s] does not contain a list of binaries", section)
            }

            for _, bin := range binaries {
                binMap, ok := bin.(map[string]interface{})
                if !ok {
                    continue
                }
                if binMap["pkg"] == binaryName {
                    foundURLs = append(foundURLs, binMap["download_url"].(string))
                    foundB3sum = append(foundB3sum, binMap["bsum"].(string))
                    if verbosityLevel >= extraVerbose {
                        fmt.Printf("\033[2K\rFound \"%s\" in section \"%s\"", binaryName, section)
                    }
                    break
                }
            }
            continue
        }

        // Try to get binary info from info.go logic
        fullBinaryName, err := getFullName(binaryName)
        if err == nil && fullBinaryName != "" {
            binInfo, err := getBinaryInfo(config, fullBinaryName, metadata)
            if err == nil && binInfo.DownloadURL != "" {
                // If the download_url (Source) is available, return it with BLAKE3sum
                if verbosityLevel >= extraVerbose {
                    fmt.Printf("\033[2K\rFound \"%s\" via the metadata files", binaryName)
                }
                foundURLs = append(foundURLs, binInfo.DownloadURL)
                foundB3sum = append(foundB3sum, binInfo.Bsum)
                continue
            }
        }

        // If no valid download_url found, proceed with HEAD requests on repositories
        found, repoURLs := false, config.RepoURLs
        for i, repository := range repoURLs {
            url := fmt.Sprintf("%s%s", repository, binaryName)

            // Show progress only in verbose modes
            if verbosityLevel >= normalVerbosity {
                fmt.Printf("\033[2K\r<%d/%d> | Checking \"%s\" in repository \"%s\"\r", i+1, len(repoURLs), binaryName, repository)
            }

            // Create a new request with the User-Agent header
            req, err := http.NewRequest("HEAD", url, nil)
            if err != nil {
                continue
            }
            req.Header.Set("User-Agent", "dbin")

            // Perform the request
            resp, err := http.DefaultClient.Do(req)
            if err == nil && resp.StatusCode == http.StatusOK {
                // If found, print message based on verbosity
                if verbosityLevel >= extraVerbose {
                    fmt.Printf("\033[2K\r<%d/%d> | Found \"%s\" at %s", i+1, len(repoURLs), binaryName, repository)
                }
                foundURLs = append(foundURLs, url)
                foundB3sum = append(foundB3sum, "!no_warn") // No BLAKE3sum if found this way
                found = true
                break
            }
        }

        // Cleanup last progress message if no URL was found
        if verbosityLevel >= normalVerbosity {
            fmt.Printf("\033[2K\r")
        }

        // Handle verbosity for error output
        if !found && verbosityLevel >= silentVerbosityWithErrors {
            return nil, nil, fmt.Errorf("error: didn't find the DOWNLOAD_URL for [%s]\n", binaryName)
        }
    }

    return foundURLs, foundB3sum, nil
}
