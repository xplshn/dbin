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

	"github.com/fxamacker/cbor/v2" //"github.com/shamaton/msgpack/v2"
	"github.com/goccy/go-json"
	"github.com/goccy/go-yaml"
	"golang.org/x/term"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"

	"github.com/pkg/xattr"
	"github.com/zeebo/blake3"
	"github.com/zeebo/errs"
)

var (
	errFileAccess      = errs.Class("file access error")
	errFileTypeInvalid = errs.Class("invalid file type")
	errFileNotFound    = errs.Class("file not found")
	errXAttr           = errs.Class("xattr error")
	errCacheAccess     = errs.Class("cache access error")
	delimiters         = []rune{
		'#', // .PkgID
		':', // .Version
		'@', // .Repository.Name
	}
)

const (
	blueColor         = "\x1b[0;34m"
	cyanColor         = "\x1b[0;36m"
	intenseBlackColor = "\x1b[0;90m"
	blueBgWhiteFg     = "\x1b[48;5;4m"
	resetColor        = "\x1b[0m"
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
	result := entry.Name

	if ansi && term.IsTerminal(int(os.Stdout.Fd())) {
		if entry.PkgID != "" {
			result += blueColor + string(delimiters[0]) + entry.PkgID + resetColor
		}
		if entry.Version != "" {
			result += cyanColor + string(delimiters[1]) + entry.Version + resetColor
		}
		if entry.Repository.Name != "" {
			result += intenseBlackColor + string(delimiters[2]) + entry.Repository.Name + resetColor
		}
		return result
	}

	if entry.PkgID != "" {
		result += string(delimiters[0]) + entry.PkgID
	}
	//if entry.Version != "" {
	//	result += string(delimiters[1]) + entry.Version
	//}
	if entry.Repository.Name != "" {
		result += string(delimiters[2]) + entry.Repository.Name
	}
	return result
}

