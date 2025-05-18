package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/goccy/go-json"
	"github.com/goccy/go-yaml"
	"golang.org/x/term"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"

	"github.com/pkg/xattr"
	"github.com/zeebo/blake3"
)

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

func isExecutable(filePath string) bool {
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && (info.Mode().Perm()&0o111) != 0
}

func parseBinaryEntry(entry binaryEntry, ansi bool) string {
	if ansi && term.IsTerminal(int(os.Stdout.Fd())) {
		return entry.Name + "\033[94m" + ternary(entry.PkgId != "", "#"+entry.PkgId, "") + "\033[0m" + ternary(entry.Repository.Name != "", "\033[92m"+"@"+entry.Repository.Name+"\033[0m", "")
	}
	return entry.Name + ternary(entry.PkgId != "", "#"+entry.PkgId, "") + ternary(entry.Repository.Name != "", "@"+entry.Repository.Name, "")
}

func stringToBinaryEntry(input string) binaryEntry {
	var bEntry binaryEntry

	parts := strings.SplitN(input, "@", 2)
	bEntry.Name = parts[0]

	if len(parts) > 1 {
		bEntry.Repository.Name = parts[1]
	}

	nameParts := strings.SplitN(bEntry.Name, "#", 2)
	bEntry.Name = nameParts[0]

	if len(nameParts) > 1 {
		idVer := strings.SplitN(nameParts[1], ":", 2)
		bEntry.PkgId = idVer[0]
		if len(idVer) > 1 {
			bEntry.Version = idVer[1]
		}
	}

	return bEntry
}

func arrStringToArrBinaryEntry(args []string) []binaryEntry {
	var entries []binaryEntry
	for _, arg := range args {
		entries = append(entries, stringToBinaryEntry(arg))
	}
	return entries
}

func binaryEntriesToArrString(entries []binaryEntry, ansi bool) []string {
	var result []string
	seen := make(map[string]bool)

	for _, entry := range entries {
		key := parseBinaryEntry(entry, ansi)
		if !seen[key] {
			result = append(result, key)
		} else {
			seen[key] = true
			if entry.Version != "" {
				result = append(result, key, ternary(!ansi, entry.Version, "\033[90m"+entry.Version+"\033[0m"))
			}
		}
	}

	return result
}

func validateProgramsFrom(config *Config, programsToValidate []binaryEntry, uRepoIndex []binaryEntry) ([]binaryEntry, error) {
	programsEntries, err := listBinaries(uRepoIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote binaries: %w", err)
	}

	files, err := listFilesInDir(config.InstallDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list files in %s: %w", config.InstallDir, err)
	}

	validPrograms := make([]binaryEntry, 0, len(programsToValidate))

	validate := func(file string) (binaryEntry, bool) {
		trackedBEntry := bEntryOfinstalledBinary(file)
		if config.RetakeOwnership {
			trackedBEntry.Name = filepath.Base(file)
			if trackedBEntry.PkgId == "" {
				trackedBEntry.PkgId = "!retake"
			}
		}
		for _, remoteEntry := range programsEntries {
			if remoteEntry.Name == trackedBEntry.Name && (remoteEntry.PkgId == trackedBEntry.PkgId || trackedBEntry.PkgId == "!retake") {
				return trackedBEntry, true
			}
		}
		return binaryEntry{}, false
	}

	if len(programsToValidate) == 0 {
		for _, file := range files {
			if bEntry, valid := validate(file); valid {
				validPrograms = append(validPrograms, bEntry)
			}
		}
	} else {
		for _, program := range programsToValidate {
			file := filepath.Join(config.InstallDir, program.Name)
			if bEntry, valid := validate(file); valid {
				validPrograms = append(validPrograms, bEntry)
			}
		}
	}

	return validPrograms, nil
}

func bEntryOfinstalledBinary(binaryPath string) binaryEntry {
	if isSymlink(binaryPath) {
		return binaryEntry{}
	}
	trackedBEntry, err := readEmbeddedBEntry(binaryPath)
	if err != nil || trackedBEntry.Name == "" {
		return binaryEntry{}
	}
	return trackedBEntry
}

func getTerminalWidth() int {
	w, _, _ := term.GetSize(int(os.Stdout.Fd()))
	if w != 0 {
		return w
	}
	return 80
}

func truncateSprintf(indicator, format string, a ...any) string {
	text := fmt.Sprintf(format, a...)
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return text
	}

	width := uint(getTerminalWidth() - len(indicator))
	if width <= 0 {
		return text
	}

	var out bytes.Buffer
	var visibleCount uint
	var inEscape bool
	var escBuf bytes.Buffer

	for i := range len(text) {
		c := text[i]

		switch {
		case c == '\x1b':
			inEscape = true
			escBuf.Reset()
			escBuf.WriteByte(c)
		case inEscape:
			escBuf.WriteByte(c)
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				inEscape = false
				out.Write(escBuf.Bytes())
			}
		default:
			if visibleCount >= width {
				continue
			}
			out.WriteByte(c)
			visibleCount++
		}
	}

	result := out.String()
	if strings.HasSuffix(text, "\n") {
		if visibleCount >= width {
			return result + indicator + "\n"
		}
		return result
	}
	if visibleCount >= width {
		return result + indicator
	}
	return result
}

func truncatePrintf(disableTruncation bool, format string, a ...any) (n int, err error) {
	if disableTruncation {
		return fmt.Printf(format, a...)
	}
	text := truncateSprintf(Indicator, format, a...)
	return fmt.Print(text)
}

func listFilesInDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files, nil
}

