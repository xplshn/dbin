// Copyright (c) 2024-2024 xplshn                       [3BSD]
// For more details refer to https://github.com/xplshn/a-utils
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xplshn/a-utils/pkg/ccmd"
)

// removeNixGarbageFromFile corrects any /nix/store/ or /bin/ binary path in the provided file content.
func removeNixGarbageFromFile(content string) (string, bool) {
	// Regex to match and remove the /nix/store/.../ prefix in the shebang line, preserving the rest of the path
	nixShebangRegex := regexp.MustCompile(`^#!\s*/nix/store/[^/]+/`)
	// Regex to match and remove the /nix/store/*/bin/ prefix in other lines
	nixBinPathRegex := regexp.MustCompile(`/nix/store/[^/]+/bin/`)
	// Regex to match and replace /nix/store/*/bin with /bin in variable assignments
	nixBinVarRegex := regexp.MustCompile(`("[^"]*)/nix/store/[^/]+/bin("[^"]*)`)

	// Split content by lines
	lines := strings.Split(content, "\n")
	correctionsMade := false

	// Handle the shebang line separately if it exists and matches the nix pattern
	if len(lines) > 0 && nixShebangRegex.MatchString(lines[0]) {
		lines[0] = nixShebangRegex.ReplaceAllString(lines[0], "#!/")
		correctionsMade = true
	}

	// Iterate through the rest of the lines and correct any /nix/store/*/bin/ path
	for i := 1; i < len(lines); i++ {
		if nixBinPathRegex.MatchString(lines[i]) {
			lines[i] = nixBinPathRegex.ReplaceAllString(lines[i], "")
			correctionsMade = true
		}
		if nixBinVarRegex.MatchString(lines[i]) {
			lines[i] = nixBinVarRegex.ReplaceAllString(lines[i], "/bin")
			correctionsMade = true
		}
	}

	return strings.Join(lines, "\n"), correctionsMade
}

// processFile reads the input file, processes it, writes to the output, and makes the output file executable.
func processFile(inputFile, outputFile string) error {
	content, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", inputFile, err)
	}

	modifiedContent, correctionsMade := removeNixGarbageFromFile(string(content))

	// If corrections were made, write to the output file
	if correctionsMade {
		if err := os.WriteFile(outputFile, []byte(modifiedContent), 0644); err != nil {
			return fmt.Errorf("failed to write to output file %s: %v", outputFile, err)
		}
		fmt.Printf("[%s] corrections have been made.\n", filepath.Base(inputFile))
	} else {
		fmt.Printf("[%s] no corrections were necessary.\n", filepath.Base(inputFile))
	}

	// Set the output file as executable (chmod +x)
	if err := os.Chmod(outputFile, 0755); err != nil {
		return fmt.Errorf("failed to set executable permissions on file %s: %v", outputFile, err)
	}
	fmt.Printf("[%s] is now executable.\n", filepath.Base(outputFile))

	return nil
}

func main() {
	cmdInfo := &ccmd.CmdInfo{
		Authors:     []string{"xplshn"},
		Repository:  "https://github.com/xplshn/a-utils",
		Name:        "nix-garbage-remover",
		Synopsis:    "--input-file <path> --output-to <path>",
		Description: "Removes Nix garbage from specified file and writes corrected output.",
	}

	helpPage, err := cmdInfo.GenerateHelpPage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating help page: %v\n", err)
		return
	}

	// Define command-line flags
	inputFile := flag.String("input-file", "", "Path to the input file to process (required)")
	outputFile := flag.String("output-to", "", "Path to write the output file (required)")

	flag.Usage = func() { fmt.Print(helpPage) }

	flag.Parse()

	// Ensure both input and output file paths are provided
	if *inputFile == "" || *outputFile == "" {
		fmt.Fprintf(os.Stderr, "Both --input-file and --output-to must be provided.\n")
		flag.Usage()
		os.Exit(1)
	}

	// Process the file
	if err := processFile(*inputFile, *outputFile); err != nil {
		fmt.Fprintln(os.Stderr, "nix-garbage-remover failed:", err)
		os.Exit(1)
	}
}
