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

type repository struct {
	URL    string
	Name   string
	Single bool
}

type PkgForgeItem struct {
	Pkg         string   `json:"pkg"`
	Name        string   `json:"pkg_name,omitempty"`
	Family      string   `json:"pkg_family,omitempty"`
	BinId       string   `json:"pkg_id,omitempty"`
	PkgType     string   `json:"pkg_type,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Screenshots []string `json:"screenshots,omitempty"`
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
	Categories  []string `json:"categories,omitempty"`
	ExtraBins   []string `json:"provides,omitempty"`
	Note        []string `json:"note,omitempty"`
	GhcrBlob    string   `json:"ghcr_blob,omitempty"`
	Rank        string   `json:"rank,omitempty"`
}

type DbinItem struct {
	Pkg             string   `json:"pkg"`
	Name            string   `json:"pkg_name"`
	BinId           string   `json:"pkg_id,omitempty"`
	Icon            string   `json:"icon,omitempty"`
	License         string   `json:"license,omitempty"`
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
	Categories      string   `json:"categories,omitempty"`
	ExtraBins       string   `json:"provides,omitempty"`
	Note            string   `json:"note,omitempty"`
	Appstream       string   `json:"appstream,omitempty"`
	GhcrURL         string   `json:"ghcr_url,omitempty"`
	Rank            string   `json:"rank,omitempty"`
}

type DbinMetadata map[string][]DbinItem

type RepositoryHandler interface {
	FetchMetadata(url string) ([]DbinItem, error)
}

type PkgForgeHandler struct{}

func (PkgForgeHandler) FetchMetadata(url string) ([]DbinItem, error) {
	pkgforgeItems, err := downloadJSON(url)
	if err != nil {
		return nil, err
	}

	bsumMap := make(map[string]DbinItem)
	for _, item := range pkgforgeItems {
		dbinItem := convertPkgForgeToDbinItem(item)
		if existingItem, exists := bsumMap[dbinItem.Bsum]; exists {
			if len(dbinItem.Pkg) < len(existingItem.Pkg) {
				bsumMap[dbinItem.Bsum] = dbinItem
			}
		} else {
			bsumMap[dbinItem.Bsum] = dbinItem
		}
	}

	var dbinItems []DbinItem
	for _, item := range bsumMap {
		dbinItems = append(dbinItems, item)
	}

	return dbinItems, nil
}

type DbinHandler struct{}

func (DbinHandler) FetchMetadata(url string) ([]DbinItem, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var oldAppbundleMetadata OldDbinMetadata
	err = json.Unmarshal(body, &oldAppbundleMetadata)
	if err != nil {
		return nil, err
	}

	return oldAppbundleMetadata.Pkg, nil
}

// TODO: Make this dynamic. Appbundlehub meta now uses "appbundlehub" as key instead of Pkg
type OldDbinMetadata struct {
	Bin  []DbinItem `json:"bin"`
	Pkg  []DbinItem `json:"pkg"`
	Base []DbinItem `json:"base"`
}

func convertPkgForgeToDbinItem(item PkgForgeItem) DbinItem {
	var webURL, srcURL, categories, note string

	if len(item.Homepage) > 0 {
		webURL = item.Homepage[0]
	}

	if len(item.SrcURL) > 0 {
		srcURL = item.SrcURL[0]
	}

	if len(item.Categories) > 0 {
		categories = strings.Join(item.Categories, ",")
	}

	if len(item.Note) > 0 {
		note = item.Note[0]
	}

	var provides string
	if len(item.ExtraBins) > 0 {
		provides = strings.Join(item.ExtraBins, ",")
	}

	return DbinItem{
		//Pkg:         strings.TrimSuffix(t(item.Family == item.Name, item.Name, fmt.Sprintf("%s/%s", item.Family, item.Pkg)), ".static"),
		Pkg:         fmt.Sprintf("%s%s", t(item.Family == item.Name, item.Name, fmt.Sprintf("%s/%s", item.Family, item.Name)), t(item.PkgType != "static", "."+item.PkgType, "")),
		Name:        item.Name,
		BinId:       item.BinId,
		Icon:        item.Icon,
		Screenshots: item.Screenshots,
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
		Categories:  categories,
		ExtraBins:   provides,
		Note:        note,
		GhcrURL:     item.GhcrBlob,
		Rank:        item.Rank,
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
	realArchs := map[string]string{
		"x86_64-Linux":  "amd64_linux",
		"aarch64-Linux": "arm64_linux",
	}

	repositories := []struct {
		Repo    repository
		Handler RepositoryHandler
	}{
		{
			Repo: repository{
				Name:   "bincache",
				URL:    "https://meta.pkgforge.dev/bincache/%s.json",
				Single: true,
			},
			Handler: PkgForgeHandler{},
		},
		{
			Repo: repository{
				Name:   "pkgcache",
				URL:    "https://meta.pkgforge.dev/pkgcache/%s.json",
				Single: true,
			},
			Handler: PkgForgeHandler{},
		},
		{
			Repo: repository{
				Name:   "appbundlehub",
				URL:    "https://github.com/xplshn/AppBundleHUB/releases/download/latest_metadata/metadata_x86_64-Linux.json",
				Single: true,
			},
			Handler: DbinHandler{},
		},
	}

	for arch, outputArch := range realArchs {
		dbinMetadata := make(DbinMetadata)

		for _, repo := range repositories {
			url := repo.Repo.URL
			if strings.Contains(url, "%s") {
				url = fmt.Sprintf(url, arch)
			}

			items, err := repo.Handler.FetchMetadata(url)
			if err != nil {
				fmt.Printf("Error downloading %s metadata from %s: %v\n", repo.Repo.Name, url, err)
				continue
			}

			dbinMetadata[repo.Repo.Name] = append(dbinMetadata[repo.Repo.Name], items...)

			// If Single is true, save individual metadata file for this repository
			if repo.Repo.Single {
				singleMetadata := make(DbinMetadata)
				singleMetadata[repo.Repo.Name] = items
				singleOutputFile := fmt.Sprintf("METADATA_%s_%s.json", repo.Repo.Name, outputArch)

				if err := saveJSON(singleOutputFile, singleMetadata); err != nil {
					fmt.Printf("Error saving single metadata to %s: %v\n", singleOutputFile, err)
					continue
				}
				fmt.Printf("Successfully saved single metadata to %s\n", singleOutputFile)
			}
		}

		// Save combined metadata file
		outputFile := fmt.Sprintf("METADATA_%s.json", outputArch)
		if err := saveJSON(outputFile, dbinMetadata); err != nil {
			fmt.Printf("Error saving metadata to %s: %v\n", outputFile, err)
			continue
		}

		fmt.Printf("Successfully processed and saved combined metadata to %s\n", outputFile)
	}
}

// Ternary
func t[T any](cond bool, vtrue, vfalse T) T {
	if cond {
		return vtrue
	}
	return vfalse
}
