package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

func stringToBinaryEntry(input string) binaryEntry {
	var bEntry binaryEntry

	parts := strings.SplitN(input, "#", 2)
	bEntry.Name = parts[0]

	if len(parts) > 1 {
		idVer := strings.SplitN(parts[1], ":", 2)
		bEntry.PkgId = idVer[0]
		if len(idVer) > 1 {
			bEntry.Version = idVer[1]
		}
	} else {
		bEntry.Name = input
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

func parseBinaryEntry(entry binaryEntry, ansi bool) string {
	if ansi && term.IsTerminal(int(os.Stdout.Fd())) {
		return entry.Name + "\033[94m" + ternary(entry.PkgId != "", "#"+entry.PkgId, "") + "\033[0m"
	}
	return entry.Name + ternary(entry.PkgId != "", "#"+entry.PkgId, "")
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

func truncateSprintf(indicator, format string, a ...interface{}) string {
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

	for i := 0; i < len(text); i++ {
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

func truncatePrintf(disableTruncation bool, format string, a ...interface{}) (n int, err error) {
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

func decodeRepoIndex(config *Config) ([]binaryEntry, error) {
    var binaryEntries []binaryEntry

    for _, url := range config.RepoURLs {
        if url == "" {
            return nil, fmt.Errorf("repository index URL is empty. Please check your configuration or remove it")
        }

        var bodyReader io.ReadCloser
        var err error

        if strings.HasPrefix(url, "file://") {
            filePath := strings.TrimPrefix(url, "file://")
            bodyReader, err = os.Open(filePath)
            if err != nil {
                return nil, fmt.Errorf("error opening file %s: %v", filePath, err)
            }
            defer bodyReader.Close()
        } else {
            if config.NoConfig {
                bodyReader, err = fetchMetadata(url)
                if err != nil {
                    return nil, err
                }
            } else {
                cachedFilePath := filepath.Join(config.CacheDir, "."+filepath.Base(url))

                // Ensure the cache directory exists
                if err := os.MkdirAll(config.CacheDir, 0755); err != nil {
                    return nil, fmt.Errorf("error creating cache directory %s: %v", config.CacheDir, err)
                }

                fileInfo, err := os.Stat(cachedFilePath)
                if err == nil && time.Since(fileInfo.ModTime()).Hours() < 6 {
                    bodyReader, err = os.Open(cachedFilePath)
                    if err != nil {
                        return nil, fmt.Errorf("error opening cached file %s: %v", cachedFilePath, err)
                    }
                    defer bodyReader.Close()
                } else {
                    bodyReader, err = fetchMetadata(url)
                    if err != nil {
                        return nil, err
                    }
                    defer bodyReader.Close()

                    cachedFile, err := os.Create(cachedFilePath)
                    if err != nil {
                        return nil, fmt.Errorf("error creating cached file %s: %v", cachedFilePath, err)
                    }
                    defer cachedFile.Close()

                    _, err = io.Copy(cachedFile, bodyReader)
                    if err != nil {
                        return nil, fmt.Errorf("error writing to cached file %s: %v", cachedFilePath, err)
                    }

                    bodyReader, err = os.Open(cachedFilePath)
                    if err != nil {
                        return nil, fmt.Errorf("error opening cached file %s: %v", cachedFilePath, err)
                    }
                    defer bodyReader.Close()
                }
            }
        }

        if strings.HasSuffix(url, ".gz") {
            url = strings.TrimSuffix(url, ".gz")
            bodyReader, err = gzip.NewReader(bodyReader)
            if err != nil {
                return nil, fmt.Errorf("error creating gzip reader for %s: %v", url, err)
            }
            defer bodyReader.Close()
        }
        if strings.HasSuffix(url, ".zst") {
            url = strings.TrimSuffix(url, ".zst")
            zstdReader, err := zstd.NewReader(bodyReader)
            if err != nil {
                return nil, fmt.Errorf("error creating zstd reader for %s: %v", url, err)
            }
            defer zstdReader.Close()
            bodyReader = zstdReader.IOReadCloser()
        }

        body := new(bytes.Buffer)
        if _, err := io.Copy(body, bodyReader); err != nil {
            return nil, fmt.Errorf("error reading from %s: %v", url, err)
        }

        var repoIndex map[string][]binaryEntry
        switch {
        case strings.HasSuffix(url, ".cbor"):
            if err := cbor.Unmarshal(body.Bytes(), &repoIndex); err != nil {
                return nil, fmt.Errorf("error decoding CBOR from %s: %v", url, err)
            }
        case strings.HasSuffix(url, ".json"):
            if err := json.Unmarshal(body.Bytes(), &repoIndex); err != nil {
                return nil, fmt.Errorf("error decoding JSON from %s: %v", url, err)
            }
        case strings.HasSuffix(url, ".yaml"):
            if err := yaml.Unmarshal(body.Bytes(), &repoIndex); err != nil {
                return nil, fmt.Errorf("error decoding YAML from %s: %v", url, err)
            }
        default:
            return nil, fmt.Errorf("unsupported format for URL: %s", url)
        }

        for _, entries := range repoIndex {
            binaryEntries = append(binaryEntries, entries...)
        }
    }

    return binaryEntries, nil
}

func fetchMetadata(url string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request for %s: %v", url, err)
	}

	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("dbin", Version)

	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching from %s: %v. Please check your configuration's repo_urls. Ensure your network has access to the internet", url, err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error fetching from %s: received status code %d", url, response.StatusCode)
	}
	return response.Body, nil
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
