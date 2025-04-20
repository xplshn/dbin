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

const lang = ""

type Tag struct {
	XMLName xml.Name
	Content string `xml:",innerxml"`
	Lang    string `xml:"lang,attr"`
}

type Components struct {
	XMLName    xml.Name    `xml:"components"`
	Components []Component `xml:"component"`
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

type Release struct {
	Version string `xml:"version,attr"`
	Date    string `xml:"date,attr"`
}

type Releases struct {
	Release []Release `xml:"release"`
}

type Component struct {
	Name        []Tag        `xml:"name"`
	Screenshots []Screenshot `xml:"screenshots>screenshot"`
	Summary     []Tag        `xml:"summary"`
	Description []Tag        `xml:"description>p"`
	Categories  []Tag        `xml:"categories>category"`
	Keywords    []Tag        `xml:"keywords>keyword"`
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
	Type           string `xml:"type,attr"`
	Id             string `xml:"id"`
	ProjectLicense string `xml:"project_license"`
	Launchable     struct {
		DesktopId string `xml:"desktop-id"`
	} `xml:"launchable"`
	ContentRating []Tag    `xml:"content_rating"`
	Releases      Releases `xml:"releases"`
}

type AppStreamData struct {
	AppId           string   `json:"app_id,omitempty"`
	Name            string   `json:"name,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	ContentRating   string   `json:"content_rating,omitempty"`
	Icons           []string `json:"icons,omitempty"`
	Screenshots     []string `json:"screenshots,omitempty"`
	Categories      string   `json:"categories,omitempty"`
	RichDescription string   `json:"rich_description,omitempty"`
	Version         string   `json:"version,omitempty"`
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
	return getContentByLang(descriptions)
}

func getName(names []Tag) string {
	return getContentByLang(names)
}

func getSummary(summaries []Tag) string {
	return getContentByLang(summaries)
}

func getContentRating(ratings []Tag) string {
	var contentRating strings.Builder
	for _, rating := range ratings {
		contentRating.WriteString(rating.Content)
	}
	return contentRating.String()
}

func getContentByLang(tags []Tag) string {
	for _, tag := range tags {
		if tag.Lang == lang {
			return tag.Content
		}
	}
	return ""
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
		name := getName(component.Name)
		summary := getSummary(component.Summary)
		contentRating := getContentRating(component.ContentRating)
		version := ""
		if len(component.Releases.Release) > 0 {
			version = component.Releases.Release[0].Version
		}

		metadata = append(metadata, AppStreamData{
			AppId:           component.Id,
			Name:            name,
			Summary:         summary,
			ContentRating:   contentRating,
			Icons:           icons,
			Screenshots:     screenshots,
			Categories:      categories,
			RichDescription: richDescription,
			Version:         version,
		})
	}

	if err := saveAll("appstream_metadata", metadata); err != nil {
		fmt.Printf("Error saving metadata: %v\n", err)
	} else {
		fmt.Println("Metadata saved successfully.")
	}
}
