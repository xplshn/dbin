package main

import (
	"context"
	"fmt"
	"github.com/goccy/go-json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ansiColor   = "\033[36m"
	ansiNoColor = "\033[0m"
)

// Spinner struct with thread-safe mechanisms
type Spinner struct {
	frames   []rune
	index    int
	mu       sync.Mutex
	stopChan chan struct{}
	speed    time.Duration
	speedMu  sync.Mutex
}

// NewSpinner creates a new Spinner instance
func NewSpinner(frames []rune) *Spinner {
	return &Spinner{
		frames:   frames,
		index:    0,
		stopChan: make(chan struct{}),
		speed:    80 * time.Millisecond, // default interval
	}
}

// Start begins the spinner in a new goroutine
func (s *Spinner) Start() {
	go func() {
		for {
			select {
			case <-s.stopChan:
				return
			default:
				s.mu.Lock()
				fmt.Printf("%s%c%s\r", ansiColor, s.frames[s.index], ansiNoColor)
				s.index = (s.index + 1) % len(s.frames)
				s.mu.Unlock()
				time.Sleep(s.getSpeed())
			}
		}
	}()
}

// Stop stops the spinner
func (s *Spinner) Stop() {
	close(s.stopChan)
}

// SetSpeed sets the spinner speed
func (s *Spinner) SetSpeed(speed time.Duration) {
	s.speedMu.Lock()
	s.speed = speed
	s.speedMu.Unlock()
}

// getSpeed gets the spinner speed
func (s *Spinner) getSpeed() time.Duration {
	s.speedMu.Lock()
	defer s.speedMu.Unlock()
	return s.speed
}

// fetchBinaryFromURLToDest downloads the file from the given URL to the specified destination
func fetchBinaryFromURLToDest(ctx context.Context, url string, destination string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Ensure the parent directory exists
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return "", fmt.Errorf("failed to create parent directories for %s: %v", destination, err)
	}

	tempFile := destination + ".tmp"
	out, err := os.Create(tempFile)
	if err != nil {
		return "", err
	}
	defer out.Close()

	spinner := NewSpinner([]rune{
		'⠁', '⠂', '⠄', '⡀', '⡈', '⡐', '⡠', '⣀',
		'⣁', '⣂', '⣄', '⣌', '⣔', '⣤', '⣥', '⣦',
		'⣮', '⣶', '⣷', '⣿', '⡿', '⠿', '⢟', '⠟',
		'⡛', '⠛', '⠫', '⢋', '⠋', '⠍', '⡉', '⠉',
		'⠑', '⠡', '⢁', // '🕛', '🕧', '🕐', '🕜', '🕑', '🕝', '🕒', '🕞', '🕓', '🕟', '🕔', '🕠', '🕕', '🕡', '🕖', '🕢', '🕗', '🕣', '🕘', '🕤', '🕙', '🕙', '🕚', '🕦',
	})
	spinner.Start()
	defer spinner.Stop()

	var downloaded int64
	buf := make([]byte, 4096)
	startTime := time.Now()

	// Label to break out of the loop
downloadLoop:
	for {
		select {
		case <-ctx.Done():
			_ = os.Remove(tempFile)
			return "", ctx.Err()
		default:
			n, err := resp.Body.Read(buf)
			if n > 0 {
				if _, err := out.Write(buf[:n]); err != nil {
					_ = os.Remove(tempFile)
					return "", err
				}
				downloaded += int64(n)
				elapsed := time.Since(startTime).Seconds()
				speed := float64(downloaded) / elapsed // bytes per second

				// Adjust spinner speed based on download speed
				switch {
				case speed > 1024*1024: // more than 1MB/s
					spinner.SetSpeed(150 * time.Millisecond)
				case speed > 512*1024: // more than 512KB/s
					spinner.SetSpeed(100 * time.Millisecond)
				case speed > 256*1024: // more than 256KB/s
					spinner.SetSpeed(80 * time.Millisecond)
				default:
					spinner.SetSpeed(50 * time.Millisecond)
				}
			}
			if err == io.EOF {
				break downloadLoop
			}
			if err != nil {
				_ = os.Remove(tempFile)
				return "", err
			}
		}
	}

	// Modify the shebang if needed
	if err := removeNixGarbageFoundInTheRepos(tempFile); err != nil {
		_ = os.Remove(tempFile)
		return "", err
	}

	if err := os.Rename(tempFile, destination); err != nil {
		_ = os.Remove(tempFile)
		return "", err
	}

	if err := os.Chmod(destination, 0755); err != nil {
		_ = os.Remove(destination)
		return "", fmt.Errorf("failed to set executable bit for %s: %v", destination, err)
	}

	return destination, nil
}

