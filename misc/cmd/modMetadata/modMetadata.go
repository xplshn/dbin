// We modify the metadata of the repos to have the name fields of each binary reflect the directory they're in in the repo.
package main

import (
	"fmt"
	"github.com/goccy/go-json"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	minify "github.com/tdewolff/minify/v2"
	mjson "github.com/tdewolff/minify/v2/json"
)

type labeledString struct {
	mainURL           string
	fallbackURL       string
	label             string
	resolveToFinalURL bool
}

type Item struct {
	Name         string `json:"name"`
	B3sum        string `json:"b3sum,omitempty"`
	Sha256       string `json:"sha256,omitempty"`
	Description  string `json:"description,omitempty"`
	DownloadURL  string `json:"download_url,omitempty"`
	Size         string `json:"size,omitempty"`
	RepoURL      string `json:"repo_url,omitempty"`
	RepoAuthor   string `json:"repo_author,omitempty"`
	RepoInfo     string `json:"repo_info,omitempty"`
	RepoUpdated  string `json:"repo_updated,omitempty"`
	RepoReleased string `json:"repo_released,omitempty"`
	RepoVersion  string `json:"repo_version,omitempty"`
	RepoStars    string `json:"repo_stars,omitempty"`
	RepoLanguage string `json:"repo_language,omitempty"`
	RepoLicense  string `json:"repo_license,omitempty"`
	RepoTopics   string `json:"repo_topics,omitempty"`
	WebURL       string `json:"web_url,omitempty"`
	BuildScript  string `json:"build_script,omitempty"`
	ExtraBins    string `json:"extra_bins,omitempty"`
	BuildDate    string `json:"build_date,omitempty"`
	Note         string `json:"note,omitempty"`
	Bsum         string `json:"bsum,omitempty"`
	Shasum       string `json:"shasum,omitempty"`
	Category     string `json:"category,omitempty"`
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
	// Marshal JSON with indentation
	jsonData, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}

	// Write the pretty-printed JSON to the file
	err = ioutil.WriteFile(filename, jsonData, 0644)
	if err != nil {
		return err
	}

	// Minify and save the file
	return minifyJSON(filename, jsonData)
}

func minifyJSON(filename string, jsonData []byte) error {
	// Create a new minifier
	m := minify.New()
	m.AddFunc("application/json", mjson.Minify)

	// Minify the JSON data
	minifiedData, err := m.Bytes("application/json", jsonData)
	if err != nil {
		return err
	}

	// Create the minified file name
	minFilename := strings.TrimSuffix(filename, ".json") + ".min.json"

	// Write the minified data to a new file
	err = ioutil.WriteFile(minFilename, minifiedData, 0644)
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
