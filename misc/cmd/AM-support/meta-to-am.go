package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

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

func main() {
	// dbin flavored metadata (has no "duplicates")
	url := "https://raw.githubusercontent.com/xplshn/dbin-metadata/refs/heads/master/misc/cmd/modMetadataAIO-ng/METADATA_AIO_amd64_linux.min.json"

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

	// file.WriteString("| name | description | weburl | download_url | b3sum[0..12] |\n")
	// file.WriteString("|------|-------------|--------|---------------|--------------|\n")

	// Process each package and write to the file
	for _, pkg := range append(append(metadata.Bin, metadata.Pkg...), metadata.Base...) {
		webURL := pkg.SrcURL
		if webURL == "" {
			webURL = pkg.Homepage
		}
		if webURL == "" {
			webURL = pkg.DownloadURL
		}

		bsum := pkg.Bsum
		if bsum == "" {
			bsum = "nil"
		} else if len(bsum) > 12 {
			bsum = pkg.Bsum[:12]
		}

		file.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			pkg.Pkg, pkg.Description, webURL, pkg.DownloadURL, bsum))
	}

	fmt.Println("Data has been written to AM.txt")
}