func stringToBinaryEntry(input string) binaryEntry {
	var bEntry binaryEntry

	// Accepted formats:
	// name
	// name#id
	// name#id:version
	// name#id@repo
	// name#id:version@repo

	// Split by repository delimiter (@)
	parts := strings.SplitN(input, string(delimiters[2]), 2)
	bEntry.Name = parts[0]
	if len(parts) > 1 {
		bEntry.Repository.Name = parts[1]
	}

	// Split name part by ID delimiter (#)
	nameParts := strings.SplitN(bEntry.Name, string(delimiters[0]), 2)
	bEntry.Name = nameParts[0]
	if len(nameParts) > 1 {
		// Split ID part by version delimiter (:)
		idVer := strings.SplitN(nameParts[1], string(delimiters[1]), 2)
		bEntry.PkgID = idVer[0]
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

func validateProgramsFrom(config *config, programsToValidate []binaryEntry, uRepoIndex []binaryEntry) ([]binaryEntry, error) {
	var (
		programsEntries []binaryEntry
		validPrograms   []binaryEntry
		err             error
		files           []string
	)

	if config.RetakeOwnership {
		if uRepoIndex == nil {
			uRepoIndex, err = fetchRepoIndex(config)
			if err != nil {
				return nil, err
			}
		}

		programsEntries, err = listBinaries(uRepoIndex)
		if err != nil {
			return nil, fmt.Errorf("failed to list remote binaries: %w", err)
		}
	}

	files, err = listFilesInDir(config.InstallDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list files in %s: %w", config.InstallDir, err)
	}

	var toProcess []string
	if len(programsToValidate) == 0 {
		// All files in install dir
		toProcess = files
	} else {
		// Only the specific binaries requested
		toProcess = toProcess[:0]
		for i := range programsToValidate {
			file := filepath.Join(config.InstallDir, programsToValidate[i].Name)
			toProcess = append(toProcess, file)
		}
	}

	// Only allocate once, at most as many entries as files to process
	validPrograms = make([]binaryEntry, 0, len(toProcess))

	for i := range toProcess {
		file := toProcess[i]
		if !isExecutable(file) || (len(programsToValidate) != 0 && !fileExists(file)) {
			continue
		}

		baseName := filepath.Base(file)
		trackedBEntry := bEntryOfinstalledBinary(file)

		if config.RetakeOwnership {
			if trackedBEntry.Name == "" {
				trackedBEntry.Name = baseName
				trackedBEntry.PkgID = "!retake"
			}

			for j := range programsEntries {
				if programsEntries[j].Name == trackedBEntry.Name {
					validPrograms = append(validPrograms, trackedBEntry)
					break
				}
			}
			continue
		}

		// Non-retake: must have metadata and match uRepoIndex
		if trackedBEntry.Name == "" {
			continue
		}
		if uRepoIndex == nil {
			// If uRepoIndex is nil, append any entry with Name != ""
			validPrograms = append(validPrograms, trackedBEntry)
			continue
		}
		for j := range uRepoIndex {
			if uRepoIndex[j].Name == trackedBEntry.Name && uRepoIndex[j].PkgID == trackedBEntry.PkgID {
				validPrograms = append(validPrograms, trackedBEntry)
				break
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
	text := truncateSprintf("..>", format, a...)
	return fmt.Print(text)
}

func listFilesInDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errFileAccess.Wrap(err)
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
		return errXAttr.Wrap(err)
	}
	return nil
}

func readEmbeddedBEntry(binaryPath string) (binaryEntry, error) {
	if !fileExists(binaryPath) {
		return binaryEntry{}, errFileNotFound.New("Tried to get EmbeddedBEntry of non-existent file: %s", binaryPath)
	}

	fullName, err := xattr.Get(binaryPath, "user.FullName")
	if err != nil {
		return binaryEntry{}, errXAttr.New("xattr: user.FullName attribute not found for binary: %s", binaryPath)
	}

	return stringToBinaryEntry(string(fullName)), nil
}

func accessCachedOrFetch(url, filename string, cfg *config) ([]byte, error) {
	cacheFilePath := filepath.Join(cfg.CacheDir, ternary(filename != "", "."+filename, "."+filepath.Base(url)))

	if err := os.MkdirAll(cfg.CacheDir, 0755); err != nil {
		return nil, errCacheAccess.Wrap(err)
	}

	fileInfo, err := os.Stat(cacheFilePath)
	if err == nil && time.Since(fileInfo.ModTime()).Hours() < 6 {
		bodyBytes, err := os.ReadFile(cacheFilePath)
		if err != nil {
			return nil, errCacheAccess.Wrap(err)
		}
		return bodyBytes, nil
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, errCacheAccess.Wrap(err)
	}
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("dbin", strconv.FormatFloat(version, 'f', -1, 32))
	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return nil, errCacheAccess.Wrap(err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, errCacheAccess.New("received status code %d", response.StatusCode)
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, errCacheAccess.Wrap(err)
	}

	err = os.WriteFile(cacheFilePath, bodyBytes, 0644)
	if err != nil {
		return nil, errCacheAccess.Wrap(err)
	}

	return bodyBytes, nil
}

func decodeRepoIndex(config *config) ([]binaryEntry, error) {
	var binaryEntries []binaryEntry
	var parsedRepos = make(map[string]bool)

	for _, repo := range config.Repositories {
		if parsedRepos[repo.URL] {
			continue
		}

		var bodyBytes []byte
		var err error

		if strings.HasPrefix(repo.URL, "file://") {
			bodyBytes, err = os.ReadFile(strings.TrimPrefix(repo.URL, "file://"))
			if err != nil {
				return nil, errFileAccess.Wrap(err)
			}
		} else {
			bodyBytes, err = accessCachedOrFetch(repo.URL, "", config)
			if err != nil {
				return nil, err
			}
		}

		bodyReader := io.NopCloser(bytes.NewReader(bodyBytes))

		switch {
		case strings.HasSuffix(repo.URL, ".gz"):
			repo.URL = strings.TrimSuffix(repo.URL, ".gz")
			gzipReader, err := gzip.NewReader(bodyReader)
			if err != nil {
				return nil, errFileTypeInvalid.Wrap(err)
			}
			defer gzipReader.Close()

			bodyBytes, err = io.ReadAll(gzipReader)
			if err != nil {
				return nil, errFileAccess.Wrap(err)
			}
		case strings.HasSuffix(repo.URL, ".zst"):
			repo.URL = strings.TrimSuffix(repo.URL, ".zst")
			zstdReader, err := zstd.NewReader(bodyReader)
			if err != nil {
				return nil, errFileTypeInvalid.Wrap(err)
			}
			defer zstdReader.Close()

			bodyBytes, err = io.ReadAll(zstdReader.IOReadCloser())
			if err != nil {
				return nil, errFileAccess.Wrap(err)
			}
		}

		var repoIndex map[string][]binaryEntry
		switch {
		//case strings.HasSuffix(repo.URL, ".msgp"):
		//	if err := msgpack.Unmarshal(bodyBytes, &repoIndex); err != nil {
		//		return nil, errFileTypeInvalid.Wrap(err)
		//	}
		case strings.HasSuffix(repo.URL, ".cbor"):
			if err := cbor.Unmarshal(bodyBytes, &repoIndex); err != nil {
				return nil, errFileTypeInvalid.Wrap(err)
			}
		case strings.HasSuffix(repo.URL, ".json"):
			if err := json.Unmarshal(bodyBytes, &repoIndex); err != nil {
				return nil, errFileTypeInvalid.Wrap(err)
			}
		case strings.HasSuffix(repo.URL, ".yaml"):
			if err := yaml.Unmarshal(bodyBytes, &repoIndex); err != nil {
				return nil, errFileTypeInvalid.Wrap(err)
			}
		default:
			return nil, errFileTypeInvalid.New("unsupported format for URL: %s", repo.URL)
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
		return "", errFileAccess.Wrap(err)
	}
	defer file.Close()

	hasher := blake3.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", errFileAccess.Wrap(err)
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
