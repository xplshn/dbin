package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/goccy/go-json"
	minify "github.com/tdewolff/minify/v2"
	mjson "github.com/tdewolff/minify/v2/json"
)

type labeledString struct {
	mainURL           string
	fallbackURL       string
	resolveToFinalURL bool
}

var repoLabels = map[string]string{
	"bin":  "Toolpacks",
	"pkg":  "Toolpacks-extras",
	"base": "Baseutils",
}

type Item struct {
	RealName        string   `json:"pkg"`              // should map to bin
	Name            string   `json:"pkg_name"`         // should map to bin_name
	BinId           string   `json:"pkg_id,omitempty"` // should map to bin_id
	Icon            string   `json:"icon,omitempty"`
	Description     string   `json:"description,omitempty"`
	RichDescription string   `json:"rich_description,omitempty"`
	Screenshots     []string `json:"screenshots,omitempty"`
	Version         string   `json:"version,omitempty"`
	DownloadURL     string   `json:"download_url,omitempty"`
	Size            string   `json:"size,omitempty"`
	Bsum            string   `json:"bsum,omitempty"`   // BLAKE3
	Shasum          string   `json:"shasum,omitempty"` // SHA256
	BuildDate       string   `json:"build_date,omitempty"`
	SrcURL          string   `json:"src_url,omitempty"`
	WebURL          string   `json:"homepage,omitempty"` // should map to web_url
	BuildScript     string   `json:"build_script,omitempty"`
	BuildLog        string   `json:"build_log,omitempty"`
	Category        string   `json:"category,omitempty"`
	ExtraBins       string   `json:"provides,omitempty"` // should map to extra_bins
	Note            string   `json:"note,omitempty"`
	Appstream       string   `json:"appstream,omitempty"`
}

type Metadata struct {
	Bin  []Item `json:"bin"`
	Pkg  []Item `json:"pkg"`
	Base []Item `json:"base"`
}

type OutputMetadata struct {
	Bin  []OutputItem `json:"bin"`
	Pkg  []OutputItem `json:"pkg"`
	Base []OutputItem `json:"base"`
}

type OutputItem struct {
	Bin             string   `json:"bin"`
	BinName         string   `json:"bin_name"`
	BinId           string   `json:"bin_id,omitempty"`
	Icon            string   `json:"icon,omitempty"`
	Description     string   `json:"description,omitempty"`
	RichDescription string   `json:"rich_description,omitempty"`
	Screenshots     []string `json:"screenshots,omitempty"`
	Version         string   `json:"version,omitempty"`
	DownloadURL     string   `json:"download_url,omitempty"`
	Size            string   `json:"size,omitempty"`
	Bsum            string   `json:"bsum,omitempty"`
	Shasum          string   `json:"shasum,omitempty"`
	BuildDate       string   `json:"build_date,omitempty"`
	SrcURL          string   `json:"src_url,omitempty"`
	WebURL          string   `json:"web_url,omitempty"`
	BuildScript     string   `json:"build_script,omitempty"`
	BuildLog        string   `json:"build_log,omitempty"`
	Category        string   `json:"category,omitempty"`
	ExtraBins       string   `json:"extra_bins,omitempty"`
	Note            string   `json:"note,omitempty"`
	Appstream       string   `json:"appstream,omitempty"`
}

func convertItem(item Item) OutputItem {
	return OutputItem{
		Bin:             item.RealName,
		BinName:         item.Name,
		BinId:           item.BinId,
		Icon:            item.Icon,
		Description:     item.Description,
		RichDescription: item.RichDescription,
		Screenshots:     item.Screenshots,
		Version:         item.Version,
		DownloadURL:     item.DownloadURL,
		Size:            item.Size,
		Bsum:            item.Bsum,
		Shasum:          item.Shasum,
		BuildDate:       item.BuildDate,
		SrcURL:          item.SrcURL,
		WebURL:          item.WebURL,
		BuildScript:     item.BuildScript,
		BuildLog:        item.BuildLog,
		Category:        item.Category,
		ExtraBins:       item.ExtraBins,
		Note:            item.Note,
		Appstream:       item.Appstream,
	}
}

