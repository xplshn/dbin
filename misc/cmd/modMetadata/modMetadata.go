package main

import (
	"fmt"
	"github.com/goccy/go-json"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type labeledString struct {
	mainURL           string
	fallbackURL       string
	label             string
	resolveToFinalURL bool
}

type Item struct {
	Name         string `json:"name"`                    // Name of the entry
	B3sum        string `json:"b3sum,omitempty"`         // BLAKE3 CHECKSUM
	Sha256       string `json:"sha256,omitempty"`        // SHA256 CHECKSUM
	Description  string `json:"description,omitempty"`   // Description of the item
	DownloadURL  string `json:"download_url,omitempty"`  // URL to download the item
	Size         string `json:"size,omitempty"`          // Size of the item
	RepoURL      string `json:"repo_url,omitempty"`      // URL of the repository
	RepoAuthor   string `json:"repo_author,omitempty"`   // Author of the repository
	RepoInfo     string `json:"repo_info,omitempty"`     // Info about the repository
	RepoUpdated  string `json:"repo_updated,omitempty"`  // Last update date of the repo
	RepoReleased string `json:"repo_released,omitempty"` // Release date of the repo
	RepoVersion  string `json:"repo_version,omitempty"`  // Version of the item
	RepoStars    string `json:"repo_stars,omitempty"`    // Stars on the repo
	RepoLanguage string `json:"repo_language,omitempty"` // Language used in the repo
	RepoLicense  string `json:"repo_license,omitempty"`  // License of the repo
	RepoTopics   string `json:"repo_topics,omitempty"`   // Topics of the repo
	WebURL       string `json:"web_url,omitempty"`       // Website URL of the item
	BuildScript  string `json:"build_script,omitempty"`  // URL to the build script
	ExtraBins    string `json:"extra_bins,omitempty"`    // Extra binaries, if any
	BuildDate    string `json:"build_date,omitempty"`    // Build date of the item
	Note         string `json:"note,omitempty"`          // Additional notes
	// --- For compat with pkg.ajam.dev ---
	Bsum     string `json:"bsum,omitempty"`
	Shasum   string `json:"shasum,omitempty"`
	Category string `json:"category,omitempty"`
	// --- = ---
}

func urldecode(encoded string) (string, error) {
	return url.PathUnescape(encoded)
}

func processItems(items []Item, realArchs, validatedArchs []string, repo labeledString) []Item {
	for i, item := range items {
		// Map fields from new to old format
		if items[i].Shasum != "" || items[i].Bsum != "" {
			items[i].Shasum = items[i].Sha256 // direct mapping from "shasum"
			items[i].Bsum = items[i].B3sum    // direct mapping from "bsum"
		}

		// If resolveToFinalURL is false, skip URL transformation
		if !repo.resolveToFinalURL {
			continue
		}

		// Parse the download URL to get its path
		parsedURL, err := url.Parse(item.DownloadURL)
		if err != nil {
			continue
		}

		// Extract the path from the URL and remove leading slashes
		cleanPath := parsedURL.Path
		if strings.HasPrefix(cleanPath, "/") {
			cleanPath = cleanPath[1:]
		}

		// Remove the architecture-specific path from the download URL path
		for _, prefix := range append(realArchs, validatedArchs...) {
			if strings.HasPrefix(cleanPath, prefix) {
				cleanPath = strings.TrimPrefix(cleanPath, prefix+"/")
				break
			}
		}

		// Remove the repo's label
		if strings.HasPrefix(cleanPath, repo.label+"/") {
			cleanPath = strings.TrimPrefix(cleanPath, repo.label+"/")
		}

		// Set the correct `Name` field based on the path
		items[i].Name = cleanPath
	}
	return items
}

func downloadJSON(url string) ([]Item, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var items []Item
	err = json.Unmarshal(body, &items)
	if err != nil {
		return nil, err
	}

	return items, nil
}

func saveJSON(filename string, items []Item) error {
	jsonData, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filename, jsonData, 0644)
	if err != nil {
		return err
	}

	return nil
}

func downloadWithFallback(repo labeledString) ([]Item, error) {
	items, err := downloadJSON(repo.mainURL)
	if err == nil {
		fmt.Printf("Downloaded JSON from: %s\n", repo.mainURL)
		return items, nil
	}

	fmt.Printf("Error downloading from main URL %s: %v. Trying fallback URL...\n", repo.mainURL, err)
	items, err = downloadJSON(repo.fallbackURL)
	if err == nil {
		fmt.Printf("Downloaded JSON from fallback URL: %s\n", repo.fallbackURL)
		return items, nil
	}

	fmt.Printf("Error downloading from fallback URL %s: %v\n", repo.fallbackURL, err)
	return nil, err
}

func main() {
	validatedArchs := []string{"amd64_linux", "arm64_linux", "arm64_android"}
	realArchs := []string{"x86_64_Linux", "aarch64_Linux", "aarch64_arm64_Linux", "arm64_v8a_Android", "x64_Windows"}

	// Loop over the indices to access both validatedArchs and realArchs
	for i := range validatedArchs {
		arch := validatedArchs[i]
		realArch := realArchs[i]

		repos := []labeledString{
			{"https://bin.ajam.dev/" + arch + "/METADATA.json",
				"https://huggingface.co/datasets/Azathothas/Toolpacks-Snapshots/resolve/main/" + arch + "/METADATA.json?download=true",
				"Toolpacks", true},
			{"https://bin.ajam.dev/" + arch + "/Baseutils/METADATA.json",
				"https://huggingface.co/datasets/Azathothas/Toolpacks-Snapshots/resolve/main/Baseutils/METADATA.json?download=true",
				"Baseutils", true},
			{"https://pkg.ajam.dev/" + arch + "/METADATA.json?download=true",
				"https://pkg.ajam.dev/",
				"Toolpacks-extras", false}, // Skip URL path transformation for Toolpacks-extras
		}

		for _, repo := range repos {
			items, err := downloadWithFallback(repo)
			if err != nil {
				fmt.Printf("Error downloading JSON from both main and fallback URLs for repo %s: %v\n", repo.label, err)
				continue
			}

			save := func(outputFile string, processedItems []Item) {
				if err := saveJSON(outputFile, processedItems); err != nil {
					fmt.Printf("Error saving JSON to %s: %v\n", outputFile, err)
					return
				}
				fmt.Printf("Processed and saved to %s\n", outputFile)
			}

			processedItems := processItems(items, realArchs, validatedArchs, repo)

			// 0.FOURTH compat
			outputFile := fmt.Sprintf("%s.dbin_%s.json", repo.label, realArch)
			save(outputFile, processedItems)
			// New dbin
			outputFile = fmt.Sprintf("%s.dbin_%s.json", repo.label, arch)
			save(outputFile, processedItems)
		}
	}
}