func fetchJSON(url string, v interface{}) error {
	spinner := NewSpinner([]rune{
		'⠁', '⠂', '⠄', '⡀', '⡈', '⡐', '⡠', '⣀',
		'⣁', '⣂', '⣄', '⣌', '⣔', '⣤', '⣥', '⣦',
		'⣮', '⣶', '⣷', '⣿', '⡿', '⠿', '⢟', '⠟',
		'⡛', '⠛', '⠫', '⢋', '⠋', '⠍', '⡉', '⠉',
		'⠑', '⠡', '⢁', // '🕛', '🕧', '🕐', '🕜', '🕑', '🕝', '🕒', '🕞', '🕓', '🕟', '🕔', '🕠', '🕕', '🕡', '🕖', '🕢', '🕗', '🕣', '🕘', '🕤', '🕙', '🕙', '🕚', '🕦',
	})
	spinner.Start()
	defer spinner.Stop()

	response, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("error fetching from %s: %v", url, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("error reading from %s: %v", url, err)
	}

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("error decoding from %s: %v", url, err)
	}

	return nil
}

// contanins will return true if the provided slice of []strings contains the word str
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

// isExecutable checks if the file at the specified path is executable.
func isExecutable(filePath string) bool {
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && (info.Mode().Perm()&0o111) != 0
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

// validateProgramsFrom checks the validity of programs against a remote source
func validateProgramsFrom(installDir, trackerFile string, metadataURLs, programsToValidate []string) ([]string, error) {
	// Fetch the list of binaries from the remote source
	remotePrograms, err := listBinaries(metadataURLs)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote binaries: %w", err)
	}

	// List files from the specified directory
	files, err := listFilesInDir(installDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list files in %s: %w", installDir, err)
	}

	validPrograms := []string{}
	invalidPrograms := []string{}

	// Remove duplicate entries from the programs list
	programsToValidate = removeDuplicates(programsToValidate)

	// Function to get the program name for validation
	getProgramNameForValidation := func(binaryName string) (string, error) {
		realBinaryName, err := getBinaryNameFromTrackerFile(trackerFile, binaryName)
		if err != nil {
			return "", err
		}
		return realBinaryName, nil
	}

	// Inline function to validate a program name
	validateProgram := func(program string) (bool, string) {
		realBinaryName, _ := getProgramNameForValidation(program)
		if realBinaryName == "" {
			realBinaryName = program
		}
		if contains(remotePrograms, realBinaryName) {
			return true, realBinaryName
		}
		return false, realBinaryName
	}

	if len(programsToValidate) == 0 {
		// Validate all files in the directory
		for _, file := range files {
			fileName := filepath.Base(file)
			isValid, binaryName := validateProgram(fileName)
			if isValid {
				validPrograms = append(validPrograms, binaryName)
			} else {
				invalidPrograms = append(invalidPrograms, binaryName)
			}
		}
	} else {
		// Validate only the specified programs
		for _, program := range programsToValidate {
			isValid, binaryName := validateProgram(program)
			if isValid {
				validPrograms = append(validPrograms, binaryName)
			} else {
				invalidPrograms = append(invalidPrograms, binaryName)
			}
		}
	}

	return validPrograms, nil
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

// GetTerminalWidth attempts to determine the width of the terminal.
// It first tries using "stty size", then "tput cols", and finally falls back to  80 columns.
func getTerminalWidth() int {
	// Try using stty size
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err == nil {
		// stty size returns rows and columns
		parts := strings.Split(strings.TrimSpace(string(out)), " ")
		if len(parts) == 2 {
			width, _ := strconv.Atoi(parts[1])
			return width
		}
	}

	// Fallback to tput cols
	cmd = exec.Command("tput", "cols")
	cmd.Stdin = os.Stdin
	out, err = cmd.Output()
	if err == nil {
		width, _ := strconv.Atoi(strings.TrimSpace(string(out)))
		return width
	}

	// Fallback to  80 columns
	return 80
}

// NOTE: \n will always get cut off when using a truncate function, this may also happen to other formatting options
// truncateSprintf formats the string and truncates it if it exceeds the terminal width.
func truncateSprintf(indicator, format string, a ...interface{}) string {
	// Format the string first
	formatted := fmt.Sprintf(format, a...)

	// Determine the truncation length & truncate the formatted string if it exceeds the available space
	availableSpace := getTerminalWidth() - len(indicator)
	if len(formatted) > availableSpace {
		formatted = formatted[:availableSpace]
		for strings.HasSuffix(formatted, ",") || strings.HasSuffix(formatted, ".") || strings.HasSuffix(formatted, " ") {
			formatted = formatted[:len(formatted)-1]
		}
		formatted = fmt.Sprintf("%s%s", formatted, indicator) // Add the dots.
	}

	return formatted
}

// truncatePrintf is a drop-in replacement for fmt.Printf that truncates the input string if it exceeds a certain length.
func truncatePrintf(disableTruncation, addNewLine bool, format string, a ...interface{}) (n int, err error) {
	if disableTruncation {
		return fmt.Printf(format, a...)
	}
	if addNewLine {
		return fmt.Println(truncateSprintf(indicator, format, a...))
	}
	return fmt.Print(truncateSprintf(indicator, format, a...))
}

// addToTrackerFile appends a binary name to the tracker file.
func addToTrackerFile(trackerFile, binaryName, installDir string) error {
	tracker, err := readTrackerFile(trackerFile)
	if err != nil {
		return err
	}

	baseName := filepath.Base(binaryName)
	if _, exists := tracker[baseName]; !exists {
		tracker[baseName] = binaryName
		err := writeTrackerFile(trackerFile, tracker)
		if err != nil {
			return fmt.Errorf("could not write to tracker file: %w", err)
		}
	}

	cleanupTrackerFile(trackerFile, installDir)
	return nil
}

// getBinaryNameFromTrackerFile retrieves the full binary name from the tracker file based on the base name.
func getBinaryNameFromTrackerFile(trackerFile, baseName string) (string, error) {
	baseName = filepath.Base(baseName)

	tracker, err := readTrackerFile(trackerFile)
	if err != nil {
		return "", fmt.Errorf("could not read tracker file: %w", err)
	}

	if binaryName, exists := tracker[baseName]; exists {
		return binaryName, nil
	}

	return "", fmt.Errorf("no match found for %s in tracker file", baseName)
}

// cleanupTrackerFile removes entries for binaries no longer present in the install directory.
func cleanupTrackerFile(trackerFile, installDir string) error {
	tracker, err := readTrackerFile(trackerFile)
	if err != nil {
		return err
	}

	newTracker := make(map[string]string)
	for baseName, repoPath := range tracker {
		expectedPath := filepath.Join(installDir, baseName)
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			// If the file does not exist in the installDir, skip adding it to newTracker
			continue
		}
		// Keep the entry if the file exists
		newTracker[baseName] = repoPath
	}

	err = writeTrackerFile(trackerFile, newTracker)
	if err != nil {
		return fmt.Errorf("could not write to tracker file: %w", err)
	}

	return nil
}

