package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/goccy/go-json"
	"golang.org/x/term"

	"github.com/klauspost/compress/gzip"

	"github.com/pkg/xattr"
	"github.com/zeebo/blake3"
)

// removeDuplicates removes duplicate elements from the list
func removeDuplicates[T comparable](elements []T) []T {
	seen := make(map[T]struct{})
	result := []T{}
	for _, element := range elements {
		if _, ok := seen[element]; !ok {
			seen[element] = struct{}{}
			result = append(result, element)
		}
	}
	return result
}

// contains returns true if the provided slice of []strings contains the word str
func contains(slice []string, str string) bool {
	for _, v := range slice {
		if v == str {
			return true
		}
	}
	return false
}

// fileExists checks if a file exists.
func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

// isDirectory checks if the given path is a directory.
func isDirectory(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// isExecutable checks if the file at the specified path is executable.
func isExecutable(filePath string) bool {
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && (info.Mode().Perm()&0o111) != 0
}

// stringToBinaryEntry parses a string in the format "binary", "binary#id", "binary#id:version" or "id"
func stringToBinaryEntry(input string) binaryEntry {
	var req binaryEntry

	// Split the input string by '#' to separate the name and the id/version part
	parts := strings.SplitN(input, "#", 2)
	req.Name = parts[0]

	if len(parts) > 1 {
		// Further split the id/version part by ':' to separate the id and version
		idVer := strings.SplitN(parts[1], ":", 2)
		req.PkgId = idVer[0]
		if len(idVer) > 1 {
			req.Version = idVer[1]
		}
	} else {
		// If there's no '#', assume the whole input is the name
		req.Name = input
	}

	fmt.Printf("Parsed binaryEntry: Name=%s, PkgId=%s, Version=%s\n", req.Name, req.PkgId, req.Version)
	return req
}

func arrStringToArrBinaryEntry(args []string) []binaryEntry {
	var entries []binaryEntry
	for _, arg := range args {
		entries = append(entries, stringToBinaryEntry(arg))
	}
	return entries
}

// parseBinaryEntry formats a single binaryEntry into a string in the format "name#id"
func parseBinaryEntry(entry binaryEntry, ansi bool) string {
	if ansi {
		return entry.Name + "\033[94m#" + entry.PkgId + "\033[0m"
	}
	return entry.Name + ternary(entry.PkgId != "", "#"+entry.PkgId, entry.PkgId)
}

// parseBinaryEntries formats a slice of binaryEntry into a slice of strings, each in the format "name#id" or "name#id:version"
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

// validateProgramsFrom checks the validity of programs against a remote source
func validateProgramsFrom(config *Config, programsToValidate []binaryEntry, metadata map[string]interface{}) ([]binaryEntry, error) {
	installDir := config.InstallDir
	programsEntries, err := listBinaries(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote binaries: %w", err)
	}
	remotePrograms := binaryEntriesToArrString(programsEntries, false)

	files, err := listFilesInDir(installDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list files in %s: %w", installDir, err)
	}

	programsToValidate = removeDuplicates(programsToValidate)
	validPrograms := make([]binaryEntry, 0, len(programsToValidate))

	validate := func(file string) (binaryEntry, bool) {
		fullBinaryName := listInstalled(file)
		if config.RetakeOwnership {
			fullBinaryName = filepath.Base(file)
			if fullBinaryName == "" {
				return binaryEntry{}, false
			}
		}
		if contains(remotePrograms, fullBinaryName) {
			return stringToBinaryEntry(fullBinaryName), true
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
			file := filepath.Join(installDir, program.Name)
			if bEntry, valid := validate(file); valid {
				validPrograms = append(validPrograms, bEntry)
			}
		}
	}

	return validPrograms, nil
}

func listInstalled(binaryPath string) string {
	if isSymlink(binaryPath) {
		return ""
	}
	fullBinaryName, err := getFullName(binaryPath)
	if err != nil || fullBinaryName == "" {
		return ""
	}
	return fullBinaryName
}

// errorEncoder generates a unique error code based on the sum of ASCII values of the error message.
func errorEncoder(format string, args ...interface{}) int {
	formattedErrorMessage := fmt.Sprintf(format, args...)

	var sum int
	for _, char := range formattedErrorMessage {
		sum += int(char)
	}
	errorCode := sum % 256
	fmt.Fprint(os.Stderr, formattedErrorMessage)
	return errorCode
}

// errorOut prints the error message to stderr and exits the program with the error code generated by errorEncoder.
func errorOut(format string, args ...interface{}) {
	os.Exit(errorEncoder(format, args...))
}

// getTerminalWidth attempts to determine the width of the terminal.
func getTerminalWidth() int {
	w, _, _ := term.GetSize(int(os.Stdout.Fd()))
	if w != 0 {
		return w
	}
	return 80
}

// truncateSprintf formats text and truncates to fit the screen's size, preserving escape sequences
func truncateSprintf(indicator, format string, a ...interface{}) string {
	text := fmt.Sprintf(format, a...)
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return text
	}

	width := getTerminalWidth() - len(indicator)
	if width <= 0 {
		return text
	}

	var out bytes.Buffer
	var visibleCount int
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

// truncatePrintf formats and prints text, and offers optional truncation
func truncatePrintf(disableTruncation bool, format string, a ...interface{}) (n int, err error) {
	if disableTruncation {
		return fmt.Printf(format, a...)
	}
	text := truncateSprintf(indicator, format, a...)
	return fmt.Print(text)
}

// listFilesInDir lists all files in a directory
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

// getFullName retrieves the full binary name from the extended attributes of the binary file.
func getFullName(binaryPath string) (string, error) {
	if !fileExists(binaryPath) {
		return filepath.Base(binaryPath), nil
	}

	fullName, err := xattr.Get(binaryPath, "user.FullName")
	if err != nil {
		return "", fmt.Errorf("full name attribute not found for binary: %s", binaryPath)
	}

	return string(fullName), nil
}

// addFullName writes the full binary name to the extended attributes of the binary file.
func addFullName(binaryPath string, pkgId string) error {
	if err := xattr.Set(binaryPath, "user.FullName", []byte(pkgId)); err != nil {
		return fmt.Errorf("failed to set xattr for %s: %w", binaryPath, err)
	}
	return nil
}

// removeNixGarbageFoundInTheRepos corrects any /nix/store/ or /bin/ binary path in the file.
func removeNixGarbageFoundInTheRepos(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", filePath, err)
	}

	nixShebangRegex := regexp.MustCompile(`^#!\s*/nix/store/[^/]+/`)
	nixBinPathRegex := regexp.MustCompile(`/nix/store/[^/]+/bin/`)

	lines := strings.Split(string(content), "\n")
	correctionsMade := false

	if len(lines) > 0 && nixShebangRegex.MatchString(lines[0]) {
		lines[0] = nixShebangRegex.ReplaceAllString(lines[0], "#!/")
		for i := 1; i < len(lines); i++ {
			if nixBinPathRegex.MatchString(lines[i]) {
				lines[i] = nixBinPathRegex.ReplaceAllString(lines[i], "")
			}
		}
		correctionsMade = true
	}

	if correctionsMade {
		if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			return fmt.Errorf("failed to correct nix object [%s]: %v", filepath.Base(filePath), err)
		}
		fmt.Printf("[%s] is a nix object. Corrections have been made.\n", filepath.Base(filePath))
	}
	return nil
}

