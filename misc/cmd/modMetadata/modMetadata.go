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
	Snapshots   []string `json:"snapshots,omitempty"`
	Provides    []string `json:"provides,omitempty"`
	Note        []string `json:"note,omitempty"`
	GhcrPkg     string   `json:"ghcr_pkg,omitempty"`
	GhcrBlob    string   `json:"ghcr_blob,omitempty"`
	HfPkg       string   `json:"hf_pkg,omitempty"`
	Rank        string   `json:"rank,omitempty"`
}

type DbinItem struct {
	Pkg             string   `json:"pkg"                        `
	Name            string   `json:"pkg_name"                   `
	BinId           string   `json:"pkg_id,omitempty"           `
	Icon            string   `json:"icon,omitempty"             `
	License         string   `json:"license,omitempty"          `
	Description     string   `json:"description,omitempty"      `
	LongDescription string   `json:"description_long,omitempty" `
	Screenshots     []string `json:"screenshots,omitempty"      `
	Version         string   `json:"version,omitempty"          `
	DownloadURL     string   `json:"download_url,omitempty"     `
	Size            string   `json:"size,omitempty"             `
	Bsum            string   `json:"bsum,omitempty"             `
	Shasum          string   `json:"shasum,omitempty"           `
	BuildDate       string   `json:"build_date,omitempty"       `
	SrcURLs         []string `json:"src_urls,omitempty"         `
	WebURLs         []string `json:"web_urls,omitempty"         `
	BuildScript     string   `json:"build_script,omitempty"     `
	BuildLog        string   `json:"build_log,omitempty"        `
	Categories      string   `json:"categories,omitempty"       `
	Snapshots   	[]string `json:"snapshots,omitempty"        `
	Provides        string   `json:"provides,omitempty"         `
	Notes           []string `json:"notes,omitempty"            `
	Appstream       string   `json:"appstream,omitempty"        `
	GhcrPkg         string   `json:"ghcr_pkg,omitempty"         `
	GhcrBlob        string   `json:"ghcr_blob,omitempty"        `
	Rank            uint     `json:"rank,omitempty"             `
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
		Snapshots:    item.Snapshots,
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

// So that the metadata is ordered alphabetically, but with priority exceptions, for example:
// I may want to lower the priority of all #NixPkg pkgs, and prioritize the binaries with an id thta starts with #ppkg
// Or I may want to push Glibc to the end of the list
func reorderItems(str []map[string]string, metadata DbinMetadata) {
	for _, replacements := range str {
		for repo, items := range metadata {
			// Replace str with str2
			for oldStr, newStr := range replacements {
				for i := range items {
					items[i].BinId = strings.ReplaceAll(items[i].BinId, oldStr, newStr)
				}
			}

			// Sort items alphabetically by BinId
			sort.Slice(items, func(i, j int) bool {
				return items[i].BinId < items[j].BinId
			})

			// Replace str2 back to str
			for oldStr, newStr := range replacements {
				for i := range items {
					items[i].BinId = strings.ReplaceAll(items[i].BinId, newStr, oldStr)
				}
			}

			metadata[repo] = items
		}
	}
}

func saveAll(filename string, metadata DbinMetadata) error {
	if err := saveJSON(filename, metadata); err != nil {
		return err
	}
	if err := saveCBOR(filename, metadata); err != nil {
		return err
	}
	//genAMMeta(filename, dbinMetadata)
	return saveYAML(filename, metadata)
}

// Save metadata in various formats
func saveMetadata(filename string, metadata DbinMetadata) error {
	// Reorder items alphabetically but with priority exceptions
	reorderItems([]map[string]string{
		{"musl": "0AAAMusl"},   // Higher priority for Musl
		{"ppkg": "0AAAPpkg"},   // Higher priority for ppkg
		{"glibc": "ZZZXGlibc"}, // Push glibc to the end
	}, metadata)

	if err := saveAll(filename, metadata); err != nil {
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
	return saveAll(filename, metadata)
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
			}
		}

		outputFile := fmt.Sprintf("METADATA_%s", outputArch)
		if err := saveMetadata(outputFile, dbinMetadata); err != nil {
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

/* The following is a _favor_ I'm doing to ivan-hc and everyone that contributes
 *  And actively endorses or uses AM
 *  They are a tremendous help to the Portable Linux Apps community!
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
 */
