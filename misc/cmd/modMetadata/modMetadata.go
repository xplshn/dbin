package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/goccy/go-json"
	"github.com/goccy/go-yaml"
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
	Category    []string `json:"categories,omitempty"`
	Provides    []string `json:"provides,omitempty"`
	Note        []string `json:"note,omitempty"`
	GhcrPkg     string   `json:"ghcr_pkg,omitempty"`
	GhcrBlob    string   `json:"ghcr_blob,omitempty"`
	HfPkg       string   `json:"hf_pkg,omitempty"`
	Rank        string   `json:"rank,omitempty"`
}

type DbinItem struct {
	Pkg             string   `json:"pkg"                        cbor:"pkg"                        yaml:"pkg"                       `
	Name            string   `json:"pkg_name"                   cbor:"pkg_name"                   yaml:"pkg_name"                  `
	BinId           string   `json:"pkg_id,omitempty"           cbor:"pkg_id,omitempty"           yaml:"pkg_id,omitempty"          `
	Icon            string   `json:"icon,omitempty"             cbor:"icon,omitempty"             yaml:"icon,omitempty"            `
	License         string   `json:"license,omitempty"          cbor:"license,omitempty"          yaml:"license,omitempty"         `
	Description     string   `json:"description,omitempty"      cbor:"description,omitempty"      yaml:"description,omitempty"     `
	LongDescription string   `json:"description_long,omitempty" cbor:"description_long,omitempty" yaml:"description_long,omitempty"`
	Screenshots     []string `json:"screenshots,omitempty"      cbor:"screenshots,omitempty"      yaml:"screenshots,omitempty"     `
	Version         string   `json:"version,omitempty"          cbor:"version,omitempty"          yaml:"version,omitempty"         `
	DownloadURL     string   `json:"download_url,omitempty"     cbor:"download_url,omitempty"     yaml:"download_url,omitempty"    `
	Size            string   `json:"size,omitempty"             cbor:"size,omitempty"             yaml:"size,omitempty"            `
	Bsum            string   `json:"bsum,omitempty"             cbor:"bsum,omitempty"             yaml:"bsum,omitempty"            `
	Shasum          string   `json:"shasum,omitempty"           cbor:"shasum,omitempty"           yaml:"shasum,omitempty"          `
	BuildDate       string   `json:"build_date,omitempty"       cbor:"build_date,omitempty"       yaml:"build_date,omitempty"      `
	SrcURLs         []string `json:"src_urls,omitempty"         cbor:"src_urls,omitempty"         yaml:"src_urls,omitempty"        `
	WebURLs         []string `json:"web_urls,omitempty"         cbor:"web_urls,omitempty"         yaml:"web_urls,omitempty"        `
	BuildScript     string   `json:"build_script,omitempty"     cbor:"build_script,omitempty"     yaml:"build_script,omitempty"    `
	BuildLog        string   `json:"build_log,omitempty"        cbor:"build_log,omitempty"        yaml:"build_log,omitempty"       `
	Categories      string   `json:"categories,omitempty"       cbor:"categories,omitempty"       yaml:"categories,omitempty"      `
	Provides        string   `json:"provides,omitempty"         cbor:"provides,omitempty"         yaml:"provides,omitempty"        `
	Notes           []string `json:"notes,omitempty"            cbor:"notes,omitempty"            yaml:"notes,omitempty"           `
	Appstream       string   `json:"appstream,omitempty"        cbor:"appstream,omitempty"        yaml:"appstream,omitempty"       `
	GhcrPkg         string   `json:"ghcr_pkg,omitempty"         cbor:"ghcr_pkg,omitempty"         yaml:"ghcr_pkg,omitempty"        `
	GhcrBlob        string   `json:"ghcr_blob,omitempty"        cbor:"ghcr_blob,omitempty"        yaml:"ghcr_blob,omitempty"       `
	Rank            uint     `json:"rank,omitempty"             cbor:"rank,omitempty"             yaml:"rank,omitempty"            `
}

type DbinMetadata map[string][]DbinItem

type RepositoryHandler interface {
	FetchMetadata(url string) ([]DbinItem, error)
}

type PkgForgeHandler struct{}

func (PkgForgeHandler) FetchMetadata(url string) ([]DbinItem, error) {
	return fetchAndConvertMetadata(url, downloadJSON, convertPkgForgeToDbinItem)
}

type DbinHandler struct{}

func (DbinHandler) FetchMetadata(url string) ([]DbinItem, error) {
	return fetchAndConvertMetadata(url, downloadJSON, convertPkgForgeToDbinItem)
}