// readTrackerFile reads the tracker file and returns the tracker map.
func readTrackerFile(trackerFile string) (map[string]string, error) {
	file, err := os.Open(trackerFile)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil // If the file doesn't exist, return an empty map
		}
		return nil, fmt.Errorf("could not open tracker file: %w", err)
	}
	defer file.Close()

	tracker := make(map[string]string)
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&tracker); err != nil {
		return nil, fmt.Errorf("could not decode tracker file: %w", err)
	}

	return tracker, nil
}

// writeTrackerFile writes the tracker map to the tracker file.
func writeTrackerFile(trackerFile string, tracker map[string]string) error {
	file, err := os.OpenFile(trackerFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("could not open tracker file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(tracker); err != nil {
		return fmt.Errorf("could not encode tracker file: %w", err)
	}

	return nil
}

// removeNixGarbageFoundInTheRepos corrects any /nix/store/ or /bin/ binary path in the file.
func removeNixGarbageFoundInTheRepos(filePath string) error {
	// Read the entire file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", filePath, err)
	}

	// Regex to match and remove the /nix/store/*/ prefix in the shebang line, preserving the rest of the path
	nixShebangRegex := regexp.MustCompile(`^#!\s*/nix/store/[^/]+/`)
	// Regex to match and remove the /nix/store/*/bin/ prefix in other lines
	nixBinPathRegex := regexp.MustCompile(`/nix/store/[^/]+/bin/`)

	// Split content by lines
	lines := strings.Split(string(content), "\n")

	// Flag to track if any corrections were made
	correctionsMade := false

	// Handle the shebang line separately if it exists and matches the nix pattern
	if len(lines) > 0 && nixShebangRegex.MatchString(lines[0]) {
		lines[0] = nixShebangRegex.ReplaceAllString(lines[0], "#!/")
		// Iterate through the rest of the lines and correct any /nix/store/*/bin/ path
		for i := 1; i < len(lines); i++ {
			if nixBinPathRegex.MatchString(lines[i]) {
				lines[i] = nixBinPathRegex.ReplaceAllString(lines[i], "")
			}
		}
		correctionsMade = true
	}

	// If any corrections were made, write the modified content back to the file
	if correctionsMade {
		if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			return fmt.Errorf("failed to correct nix object [%s]: %v", filepath.Base(filePath), err)
		}
		fmt.Printf("[%s] is a nix object. Corrections have been made.\n", filepath.Base(filePath))
	}

	return nil
}
