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
	"github.com/shamaton/msgpack/v2"
	minify "github.com/tdewolff/minify/v2"
	mjson "github.com/tdewolff/minify/v2/json"
	"github.com/tiendc/go-deepcopy"
)

type repository struct {
	URL        string
	Name       string
	Standalone bool
	Single     bool
}

type PkgForgeItem struct {
	Pkg         string   `json:"pkg"`
	Name        string   `json:"pkg_name,omitempty"`
	Family      string   `json:"pkg_family,omitempty"`
	PkgId       string   `json:"pkg_id,omitempty"`
	AppId       string   `json:"app_id,omitempty"`
	PkgType     string   `json:"pkg_type,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Screenshots []string `json:"screenshots,omitempty"`
	Description string   `json:"description,omitempty"`
	WebURLs     []string `json:"homepage,omitempty"`
	Version     string   `json:"version,omitempty"`
	DownloadURL string   `json:"download_url,omitempty"`
	Size        string   `json:"size,omitempty"`
	Bsum        string   `json:"bsum,omitempty"`
	Shasum      string   `json:"shasum,omitempty"`
	BuildDate   string   `json:"build_date,omitempty"`
	SrcURLs     []string `json:"src_url,omitempty"`
	BuildScript string   `json:"build_script,omitempty"`
	BuildLog    string   `json:"build_log,omitempty"`
	Category    []string `json:"categories,omitempty"`
	Snapshots   []string `json:"snapshots,omitempty"`
	Provides    []string `json:"provides,omitempty"`
	Notes       []string `json:"note,omitempty"`
	License     []string `json:"license,omitempty"`
	GhcrPkg     string   `json:"ghcr_pkg,omitempty"`
	HfPkg       string   `json:"hf_pkg,omitempty"`
	Rank        string   `json:"rank,omitempty"`
}

type snapshot struct {
	Commit  string `json:"commit,omitempty"`
	Version string `json:"version,omitempty"`
}

type DbinItem struct {
	Pkg             string     `json:"pkg,omitempty"`
	Name            string     `json:"pkg_name,omitempty"`
	PkgId           string     `json:"pkg_id,omitempty"`
	AppstreamId     string     `json:"app_id,omitempty"`
	Icon            string     `json:"icon,omitempty"`
	Description     string     `json:"description,omitempty"`
	LongDescription string     `json:"description_long,omitempty"`
	Screenshots     []string   `json:"screenshots,omitempty"`
	Version         string     `json:"version,omitempty"`
	DownloadURL     string     `json:"download_url,omitempty"`
	Size            string     `json:"size,omitempty"`
	Bsum            string     `json:"bsum,omitempty"`
	Shasum          string     `json:"shasum,omitempty"`
	BuildDate       string     `json:"build_date,omitempty"`
	SrcURLs         []string   `json:"src_urls,omitempty"`
	WebURLs         []string   `json:"web_urls,omitempty"`
	BuildScript     string     `json:"build_script,omitempty"`
	BuildLog        string     `json:"build_log,omitempty"`
	Categories      string     `json:"categories,omitempty"`
	Snapshots       []snapshot `json:"snapshots,omitempty"`
	Provides        string     `json:"provides,omitempty"`
	License         []string   `json:"license,omitempty"`
	Notes           []string   `json:"notes,omitempty"`
	Appstream       string     `json:"appstream,omitempty"`
	Rank            uint       `json:"rank,omitempty"`
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
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var metadata DbinMetadata
	err = json.Unmarshal(body, &metadata)
	if err != nil {
		return nil, err
	}

	// Since the metadata is already in Dbin format, we just need to extract the items
	for _, items := range metadata {
		return items, nil
	}

	return nil, nil
}

type AppStreamMetadata struct {
	AppId           string   `json:"app_id"`
	Categories      string   `json:"categories"`
	RichDescription string   `json:"rich_description"`
	Icons           []string `json:"icons"`
	Screenshots     []string `json:"screenshots"`
}

var appStreamMetadata []AppStreamMetadata
var appStreamMetadataLoaded bool

func loadAppStreamMetadata() error {
	if appStreamMetadataLoaded {
		return nil
	}

	resp, err := http.Get("https://github.com/xplshn/dbin-metadata/raw/refs/heads/master/misc/cmd/flatpakAppStreamScrapper/appstream_metadata.cbor")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = cbor.Unmarshal(body, &appStreamMetadata)
	if err != nil {
		return err
	}

	appStreamMetadataLoaded = true
	return nil
}

func updateItemWithAppStreamMetadata(item *DbinItem) {
	if item.AppstreamId == "" {
		return
	}

	for _, metadata := range appStreamMetadata {
		if metadata.AppId == item.AppstreamId {
			if len(metadata.Icons) > 0 {
				item.Icon = metadata.Icons[0]
			}
			if len(metadata.Screenshots) > 0 {
				item.Screenshots = metadata.Screenshots
			}
			if metadata.Categories != "" {
				item.Categories = metadata.Categories
			}
			if metadata.RichDescription != "" {
				item.LongDescription = metadata.RichDescription
			}
			break
		}
	}
}

func fetchAndConvertMetadata(url string, downloadFunc func(string) ([]PkgForgeItem, error), convertFunc func(PkgForgeItem, map[string]bool) (DbinItem, bool)) ([]DbinItem, error) {
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
		dbinItem, include := convertFunc(item, useFamilyFormat)
		if include {
			updateItemWithAppStreamMetadata(&dbinItem)
			dbinItems = append(dbinItems, dbinItem)
		}
	}

	return dbinItems, nil
}

func convertPkgForgeToDbinItem(item PkgForgeItem, useFamilyFormat map[string]bool) (DbinItem, bool) {
	// PkgTypes we discard, completely
	if item.PkgType == "dynamic" {
		return DbinItem{}, false
	}

	var categories, provides, downloadURL string

	if len(item.Category) > 0 {
		categories = strings.Join(item.Category, ",")
	}

	if len(item.Provides) > 0 {
		provides = strings.Join(item.Provides, ",")
	}

	if item.GhcrPkg != "" {
		downloadURL = "oci://" + item.GhcrPkg
	} else if item.HfPkg != "" {
		downloadURL = strings.Replace(item.HfPkg, "/tree/main", "/resolve/main", 1) + "/" + item.Pkg
	} else if item.DownloadURL != "" {
		downloadURL = item.DownloadURL
	}

	rank, _ := strconv.Atoi(item.Rank)

	// Parse snapshots
	var snapshots []snapshot
	for _, snapshotStr := range item.Snapshots {
		parts := strings.Split(snapshotStr, "[")
		commit := strings.TrimSpace(parts[0])
		version := ""
		if len(parts) > 1 {
			version = strings.TrimSuffix(parts[1], "]")
		}
		snapshots = append(snapshots, snapshot{Commit: commit, Version: version})
	}

	// Would love to do this: but Snapshots doesn't update as often as it should for this to be feasible
	//if strings.HasPrefix(item.Version, "HEAD-") && len(snapshots) >= 1 {
	//	if snapshots[0].Version != "" {
	//		item.Version = snapshots[0].Version
	//	}
	//}

	// - Determine the package name format
	//   | - If all packages in a family have the same name (e.g., "bwrap" in the "bubblewrap" family),
	//   |   the package name will be just the package name (e.g., "bwrap").
	//   | - If there are multiple packages with different names in a family, the format will be
	//   |   "family/package_name" (e.g., "a-utils/ccat").
	// - Applies to all occurrences
	pkgName := item.Name
	if item.Family != "" && useFamilyFormat[item.Family] {
		pkgName = fmt.Sprintf("%s/%s", item.Family, item.Name)
	}

	if item.PkgType == "static" {
		pkgName = strings.TrimSuffix(pkgName, ".static")
	} else if item.PkgType == "archive" {
		pkgName = strings.TrimSuffix(pkgName, ".archive")
	} else if item.PkgType != "" {
		pkgName = pkgName + "." + item.PkgType
	}

	item.Pkg = strings.TrimPrefix(item.Pkg, "/")

	if areSlicesEqual(item.SrcURLs, item.WebURLs) {
		item.WebURLs = []string{}
	}

	return DbinItem{
		Pkg:         pkgName,
		Name:        item.Name,
		PkgId:       item.PkgId,
		AppstreamId: item.AppId,
		Icon:        item.Icon,
		Screenshots: item.Screenshots,
		Description: item.Description,
		Version:     item.Version,
		DownloadURL: downloadURL,
		Size:        item.Size,
		Bsum:        item.Bsum,
		Shasum:      item.Shasum,
		BuildDate:   item.BuildDate,
		SrcURLs:     item.SrcURLs,
		WebURLs:     item.WebURLs,
		BuildScript: item.BuildScript,
		BuildLog:    item.BuildLog,
		Categories:  categories,
		Snapshots:   snapshots,
		Provides:    provides,
		License:     item.License,
		Notes:       item.Notes,
		Rank:        uint(rank),
	}, true
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

func reorderItems(str []map[string]string, metadata DbinMetadata) {
	for _, replacements := range str {
		for repo, items := range metadata {
			// Replace str with str2
			for oldStr, newStr := range replacements {
				for i := range items {
					items[i].PkgId = strings.ReplaceAll(items[i].PkgId, oldStr, newStr)
				}
			}

			// Sort items alphabetically by BinId
			sort.Slice(items, func(i, j int) bool {
				return items[i].PkgId < items[j].PkgId
			})

			// Replace str2 back to str
			for oldStr, newStr := range replacements {
				for i := range items {
					items[i].PkgId = strings.ReplaceAll(items[i].PkgId, newStr, oldStr)
				}
			}

			metadata[repo] = items
		}
	}
}

func saveAll(filename string, metadata DbinMetadata) error {
	if err := saveCBOR(filename, metadata); err != nil {
		return err
	}
	if err := saveJSON(filename, metadata); err != nil {
		return err
	}
	if err := saveMsgp(filename, metadata); err != nil {
		return err
	}
	//genAMMeta(filename, dbinMetadata)
	return saveYAML(filename, metadata)
}

func saveMetadata(filename string, metadata DbinMetadata) error {
	// Reorder items alphabetically but with priority exceptions, to ensure a higher level of quality.
	// We basically do a search&replace, order alphabetically, and then do a search&replace again.
	// I prioritize binaries with a smaller size, more hardware compat, and that are truly static.
	reorderItems([]map[string]string{
		{"musl": "0000_"},    // | Higher priority for Musl
		{"ppkg": "0020__"},   // | Higher priority for ppkg
		{"glibc": "0040___"}, // | Push glibc to the end
		// -					 // | - Little Glenda says hi!
		// -      				 // |   (\(\
		{"musl-v3": "0080_"},    // |   ¸". ..
		{"glibc-v3": "0100___"}, // |   (  . .)
		// -    				 // |   |   ° ¡
		{"musl-v4": "0200_"},    // |   ¿     ;
		{"glibc-v4": "0400___"}, // |  c?".UJ"
	}, metadata)

	if err := saveAll(filename, metadata); err != nil {
		return err
	}

	// "web" version
	var webMetadata DbinMetadata
	_ = deepcopy.Copy(&webMetadata, &metadata)
	for _, items := range webMetadata {
		for i := range items {
			items[i].Provides = ""
			items[i].Shasum = ""
			items[i].Bsum = ""
		}
	}
	saveAll(filename+".web", webMetadata)
	// "lite" version
	for _, items := range metadata {
		for i := range items {
			items[i].Icon = ""
			items[i].Provides = ""
			items[i].Shasum = ""
			items[i].AppstreamId = ""
			items[i].LongDescription = ""
			items[i].Screenshots = []string{}
		}
	}
	return saveAll(filename+".lite", metadata)
}

func saveCBOR(filename string, metadata DbinMetadata) error {
	cborData, err := cbor.Marshal(metadata)
	if err != nil {
		return err
	}
	return os.WriteFile(filename+".cbor", cborData, 0644)
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
func saveMsgp(filename string, metadata DbinMetadata) error {
	msgpData, err := msgpack.Marshal(metadata)
	if err != nil {
		return err
	}
	return os.WriteFile(filename+".msgp", msgpData, 0644)
}
func saveYAML(filename string, metadata DbinMetadata) error {
	yamlData, err := yaml.Marshal(metadata)
	if err != nil {
		return err
	}
	return os.WriteFile(filename+".yaml", yamlData, 0644)
}

func main() {
	// Load AppStream metadata once at startup
	if err := loadAppStreamMetadata(); err != nil {
		fmt.Printf("Error loading AppStream metadata: %v\n", err)
	}

	realArchs := map[string]string{
		"x86_64-Linux":  "amd64_linux",
		"aarch64-Linux": "arm64_linux",
		"riscv64-Linux": "riscv64_linux",
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
				Name:       "AM",
				URL:        "https://meta.pkgforge.dev/external/am/%s.json",
				Standalone: true,
			},
			Handler: PkgForgeHandler{},
		},
		{
			Repo: repository{
				Name:       "appimage-github-io",
				URL:        "https://meta.pkgforge.dev/external/appimage.github.io/%s.json.",
				Standalone: true,
			},
			Handler: PkgForgeHandler{},
		},
		{
			Repo: repository{
				Name:   "AppBundleHUB",
				URL:    "https://github.com/xplshn/AppBundleHUB/releases/download/latest_metadata/metadata_%s.json",
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

			// Filter items from "pkgcache" repository that do not contain "[PORTABLE]" in their Notes
			if repo.Repo.Name == "pkgcache" {
				var filteredItems []DbinItem
				for _, item := range items {
					hasPortableNote := false
					for _, note := range item.Notes {
						if strings.Contains(note, "[PORTABLE]") {
							hasPortableNote = true
							break
						}
					}
					if hasPortableNote {
						filteredItems = append(filteredItems, item)
					}
				}
				items = filteredItems
			}

			if !repo.Repo.Standalone {
				dbinMetadata[repo.Repo.Name] = append(dbinMetadata[repo.Repo.Name], items...)
			}

			if repo.Repo.Single || repo.Repo.Standalone {
				singleMetadata := make(DbinMetadata)
				singleMetadata[repo.Repo.Name] = items
				singleOutputFile := fmt.Sprintf("%s_%s", repo.Repo.Name, outputArch)

				if err := saveMetadata(singleOutputFile, singleMetadata); err != nil {
					fmt.Printf("Error saving single metadata to %s: %v\n", singleOutputFile, err)
					continue
				}
				fmt.Printf("Successfully saved single metadata to %s\n", singleOutputFile)
			}
		}

		outputFile := fmt.Sprintf("%s", outputArch)
		if err := saveMetadata(outputFile, dbinMetadata); err != nil {
			fmt.Printf("Error saving metadata to %s: %v\n", outputFile, err)
			continue
		}

		fmt.Printf("Successfully processed and saved combined metadata to %s\n", outputFile)
	}
}

func areSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

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