/*
func fetchAndConvertMetadata(url string, downloadFunc func(string) ([]PkgForgeItem, error), convertFunc func(PkgForgeItem) DbinItem) ([]DbinItem, error) {
	items, err := downloadFunc(url)
	if err != nil {
		return nil, err
	}

	bsumMap := make(map[string]DbinItem)
	for _, item := range items {
		dbinItem := convertFunc(item)
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
		if item.Name != "" {
			dbinItems = append(dbinItems, item)
		}
	}

	return dbinItems, nil
}*/

func fetchAndConvertMetadata(url string, downloadFunc func(string) ([]PkgForgeItem, error), convertFunc func(PkgForgeItem, map[string]bool) DbinItem) ([]DbinItem, error) {
	items, err := downloadFunc(url)
	if err != nil {
		return nil, err
	}

	familyCount := make(map[string]int)
	familyNames := make(map[string]string)
	useFamilyFormat := make(map[string]bool)

	for _, item := range items {
		familyCount[item.Family]++
		if familyNames[item.Family] == "" {
			familyNames[item.Family] = item.Name
		} else if familyNames[item.Family] != item.Name {
			useFamilyFormat[item.Family] = true
		}
	}

	var dbinItems []DbinItem
	for _, item := range items {
		dbinItem := convertFunc(item, useFamilyFormat)
		dbinItems = append(dbinItems, dbinItem)
	}

	return dbinItems, nil
}