func embedBEntry(binaryPath string, bEntry binaryEntry) error {
	bEntry.Version = ""
	if err := xattr.Set(binaryPath, "user.FullName", []byte(parseBinaryEntry(bEntry, false))); err != nil {
		return fmt.Errorf("failed to set xattr for %s: %w", binaryPath, err)
	}
	return nil
}

func readEmbeddedBEntry(binaryPath string) (binaryEntry, error) {
	if !fileExists(binaryPath) {
		return binaryEntry{}, fmt.Errorf("error: Tried to get EmbeddedBEntry of non-existant file: %s", binaryPath)
	}

	fullName, err := xattr.Get(binaryPath, "user.FullName")
	if err != nil {
		return binaryEntry{}, fmt.Errorf("xattr: user.FullName attribute not found for binary: %s", binaryPath)
	}

	return stringToBinaryEntry(string(fullName)), nil
}

func accessCachedOrFetch(url, filename string, cfg *Config) ([]byte, error) {
	cacheFilePath := filepath.Join(cfg.CacheDir, ternary(filename != "", "."+filename, "."+filepath.Base(url)))

	if err := os.MkdirAll(cfg.CacheDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating cache directory %s: %v", cfg.CacheDir, err)
	}

	// Check if the file is already cached and not expired
	fileInfo, err := os.Stat(cacheFilePath)
	if err == nil && time.Since(fileInfo.ModTime()).Hours() < 6 {
		// Read from cache file
		bodyBytes, err := os.ReadFile(cacheFilePath)
		if err != nil {
			return nil, fmt.Errorf("error reading cached file %s: %v", cacheFilePath, err)
		}
		return bodyBytes, nil
	}

	// Fetch from remote
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request for %s: %v", url, err)
	}
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("dbin", strconv.FormatFloat(Version, 'f', -1, 32))
	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching from %s: %v. Please check your configuration's repo_urls. Ensure your network has access to the internet", url, err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error fetching from %s: received status code %d", url, response.StatusCode)
	}

	// Read the entire body
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading from %s: %v", url, err)
	}

	// Write to cache file
	err = os.WriteFile(cacheFilePath, bodyBytes, 0644)
	if err != nil {
		return nil, fmt.Errorf("error writing to cached file %s: %v", cacheFilePath, err)
	}

	return bodyBytes, nil
}

func decodeRepoIndex(config *Config) ([]binaryEntry, error) {
	var binaryEntries []binaryEntry
	var parsedRepos = make(map[string]bool)

	for _, repo := range config.Repositories {
		if parsedRepos[repo.URL] {
			continue // Skip if URL is already parsed
		}

		var bodyBytes []byte
		var err error

		if strings.HasPrefix(repo.URL, "file://") {
			filePath := strings.TrimPrefix(repo.URL, "file://")
			bodyBytes, err = os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("error opening file %s: %v", filePath, err)
			}
		} else {
			bodyBytes, err = accessCachedOrFetch(repo.URL, "", config)
			if err != nil {
				return nil, err
			}
		}

		// Create a new reader from body bytes for decompression
		bodyReader := io.NopCloser(bytes.NewReader(bodyBytes))

		switch {
		case strings.HasSuffix(repo.URL, ".gz"):
			repo.URL = strings.TrimSuffix(repo.URL, ".gz")
			gzipReader, err := gzip.NewReader(bodyReader)
			if err != nil {
				return nil, fmt.Errorf("error creating gzip reader for %s: %v", repo.URL, err)
			}
			defer gzipReader.Close()

			// Read the decompressed data
			bodyBytes, err = io.ReadAll(gzipReader)
			if err != nil {
				return nil, fmt.Errorf("error reading gzip data from %s: %v", repo.URL, err)
			}
		case strings.HasSuffix(repo.URL, ".zst"):
			repo.URL = strings.TrimSuffix(repo.URL, ".zst")
			zstdReader, err := zstd.NewReader(bodyReader)
			if err != nil {
				return nil, fmt.Errorf("error creating zstd reader for %s: %v", repo.URL, err)
			}
			defer zstdReader.Close()

			// Read the decompressed data
			bodyBytes, err = io.ReadAll(zstdReader.IOReadCloser())
			if err != nil {
				return nil, fmt.Errorf("error reading zstd data from %s: %v", repo.URL, err)
			}
		}

		var repoIndex map[string][]binaryEntry
		switch {
		case strings.HasSuffix(repo.URL, ".cbor"):
			if err := cbor.Unmarshal(bodyBytes, &repoIndex); err != nil {
				return nil, fmt.Errorf("error decoding CBOR from %s: %v", repo.URL, err)
			}
		case strings.HasSuffix(repo.URL, ".json"):
			if err := json.Unmarshal(bodyBytes, &repoIndex); err != nil {
				return nil, fmt.Errorf("error decoding JSON from %s: %v", repo.URL, err)
			}
		case strings.HasSuffix(repo.URL, ".yaml"):
			if err := yaml.Unmarshal(bodyBytes, &repoIndex); err != nil {
				return nil, fmt.Errorf("error decoding YAML from %s: %v", repo.URL, err)
			}
		default:
			return nil, fmt.Errorf("unsupported format for URL: %s", repo.URL)
		}

		for repoName, entries := range repoIndex {
			for _, entry := range entries {
				entry.Repository = repo
				entry.Repository.Name = repoName
				binaryEntries = append(binaryEntries, entry)
			}
		}

		parsedRepos[repo.URL] = true
	}

	return binaryEntries, nil
}

func calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := blake3.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func isSymlink(filePath string) bool {
	fileInfo, err := os.Lstat(filePath)
	return err == nil && fileInfo.Mode()&os.ModeSymlink != 0
}

func ternary[T any](cond bool, vtrue, vfalse T) T {
	if cond {
		return vtrue
	}
	return vfalse
}
