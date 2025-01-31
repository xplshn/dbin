package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const pipeRepl = "ǀ" // Replacement for `|` to avoid breaking the MD table

type Metadata struct {
	Bin  []Package `json:"bin"`
	Pkg  []Package `json:"pkg"`
	Base []Package `json:"base"`
}

type Package struct {
	Pkg         string `json:"pkg"`
	PkgName     string `json:"pkg_name"`
	Description string `json:"description"`
	SrcURL      string `json:"src_url"`
	Homepage    string `json:"homepage"`
	DownloadURL string `json:"download_url"`
	Bsum        string `json:"bsum"`
}

// Utility function to replace `|` in string fields
func replacePipeFields(pkg *Package) {
	pkg.PkgName = strings.ReplaceAll(pkg.PkgName, "|", pipeRepl)
	pkg.Description = strings.ReplaceAll(pkg.Description, "|", pipeRepl)
	pkg.SrcURL = strings.ReplaceAll(pkg.SrcURL, "|", pipeRepl)
	pkg.Homepage = strings.ReplaceAll(pkg.Homepage, "|", pipeRepl)
	pkg.DownloadURL = strings.ReplaceAll(pkg.DownloadURL, "|", pipeRepl)
}

// Utility function to replace empty strings with "nil"
func replaceEmptyWithNil(value string) string {
	if value == "" {
		return "nil"
	}
	return value
}

// Utility function to process PkgName as required
func processPkgName(pkgName string) string {
	// Step 1: Convert to lowercase
	pkgName = strings.ToLower(pkgName)
	// Step 2: Replace spaces with hyphens
	pkgName = strings.ReplaceAll(pkgName, " ", "-")
	// Step 3: Append `.appbundle` if not already present
	if !strings.HasSuffix(pkgName, ".appbundle") {
		pkgName += ".dwfs.appbundle"
	}
	return pkgName
}

func main() {
	url := "https://github.com/xplshn/AppBundleHUB/releases/download/latest_metadata/metadata_x86_64-Linux.json"

	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Error fetching JSON data:", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading JSON data:", err)
		return
	}

	var metadata Metadata
	err = json.Unmarshal(body, &metadata)
	if err != nil {
		fmt.Println("Error parsing JSON data:", err)
		return
	}

	file, err := os.Create("AM.txt")
	if err != nil {
		fmt.Println("Error creating output file:", err)
		return
	}
	defer file.Close()

	// Process each package and write to the file
	for _, pkg := range append(append(metadata.Bin, metadata.Pkg...), metadata.Base...) {
		// Replace empty fields with "nil"
		pkg.PkgName = replaceEmptyWithNil(pkg.PkgName)
		pkg.Description = replaceEmptyWithNil(pkg.Description)
		pkg.SrcURL = replaceEmptyWithNil(pkg.SrcURL)
		pkg.Homepage = replaceEmptyWithNil(pkg.Homepage)
		pkg.DownloadURL = replaceEmptyWithNil(pkg.DownloadURL)

		// If both SrcURL and Homepage are empty, fall back to "nil"
		webURL := pkg.SrcURL
		if webURL == "nil" {
			webURL = pkg.Homepage
		}
		if webURL == "nil" {
			webURL = pkg.DownloadURL
		}

		// Replace `|` in fields
		replacePipeFields(&pkg)

		// Process PkgName
		pkgName := processPkgName(pkg.PkgName)

		// Handle bsum
		bsum := pkg.Bsum
		if len(bsum) > 12 {
			bsum = bsum[:12]
		} else {
			bsum = "nil"
		}

		// Write formatted data to file
		file.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			pkgName, pkg.Description, webURL, pkg.DownloadURL, bsum))
	}

	fmt.Println("Data has been written to AM.txt")
}