func convertPkgForgeToDbinItem(item PkgForgeItem, useFamilyFormat map[string]bool) DbinItem {
	var categories, provides, downloadURL string

	if len(item.Category) > 0 {
		categories = strings.Join(item.Category, ",")
	}

	if len(item.Provides) > 0 {
		provides = strings.Join(item.Provides, ",")
	}

	if item.HfPkg != "" {
		downloadURL = strings.Replace(item.HfPkg, "/tree/main", "/resolve/main", 1) + "/" + item.Pkg
	}

	rank, _ := strconv.Atoi(item.Rank)

	// PkgTypes we discard, completely
	if item.PkgType == "archive" {
		return DbinItem{}
	}

	// - Determine the package name format
	//   | - If all packages in a family have the same name (e.g., "bwrap" in the "bubblewrap" family),
	//   |   the package name will be just the package name (e.g., "bwrap").
	//   | - If there are multiple packages with different names in a family, the format will be
	//   |   "family/package_name" (e.g., "a-utils/ccat").
	// - Applies to all occurrences
	pkgName := item.Name
	if useFamilyFormat[item.Family] {
		pkgName = fmt.Sprintf("%s/%s", item.Family, item.Name)
	}
	if item.PkgType == "static" {
		pkgName = strings.TrimSuffix(pkgName, ".static")
	} else if item.PkgType != "" {
		pkgName = pkgName + "." + item.PkgType
	}

	return DbinItem{
		Pkg:         pkgName,
		Name:        item.Name,
		BinId:       item.BinId,
		Icon:        item.Icon,
		Screenshots: item.Screenshots,
		Description: item.Description,
		Version:     item.Version,
		DownloadURL: downloadURL,
		Size:        item.Size,
		Bsum:        item.Bsum,
		Shasum:      item.Shasum,
		BuildDate:   item.BuildDate,
		SrcURLs:     item.SrcURL,
		WebURLs:     item.Homepage,
		BuildScript: item.BuildScript,
		BuildLog:    item.BuildLog,
		Categories:  categories,
		Provides:    provides,
		Notes:       item.Note,
		GhcrPkg:     "oci://" + item.GhcrPkg,
		GhcrBlob:    "oci://" + item.GhcrBlob,
		Rank:        uint(rank),
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

// Save metadata in both JSON and CBOR formats
func saveMetadata(filename string, metadata DbinMetadata) error {
	// Replace "musl" with "AAA111Musl"
	for repo, items := range metadata {
		for i := range items {
			items[i].BinId = strings.ReplaceAll(items[i].BinId, "musl", "AAA111Musl")
		}
		// Sort items alphabetically by BinId
		sort.Slice(items, func(i, j int) bool {
			return items[i].BinId < items[j].BinId
		})
		// Replace "AAA111Musl" back to "musl"
		for i := range items {
			items[i].BinId = strings.ReplaceAll(items[i].BinId, "AAA111Musl", "musl")
		}
		metadata[repo] = items
	}

	if err := saveJSON(filename, metadata); err != nil {
		return err
	}
	if err := saveCBOR(filename, metadata); err != nil {
		return err
	}
	if err := saveYAML(filename, metadata); err != nil {
		return err
	}

	// "lite" version
	for _, items := range metadata {
		for i := range items {
			items[i].Icon = ""
			items[i].Provides = ""
		}
	}
	filename += ".lite"

	if err := saveJSON(filename, metadata); err != nil {
		return err
	}
	if err := saveCBOR(filename, metadata); err != nil {
		return err
	}
	return saveYAML(filename, metadata)
}

func saveCBOR(filename string, metadata DbinMetadata) error {
	cborData, err := cbor.Marshal(metadata)
	if err != nil {
		return err
	}
	return os.WriteFile(filename+".cbor", cborData, 0644)
}
func saveYAML(filename string, metadata DbinMetadata) error {
	yamlData, err := yaml.Marshal(metadata)
	if err != nil {
		return err
	}
	return os.WriteFile(filename+".yaml", yamlData, 0644)
}
func saveJSON(filename string, metadata DbinMetadata) error {
	jsonData, err := json.MarshalIndent(metadata, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filename+".json", jsonData, 0644); err != nil {
		return err
	}
	// Minify JSON
	m := minify.New()
	m.AddFunc("application/json", mjson.Minify)
	if jsonData, err = m.Bytes("application/json", jsonData); err != nil {
		return err
	} else if err := os.WriteFile(filename+".min.json", jsonData, 0644); err != nil {
		return err
	}
	return nil
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

			if repo.Repo.Single {
				singleMetadata := make(DbinMetadata)
				singleMetadata[repo.Repo.Name] = items
				singleOutputFile := fmt.Sprintf("METADATA_%s_%s", repo.Repo.Name, outputArch)

				if err := saveMetadata(singleOutputFile, singleMetadata); err != nil {
					fmt.Printf("Error saving single metadata to %s: %v\n", singleOutputFile, err)
					continue
				}
				fmt.Printf("Successfully saved single metadata to %s\n", singleOutputFile)
				genAMMeta(fmt.Sprintf("AM_METADATA_%s_%s", repo.Repo.Name, outputArch), dbinMetadata)
			}
		}

		outputFile := fmt.Sprintf("METADATA_%s", outputArch)
		if err := saveMetadata(outputFile, dbinMetadata); err != nil {
			fmt.Printf("Error saving metadata to %s: %v\n", outputFile, err)
			continue
		}
		genAMMeta(fmt.Sprintf("AM_METADATA_%s", outputArch), dbinMetadata)

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

/* The following is a _favor_ I'm doing to ivan-hc and everyone that contributes
 *  And actively endorses or uses AM
 *  They are a tremendous help to the Portable Linux Apps community!
 */

const pipeRepl = "ǀ" // Replacement for `|` to avoid breaking the MD table
func replacePipeFields(pkg *DbinItem) {
	pkg.Name = strings.ReplaceAll(pkg.Name, "|", pipeRepl)
	pkg.Description = strings.ReplaceAll(pkg.Description, "|", pipeRepl)
	pkg.DownloadURL = strings.ReplaceAll(pkg.DownloadURL, "|", pipeRepl)
	for i := range pkg.WebURLs {
		pkg.WebURLs[i] = strings.ReplaceAll(pkg.WebURLs[i], "|", pipeRepl)
	}
}

func genAMMeta(filename string, metadata DbinMetadata) {
	replaceEmptyWithNil := func(value string) string {
		if value == "" {
			return "nil"
		}
		return value
	}

	file, err := os.Create(filename + ".txt")
	if err != nil {
		fmt.Println("Error creating output file:", err)
		return
	}
	defer file.Close()

	for _, items := range metadata {
		for _, pkg := range items {
			pkg.Name = replaceEmptyWithNil(pkg.Name)
			pkg.Description = replaceEmptyWithNil(pkg.Description)
			pkg.DownloadURL = replaceEmptyWithNil(pkg.DownloadURL)

			webURL := pkg.DownloadURL
			if webURL == "nil" && len(pkg.WebURLs) > 0 {
				webURL = pkg.WebURLs[0]
			}

			replacePipeFields(&pkg)

			pkgName := pkg.Name
			strings.ToLower(pkgName)
			strings.ReplaceAll(pkgName, " ", "-")

			bsum := pkg.Bsum
			if len(bsum) > 12 {
				bsum = bsum[:12]
			} else {
				bsum = "nil"
			}

			file.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
				pkgName, pkg.Description, webURL, pkg.DownloadURL, bsum))
		}
	}
}