func fetchJSON(url string, v interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("error creating request for %s: %v", url, err)
	}

	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Expires", "0")

	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error fetching from %s: %v", url, err)
	}
	defer response.Body.Close()

	var bodyReader io.Reader = response.Body
	if strings.HasSuffix(url, ".gz") {
		bodyReader, err = gzip.NewReader(response.Body)
		if err != nil {
			return fmt.Errorf("error creating gzip reader for %s: %v", url, err)
		}
		defer bodyReader.(*gzip.Reader).Close()
	}

	body := &bytes.Buffer{}
	if _, err := io.Copy(body, bodyReader); err != nil {
		return fmt.Errorf("error reading from %s: %v", url, err)
	}

	if err := json.Unmarshal(body.Bytes(), v); err != nil {
		return fmt.Errorf("error decoding from %s: %v", url, err)
	}

	return nil
}

// calculateChecksum calculates the checksum of a file
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

func sanitizeString(input string) string {
	var sanitized strings.Builder
	for _, ch := range input {
		if ch >= 32 && ch <= 126 {
			sanitized.WriteRune(ch)
		}
	}
	return sanitized.String()
}

// ternary function
func ternary[T any](cond bool, vtrue, vfalse T) T {
	if cond {
		return vtrue
	}
	return vfalse
}
