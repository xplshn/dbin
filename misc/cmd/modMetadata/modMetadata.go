package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/goccy/go-json"
	minify "github.com/tdewolff/minify/v2"
	mjson "github.com/tdewolff/minify/v2/json"
)

type PkgForgeItem struct {
	RealName    string   `json:"pkg"`
	Name        string   `json:"pkg_name"`
	Family      string   `json:"pkg_family"`
	BinId       string   `json:"pkg_id"`
	Icon        string   `json:"icon,omitempty"`
	Description string   `json:"description,omitempty"`
	Homepage    []string `json:"homepage,omitempty"`
	Version     string   `json:"version,omitempty"`
	DownloadURL string   `json:"download_url,omitempty"`
	Size        string   `json:"size,omitempty"`
	Bsum        string   `json:"bsum,omitempty"`
	Shasum      string   `json:"shasum,omitempty"`
	BuildDate   string   `json:"build_date,omitempty"`
	SrcURL      []string `json:"src_url,omitempty"`
	BuildScript string   `json:"build_script,omitempty"`
	BuildLog    string   `json:"build_log,omitempty"`
	Category    []string `json:"category,omitempty"`
	ExtraBins   []string `json:"provides,omitempty"`
	Note        []string `json:"note,omitempty"`
}

type DbinItem struct {
	RealName        string   `json:"pkg"`
	Name            string   `json:"pkg_name"`
	BinId           string   `json:"pkg_id,omitempty"`
	Icon            string   `json:"icon,omitempty"`
	Description     string   `json:"description,omitempty"`
	LongDescription string   `json:"description_long,omitempty"`
	Screenshots     []string `json:"screenshots,omitempty"`
	Version         string   `json:"version,omitempty"`
	DownloadURL     string   `json:"download_url,omitempty"`
	Size            string   `json:"size,omitempty"`
	Bsum            string   `json:"bsum,omitempty"`
	Shasum          string   `json:"shasum,omitempty"`
	BuildDate       string   `json:"build_date,omitempty"`
	SrcURL          string   `json:"src_url,omitempty"`
	WebURL          string   `json:"homepage,omitempty"`
	BuildScript     string   `json:"build_script,omitempty"`
	BuildLog        string   `json:"build_log,omitempty"`
	Category        string   `json:"category,omitempty"`
	ExtraBins       string   `json:"provides,omitempty"`
	Note            string   `json:"note,omitempty"`
	Appstream       string   `json:"appstream,omitempty"`
}

type DbinMetadata struct {
	Bin  []DbinItem `json:"bin"`
	Pkg  []DbinItem `json:"pkg"`
	Base []DbinItem `json:"base"`
}

func convertPkgForgeToDbinItem(item PkgForgeItem) DbinItem {
	var webURL, srcURL, category, note string

	if len(item.Homepage) > 0 {
		webURL = item.Homepage[0]
	}

	if len(item.SrcURL) > 0 {
		srcURL = item.SrcURL[0]
	}

	if len(item.Category) > 0 {
		category = item.Category[0]
	}

	if len(item.Note) > 0 {
		note = item.Note[0]
	}

	// Convert provides array to comma-separated string
	var provides string
	if len(item.ExtraBins) > 0 {
		provides = strings.Join(item.ExtraBins, ",")
	}

	return DbinItem{
		RealName:    t(item.Family == item.Name, item.Name, fmt.Sprintf("%s/%s", item.Family, item.Name)), // If item.Name != item.Family, use item.Family/item.Name
		Name:        item.Name,
		BinId:       item.BinId,
		Icon:        item.Icon,
		Description: item.Description,
		Version:     item.Version,
		DownloadURL: item.DownloadURL,
		Size:        item.Size,
		Bsum:        item.Bsum,
		Shasum:      item.Shasum,
		BuildDate:   item.BuildDate,
		SrcURL:      srcURL,
		WebURL:      webURL,
		BuildScript: item.BuildScript,
		BuildLog:    item.BuildLog,
		Category:    category,
		ExtraBins:   provides,
		Note:        note,
	}
}

func downloadJSON(url string) ([]PkgForgeItem, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var items []PkgForgeItem
	err = json.Unmarshal(body, &items)
	if err != nil {
		return nil, err
	}

	return items, nil
}

func saveJSON(filename string, metadata DbinMetadata) error {
	jsonData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(filename, jsonData, 0644)
	if err != nil {
		return err
	}

	return minifyJSON(filename, jsonData)
}

func minifyJSON(filename string, jsonData []byte) error {
	m := minify.New()
	m.AddFunc("application/json", mjson.Minify)

	minifiedData, err := m.Bytes("application/json", jsonData)
	if err != nil {
		return err
	}

	minFilename := strings.TrimSuffix(filename, ".json") + ".min.json"
	return os.WriteFile(minFilename, minifiedData, 0644)
}

func main() {
	realArchs := []string{"x86_64-Linux", "aarch64-Linux"}

	for _, arch := range realArchs {
		pkgforgeURL := fmt.Sprintf("https://meta.pkgforge.dev/bincache/%s.json", arch)

		// Download and parse pkgforge metadata
		pkgforgeItems, err := downloadJSON(pkgforgeURL)
		if err != nil {
			fmt.Printf("Error downloading pkgforge metadata from %s: %v\n", pkgforgeURL, err)
			continue
		}

		// Download AppBundleHUB metadata
		appbundleURL := "https://github.com/xplshn/AppBundleHUB/releases/download/latest_metadata/metadata.json"
		var appbundleMetadata DbinMetadata
		resp, err := http.Get(appbundleURL)
		if err == nil {
			defer resp.Body.Close()
			if body, err := io.ReadAll(resp.Body); err == nil {
				json.Unmarshal(body, &appbundleMetadata)
			}
		}

		// Convert pkgforge items to dbin format
		var dbinMetadata DbinMetadata
		bsumMap := make(map[string]DbinItem)

		for _, item := range pkgforgeItems {
			dbinItem := convertPkgForgeToDbinItem(item)

			// Check if the b3sum already exists in the map
			if existingItem, exists := bsumMap[dbinItem.Bsum]; exists {
				// Keep the item with the shortest name
				if len(dbinItem.Name) < len(existingItem.Name) {
					bsumMap[dbinItem.Bsum] = dbinItem
				}
			} else {
				bsumMap[dbinItem.Bsum] = dbinItem
			}
		}

		// Add items to the appropriate section based on pkg_type
		for _, item := range bsumMap {
			switch {
			case strings.HasSuffix(item.RealName, ".static"):
				dbinMetadata.Bin = append(dbinMetadata.Bin, item)
			case strings.HasSuffix(item.RealName, ".dynamic"):
				dbinMetadata.Pkg = append(dbinMetadata.Pkg, item)
			default:
				dbinMetadata.Base = append(dbinMetadata.Base, item)
			}
		}

		// Merge with AppBundleHUB metadata if available
		if len(appbundleMetadata.Pkg) > 0 {
			dbinMetadata.Pkg = append(dbinMetadata.Pkg, appbundleMetadata.Pkg...)
		}

		// Save the processed metadata
		outputFile := fmt.Sprintf("METADATA_AIO_%s.json", arch)
		if err := saveJSON(outputFile, dbinMetadata); err != nil {
			fmt.Printf("Error saving metadata to %s: %v\n", outputFile, err)
			continue
		}

		fmt.Printf("Successfully processed and saved metadata to %s\n", outputFile)
	}
}

// Helper function
func t[T any](cond bool, vtrue, vfalse T) T {
	if cond {
		return vtrue
	}
	return vfalse
}
