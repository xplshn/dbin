package main

type snapshot struct {
    Commit  string `json:"commit"`
    Version string `json:"version,omitempty"`
}

type binaryEntry struct {
	Name        string   			`json:"pkg"                   `
	PrettyName  string   			`json:"pkg_name"              `
	PkgId       string   			`json:"pkg_id"                `
	Description string   			`json:"description,omitempty" `
	Version     string   			`json:"version,omitempty"     `
	DownloadURL string   			`json:"download_url,omitempty"`
	Icon        string              `json:"icon,omitempty"        `
	Size        string   			`json:"size,omitempty"        `
	Bsum        string   			`json:"bsum,omitempty"        `
	Shasum      string   			`json:"shasum,omitempty"      `
	BuildDate   string   			`json:"build_date,omitempty"  `
	BuildScript string   			`json:"build_script,omitempty"`
	BuildLog    string   			`json:"build_log,omitempty"   `
	Categories  string   			`json:"categories,omitempty"  `
	ExtraBins   string   			`json:"provides,omitempty"    `
	Screenshots []string            `json:"screenshots,omitempty" `
	License     []string   			`json:"license,omitempty"     `
	Notes       []string 			`json:"notes,omitempty"       `
	SrcURLs     []string 			`json:"src_urls,omitempty"    `
	WebURLs     []string 			`json:"web_urls,omitempty"    `
	Snapshots   []snapshot          `json:"snapshots,omitempty"   `
	Rank        uint16   			`json:"rank,omitempty"        `
	Repository  Repository
}
