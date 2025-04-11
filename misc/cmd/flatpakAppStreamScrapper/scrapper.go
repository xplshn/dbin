package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

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
	Image   ScreenshotImage `xml:"image"`
}

type Component struct {
	Id         string      `xml:"id"`
	Screenshots []Screenshot `xml:"screenshots>screenshot"`
	Url        []struct {
		Type string `xml:"type,attr"`
		Url  string `xml:",chardata"`
	} `xml:"url"`
}

type Components struct {
	XMLName    xml.Name    `xml:"components"`
	Components []Component `xml:"component"`
}

type AppStreamData struct {
	AppId       string   `json:"app_id" cbor:"app_id"`
	Icons       []string `json:"icons" cbor:"icons"`
	Screenshots []string `json:"screenshots" cbor:"screenshots"`
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

		for _, url := range component.Url {
			if url.Type == "icon" {
				icons = append(icons, url.Url)
			}
		}

		for _, screenshot := range component.Screenshots {
			if screenshot.Type == "source" || screenshot.Type == "default" {
				screenshots = append(screenshots, screenshot.Image.Url)
			}
		}

		metadata = append(metadata, AppStreamData{
			AppId:       component.Id,
			Icons:        icons,
			Screenshots: screenshots,
		})
	}

	if err := saveAll("appstream_metadata", metadata); err != nil {
		fmt.Printf("Error saving metadata: %v\n", err)
	} else {
		fmt.Println("Metadata saved successfully.")
	}
}
