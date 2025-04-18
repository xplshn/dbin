package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/goccy/go-json"
	minify "github.com/tdewolff/minify/v2"
	mjson "github.com/tdewolff/minify/v2/json"
)

type Tag struct {
	XMLName xml.Name
	Content string `xml:",innerxml"`
	Lang    string `xml:"lang,attr"`
}

type ScreenshotImage struct {
	Type   string `xml:"type,attr"`
	Width  string `xml:"width,attr"`
	Height string `xml:"height,attr"`
	Url    string `xml:",innerxml"`
}

type Screenshot struct {
	Type    string            `xml:"type,attr"`
	Caption string            `xml:"caption"`
	Images  []ScreenshotImage `xml:"image"`
}

type Component struct {
	Id          string       `xml:"id"`
	Screenshots []Screenshot `xml:"screenshots>screenshot"`
	Description []Tag        `xml:"description>p"`
	Categories  []Tag        `xml:"categories>category"`
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
	AppId           string   `json:"app_id,omitempty"           `
	Icons           []string `json:"icons,omitempty"            `
	Screenshots     []string `json:"screenshots,omitempty"      `
	Categories      string   `json:"categories,omitempty"       `
	RichDescription string   `json:"rich_description,omitempty" `
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
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(metadata); err != nil {
		return err
	}

	jsonData := buf.Bytes()

	var prettyBuf bytes.Buffer
	if err := json.Indent(&prettyBuf, jsonData, "", " "); err != nil {
		return err
	}

	if err := os.WriteFile(filename+".json", prettyBuf.Bytes(), 0644); err != nil {
		return err
	}

	m := minify.New()
	m.AddFunc("application/json", mjson.Minify)
	minifiedJSON, err := m.Bytes("application/json", jsonData)
	if err != nil {
		return err
	}

	return os.WriteFile(filename+".min.json", minifiedJSON, 0644)
}

func getCategoriesString(categories []Tag) string {
	var categoryStrings []string
	for _, cat := range categories {
		if cat.Content != "" {
			categoryStrings = append(categoryStrings, cat.Content)
		}
	}
	return strings.Join(categoryStrings, ",")
}

func getRichDescription(descriptions []Tag) string {
	var richText strings.Builder

	var bestDesc string
	for _, desc := range descriptions {
		if desc.Lang == "en" {
			bestDesc = desc.Content
			break
		} else if bestDesc == "" {
			bestDesc = desc.Content
		}
	}

	richText.WriteString(bestDesc)
	return richText.String()
}

func main() {
	tmpDir := os.TempDir()
	xmlFilePath := filepath.Join(tmpDir, "FLATPAK_APPSTREAM.xml")

	if _, err := os.Stat(xmlFilePath); os.IsNotExist(err) {
		url := "https://github.com/Azathothas/pkgcache/raw/refs/heads/main/FLATPAK_APPSTREAM.xml"
		if err := downloadFile(url, xmlFilePath); err != nil {
			fmt.Printf("Error downloading file: %v\n", err)
			return
		}
	}

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

	var metadata []AppStreamData
	for _, component := range components.Components {
		var icons []string
		var screenshots []string

		for _, icon := range component.Icons {
			if icon.Type == "remote" {
				width, err1 := strconv.Atoi(icon.Width)
				height, err2 := strconv.Atoi(icon.Height)
				if err1 == nil && err2 == nil && width >= 128 && height >= 128 {
					icons = append(icons, icon.Url)
				}
			}
		}

		for _, screenshot := range component.Screenshots {
			// Sort images by area (largest first)
			sort.Slice(screenshot.Images, func(i, j int) bool {
				widthI, _ := strconv.Atoi(screenshot.Images[i].Width)
				heightI, _ := strconv.Atoi(screenshot.Images[i].Height)
				widthJ, _ := strconv.Atoi(screenshot.Images[j].Width)
				heightJ, _ := strconv.Atoi(screenshot.Images[j].Height)
				areaI := widthI * heightI
				areaJ := widthJ * heightJ
				return areaI > areaJ
			})

			for _, image := range screenshot.Images {
				if image.Type == "source" || image.Type == "default" {
					screenshots = append(screenshots, image.Url)
				}
			}
		}

		categories := getCategoriesString(component.Categories)

		richDescription := getRichDescription(component.Description)

		metadata = append(metadata, AppStreamData{
			AppId:           component.Id,
			Icons:           icons,
			Screenshots:     screenshots,
			Categories:      categories,
			RichDescription: richDescription,
		})
	}

	if err := saveAll("appstream_metadata", metadata); err != nil {
		fmt.Printf("Error saving metadata: %v\n", err)
	} else {
		fmt.Println("Metadata saved successfully.")
	}
}
