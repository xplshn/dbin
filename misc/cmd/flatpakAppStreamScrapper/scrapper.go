package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/fxamacker/cbor/v2"
	"github.com/goccy/go-json"
	minify "github.com/tdewolff/minify/v2"
	mjson "github.com/tdewolff/minify/v2/json"
)

type ScreenshotImage struct {
	Type   string `xml:"type,attr"`
	Width  string `xml:"width,attr"`
	Height string `xml:"height,attr"`
	Url    string `xml:",innerxml"`
}

type Screenshot struct {
	Type    string          `xml:"type,attr"`
	Caption string          `xml:"caption"`
	Images  []ScreenshotImage `xml:"image"`
}

type Component struct {
	Id         string      `xml:"id"`
	Screenshots []Screenshot `xml:"screenshots>screenshot"`
	Icons       []struct {
		Type   string `xml:"type,attr"`
		Width  string `xml:"width,attr"`
		Height string `xml:"height,attr"`
		Url    string `xml:",innerxml"`
	} `xml:"icon"`
	Url []struct {
		Type string `xml:"type,attr"`
		Url  string `xml:",chardata"`
	} `xml:"url"`
}

type Components struct {
	XMLName    xml.Name    `xml:"components"`
	Components []Component `xml:"component"`
}

type AppStreamData struct {
	AppId       string   `json:"app_id,omitempty" cbor:"app_id,omitempty"`
	Icons       []string `json:"icons,omitempty" cbor:"icons,omitempty"`
	Screenshots []string `json:"screenshots,omitempty" cbor:"screenshots,omitempty"`
}

func downloadFile(url string, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func saveAll(filename string, metadata []AppStreamData) error {
	if err := saveJSON(filename, metadata); err != nil {
		return err
	}
	return saveCBOR(filename, metadata)
}

func saveCBOR(filename string, metadata []AppStreamData) error {
	cborData, err := cbor.Marshal(metadata)
	if err != nil {
		return err
	}
	return os.WriteFile(filename+".cbor", cborData, 0644)
}

func saveJSON(filename string, metadata []AppStreamData) error {
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
	tmpDir := os.TempDir()
	xmlFilePath := filepath.Join(tmpDir, "FLATPAK_APPSTREAM.xml")

	// Check if the file already exists
	if _, err := os.Stat(xmlFilePath); os.IsNotExist(err) {
		// Download the file if it doesn't exist
		url := "https://github.com/Azathothas/pkgcache/raw/refs/heads/main/FLATPAK_APPSTREAM.xml"
		if err := downloadFile(url, xmlFilePath); err != nil {
			fmt.Printf("Error downloading file: %v\n", err)
			return
		}
	}

	// Read and parse the XML file
	xmlData, err := os.ReadFile(xmlFilePath)
	if err != nil {
		fmt.Printf("Error reading XML file: %v\n", err)
		return
	}

	var components Components
	if err := xml.Unmarshal(xmlData, &components); err != nil {
		fmt.Printf("Error parsing XML file: %v\n", err)
		return
	}

	// Process the components
	var metadata []AppStreamData
	for _, component := range components.Components {
		var icons []string
		var screenshots []string

		// Filter icons to include only those of type "remote" and size 128x128 or larger
		for _, icon := range component.Icons {
			if icon.Type == "remote" {
				width, err1 := strconv.Atoi(icon.Width)
				height, err2 := strconv.Atoi(icon.Height)
				if err1 == nil && err2 == nil && width >= 128 && height >= 128 {
					icons = append(icons, icon.Url)
				}
			}
		}

		// Select the largest screenshot
		for _, screenshot := range component.Screenshots {
			var largestImage ScreenshotImage
			var largestArea int
			for _, image := range screenshot.Images {
				if image.Type == "source" || image.Type == "default" {
					width, err1 := strconv.Atoi(image.Width)
					height, err2 := strconv.Atoi(image.Height)
					if err1 == nil && err2 == nil {
						area := width * height
						if area > largestArea {
							largestArea = area
							largestImage = image
						}
					}
				}
			}
			if largestImage.Url != "" {
				screenshots = append(screenshots, largestImage.Url)
			}
		}

		// Only add the entry if there are icons or screenshots
		if len(icons) > 0 || len(screenshots) > 0 {
			metadata = append(metadata, AppStreamData{
				AppId:       component.Id,
				Icons:        icons,
				Screenshots: screenshots,
			})
		}
	}

	// Save the metadata to CBOR and JSON files
	if err := saveAll("appstream_metadata", metadata); err != nil {
		fmt.Printf("Error saving metadata: %v\n", err)
	} else {
		fmt.Println("Metadata saved successfully.")
	}
}