func convertItems(items []Item) []OutputItem {
	var outputItems []OutputItem
	for _, item := range items {
		outputItems = append(outputItems, convertItem(item))
	}
	return outputItems
}

func convertMetadata(metadata Metadata) OutputMetadata {
	return OutputMetadata{
		Bin:  convertItems(metadata.Bin),
		Pkg:  convertItems(metadata.Pkg),
		Base: convertItems(metadata.Base),
	}
}

// Function to process items by removing arch-specific and repo-label prefixes
func processItems(items []Item, realArchs, validatedArchs []string, repo labeledString, section string) []Item {
	for i, item := range items {
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

		// Remove the repo's label based on the section
		if label, ok := repoLabels[section]; ok {
			if strings.HasPrefix(cleanPath, label+"/") {
				cleanPath = strings.TrimPrefix(cleanPath, label+"/")
			}
		}

		// Set the correct `RealName` field based on the path
		items[i].RealName = cleanPath
	}
	return items
}

func downloadJSON(url string) (Metadata, error) {
	resp, err := http.Get(url)
	if err != nil {
		return Metadata{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Metadata{}, err
	}

	var metadata Metadata
	err = json.Unmarshal(body, &metadata)
	if err != nil {
		return Metadata{}, err
	}

	return metadata, nil
}

func saveJSON(filename string, metadata OutputMetadata) error {
	// Marshal JSON with indentation
	jsonData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	// Write the pretty-printed JSON to the file
	err = os.WriteFile(filename, jsonData, 0644)
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
	err = os.WriteFile(minFilename, minifiedData, 0644)
	if err != nil {
		return err
	}

	return nil
}

func downloadWithFallback(repo labeledString) (Metadata, error) {
	metadata, err := downloadJSON(repo.mainURL)
	if err == nil {
		fmt.Printf("Downloaded JSON from: %s\n", repo.mainURL)
		return metadata, nil
	}

	fmt.Printf("Error downloading from main URL %s: %v. Trying fallback URL...\n", repo.mainURL, err)
	metadata, err = downloadJSON(repo.fallbackURL)
	if err == nil {
		fmt.Printf("Downloaded JSON from fallback URL: %s\n", repo.fallbackURL)
		return metadata, nil
	}

	fmt.Printf("Error downloading from fallback URL %s: %v\n", repo.fallbackURL, err)
	return Metadata{}, err
}

func main() {
	validatedArchs := []string{"amd64_linux", "arm64_linux"}
	realArchs := []string{"x86_64_Linux", "x86_64-Linux", "aarch64_Linux", "aarch64-Linux", "aarch64_arm64_Linux", "aarch64_arm64-Linux", "x86_64", "x64_Windows"}

	for i := range validatedArchs {
		arch := validatedArchs[i]

		repos := []labeledString{
			{"https://bin.pkgforge.dev/" + arch + "/METADATA.WEB.json",
				"https://pkg.pkgforge.dev/" + arch + "/METADATA.WEB.json",
				true},
		}

		for _, repo := range repos {
			metadata, err := downloadWithFallback(repo)
			if err != nil {
				fmt.Printf("Error downloading JSON from both main and fallback URLs for repo: %v\n", err)
				continue
			}

			// Process bin, pkg, and base sections with path corrections
			metadata.Bin = processItems(metadata.Bin, realArchs, validatedArchs, repo, "bin")
			metadata.Pkg = processItems(metadata.Pkg, realArchs, validatedArchs, repo, "pkg")
			metadata.Base = processItems(metadata.Base, realArchs, validatedArchs, repo, "base")

			// Convert Metadata to OutputMetadata
			outputMetadata := convertMetadata(metadata)

			// Save the processed metadata to a JSON file
			outputFile := fmt.Sprintf("METADATA_AIO_%s.json", arch)
			if err := saveJSON(outputFile, outputMetadata); err != nil {
				fmt.Printf("Error saving JSON to %s: %v\n", outputFile, err)
				continue
			}

			fmt.Printf("Processed and saved to %s\n", outputFile)
		}
	}
}
