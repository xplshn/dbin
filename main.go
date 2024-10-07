//usr/bin/env go run findURL.go fsearch.go info.go install.go listBinaries.go main.go remove.go run.go update.go utility.go utility_progressbar.go "$@"; exit $?
// dbin - 📦 Poor man's package manager. The easy to use, easy to get, suckless software distribution system
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/xplshn/a-utils/pkg/ccmd"
)

// Verbosity is used along with >= and <= to determine which messages to hide when using `--silent` and which messages to display when using `--verbose`
type Verbosity int8

const (
	unsupportedArchMsg                  = "Unsupported architecture: "
	version                             = "0.5.1"
	indicator                           = "...>"
	maxCacheSize                        = 10
	binariesToDelete                    = 5
	normalVerbosity           Verbosity = 1
	extraVerbose              Verbosity = 2
	silentVerbosityWithErrors Verbosity = -1
	extraSilent               Verbosity = -2
)

// parseColonSeparatedEnv splits a colon-separated string into a slice. If the environment variable is not set or is empty, it returns the provided default slice.
func parseColonSeparatedEnv(envVar string, defaultValue []string) []string {
	envValue := os.Getenv(envVar)
	if envValue == "" {
		return defaultValue
	}
	return strings.Split(envValue, ";")
}

func getEnvVar(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// setupEnvironment initializes the environment settings including architecture, repositories, and metadata URLs.
func setupEnvironment() (string, string, string, []string, []string, bool, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", nil, nil, false, fmt.Errorf("failed to get user's Home directory: %v", err)
	}

	tempDir, err := os.UserCacheDir()
	if err != nil {
		return "", "", "", nil, nil, false, fmt.Errorf("failed to get user's Cache directory: %v", err)
	}
	confDir, err := os.UserConfigDir()
	if err != nil {
		return "", "", "", nil, nil, false, fmt.Errorf("failed to get user's XDG Config directory: %v", err)
	}

	tempDir = getEnvVar("DBIN_CACHEDIR", filepath.Join(tempDir, "dbin_cache"))
	trackerFile := getEnvVar("DBIN_TRACKERFILE", filepath.Join(confDir, "dbin.tracker.json"))

	// DBIN_INSTALL_DIR or XDG_BIN_HOME. "$HOME/.local/bin" as fallback
	installDir := os.Getenv("DBIN_INSTALL_DIR")
	if installDir == "" {
		installDir = os.Getenv("XDG_BIN_HOME")
		if installDir == "" {
			// If neither are set, default to "$HOME/.local/bin"
			installDir = filepath.Join(homeDir, ".local/bin")
		}
	}

	disableTruncationStr := getEnvVar("DBIN_NOTRUNCATION", "false")
	disableTruncation, err := strconv.ParseBool(disableTruncationStr)
	if err != nil {
		return "", "", "", nil, nil, false, fmt.Errorf("failed to parse DBIN_NOTRUNCATION: %v", err)
	}

	determineArch := func() (string, error) {
		arch := runtime.GOARCH + "_" + runtime.GOOS

		if arch != "amd64_linux" && arch != "arm64_linux" && arch != "arm64_android" {
			return "", fmt.Errorf(unsupportedArchMsg + arch)
		}

		return arch, nil
	}

	getRepositories := func(arch string) []string {
		// Default repository URLs
		defaultRepos := []string{
			"https://bin.ajam.dev/" + arch + "/",
			"https://bin.ajam.dev/" + arch + "/Baseutils/",
		}

		// Parse DBIN_REPO_URLS environment variable or return defaults
		return parseColonSeparatedEnv("DBIN_REPO_URLS", defaultRepos)
	}

	getMetadataURLs := func(arch string) []string {
		// Default metadata URLs
		defaultMetadataURLs := []string{
			//	"https://raw.githubusercontent.com/xplshn/dbin-metadata/master/misc/cmd/modMetadata/Toolpacks.dbin_" + arch + ".json",
			//	"https://raw.githubusercontent.com/xplshn/dbin-metadata/master/misc/cmd/modMetadata/Baseutils.dbin_" + arch + ".json",
			//	"https://raw.githubusercontent.com/xplshn/dbin-metadata/master/misc/cmd/modMetadata/Toolpacks-extras.dbin_" + arch + ".json",
			"https://github.com/xplshn/dbin-metadata/raw/refs/heads/master/misc/cmd/modMetadataAIO/unifiedAIO_" + arch + ".dbin.min.json",
		}

		// Parse DBIN_METADATA_URLS environment variable or return defaults
		return parseColonSeparatedEnv("DBIN_METADATA_URLS", defaultMetadataURLs)
	}

	arch, err := determineArch()
	if err != nil {
		return "", "", "", nil, nil, false, err
	}

	repositories := getRepositories(arch)
	metadataURLs := getMetadataURLs(arch)

	return trackerFile, installDir, tempDir, repositories, metadataURLs, disableTruncation, nil
}

func main() {
	cmdInfo := &ccmd.CmdInfo{
		Authors:     []string{"xplshn"},
		Repository:  "https://github.com/xplshn/dbin",
		Name:        "dbin",
		Synopsis:    "[-v|-h] [list|install|remove|update|run|info|search|tldr] <-args->",
		Description: "The easy to use, easy to get, software distribution system",
		CustomFields: map[string]interface{}{
			"1_Options": `-h, --help        Show this help message
-v, --version     Show the version number`,
			"2_Commands": `list              List all available binaries
install, add      Install a binary
remove, del       Remove a binary
update            Update binaries, by checking their SHA against the repo's SHA
run               Run a specified binary from cache
info              Show information about a specific binary OR display installed binaries
search            Search for a binary - (not all binaries have metadata. Use list to see all binaries)
tldr              Equivalent to "run --transparent --silent tlrc"`,
			"3_Variables": `DBIN_CACHEDIR     If present, it must contain a valid directory path
DBIN_INSTALL_DIR   If present, it must contain a valid directory path
DBIN_NOTRUNCATION  If present, and set to ONE (1), string truncation will be disabled
DBIN_TRACKERFILE   If present, it must point to a valid file path, in an existing directory
DBIN_REPO_URLS     If present, it must contain one or more repository URLS ended in / separated by ;
DBIN_METADATA_URLS If present, it must contain one or more repository's metadata url separated by ;`,
			"4_Examples": `dbin search editor
dbin install micro.upx
dbin install lux kakoune aretext shfmt
dbin --silent install bed && echo "[bed] was installed to $INSTALL_DIR/bed"
dbin del bed
dbin del orbiton tgpt lux
dbin info
dbin info | grep a-utils | xargs dbin add # install the entire a-utils suite
dbin info jq
dbin list --described
dbin tldr gum
dbin --verbose run curl -qsfSL "https://raw.githubusercontent.com/xplshn/dbin/master/stubdl" | sh -
dbin --silent run elinks -no-home "https://fatbuffalo.neocities.org/def"
dbin --silent run --transparent micro ~/.profile
dbin run btop`,
		},
	}

	helpPage, err := cmdInfo.GenerateHelpPage()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error generating help page:", err)
		os.Exit(1)
	}
	flag.Usage = func() {
		print(helpPage)
	}

	// Define and parse flags
	verboseFlag := flag.Bool("verbose", false, "Run in extra verbose mode")
	silentFlag := flag.Bool("silent", false, "Run in silent mode, only errors will be shown")
	extraSilentFlag := flag.Bool("extra-silent", false, "Run in extra silent mode, suppressing almost all output")
	versionFlag := flag.Bool("v", false, "Show the version number")
	longVersionFlag := flag.Bool("version", false, "Show the version number")
	flag.Parse()

	if *versionFlag || *longVersionFlag {
		fmt.Println(helpPage, "dbin version", version)
		os.Exit(1)
	}

	// Check for conflicting flags
	if (*verboseFlag && *silentFlag) || (*verboseFlag && *extraSilentFlag) || (*silentFlag && *extraSilentFlag) {
		fmt.Fprintln(os.Stderr, "error: Conflicting verbose flags provided.")
		os.Exit(1)
	}

	// determineVerbosity determines the verbosity level based on the flags provided.
	determineVerbosity := func(
		silentFlag, verboseFlag, extraSilentFlag bool,
		normalVerbosity, extraVerbose, silentVerbosityWithErrors, extraSilent Verbosity,
	) Verbosity {
		switch {
		case extraSilentFlag:
			return extraSilent
		case silentFlag:
			return silentVerbosityWithErrors
		case verboseFlag:
			return extraVerbose
		default:
			return normalVerbosity
		}
	}

	// Determine verbosity level
	verbosityLevel := determineVerbosity(
		*silentFlag,
		*verboseFlag,
		*extraSilentFlag,
		normalVerbosity,
		extraVerbose,
		silentVerbosityWithErrors,
		extraSilent,
	)

	args := flag.Args()

	if len(args) < 1 {
		print(helpPage)
		os.Exit(1)
	}

	command := args[0]
	args = args[1:]

	trackerFile, installDir, tempDir, repositories, metadataURLs, disableTruncation, err := setupEnvironment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	runCommandTrackerFile := filepath.Join(tempDir, "dbin.cached.tracker.json")

	switch command {
	case "findurl":
		if len(args) < 1 {
			fmt.Println("No binary names provided for findurl command.")
			os.Exit(1)
		}
		binaryNames := args
		urls, _, err := findURL(binaryNames, trackerFile, repositories, metadataURLs, verbosityLevel)
		if err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "%v", err)
			}
			os.Exit(1)
		}
		if verbosityLevel >= normalVerbosity {
			for i, url := range urls {
				fmt.Printf("URL for %s: %s\n", binaryNames[i], url)
			}
		}
	case "install", "add":
		if len(args) < 1 {
			fmt.Println("No binary name provided for install command.")
			os.Exit(1)
		}
		binaries := args
		err := installCommand(binaries, installDir, trackerFile, verbosityLevel, repositories, metadataURLs)
		if err != nil {
			fmt.Printf("%v\n", err)
			os.Exit(1)
		}
		// fmt.Println("Installation completed successfully.")
	case "remove", "del":
		if len(args) < 1 {
			fmt.Println("No binary name provided for remove command.")
			os.Exit(1)
		}
		binaries := args
		err := removeCommand(binaries, installDir, trackerFile, verbosityLevel)
		if err != nil {
			fmt.Printf("%v\n", err)
			os.Exit(1)
		}
		// fmt.Println("Removal completed successfully.")
	case "list":
		if len(os.Args) == 3 {
			if os.Args[2] == "--described" || os.Args[2] == "-d" {
				// Call fSearch with an empty query and a large limit to list all described binaries
				fSearch(metadataURLs, installDir, tempDir, "", disableTruncation, 99999, runCommandTrackerFile)
			} else {
				errorOut("dbin: Unknown command.\n")
			}
		} else {
			binaries, err := listBinaries(metadataURLs)
			if err != nil {
				fmt.Println("Error listing binaries:", err)
				os.Exit(1)
			}
			for _, binary := range binaries {
				fmt.Println(binary)
			}
		}
	case "search":
		limit := 90
		queryIndex := 0

		if len(args) < queryIndex+1 {
			fmt.Println("Usage: dbin search <--limit||-l [int]> [query]")
			os.Exit(1)
		}

		if len(args) > 0 && (args[queryIndex] == "--limit" || args[queryIndex] == "-l") {
			if len(args) > queryIndex+1 {
				var err error
				limit, err = strconv.Atoi(args[queryIndex+1])
				if err != nil {
					fmt.Printf("error: 'limit' value is not an int: %v\n", err)
					os.Exit(1)
				}
				queryIndex += 2
			} else {
				fmt.Println("error: Missing 'limit' value.")
				os.Exit(1)
			}
		}

		if len(args) <= queryIndex {
			fmt.Println("Usage: dbin search <--limit||-l [int]> [query]")
			os.Exit(1)
		}

		query := args[queryIndex]
		err := fSearch(metadataURLs, installDir, tempDir, query, disableTruncation, limit, runCommandTrackerFile)
		if err != nil {
			fmt.Printf("error searching binaries: %v\n", err)
			os.Exit(1)
		}
	case "info":
		var binaryName string
		if len(args) > 0 {
			binaryName = args[0]
		}

		if binaryName == "" {
			installedPrograms, err := validateProgramsFrom(installDir, trackerFile, metadataURLs, nil)
			if err != nil {
				fmt.Printf("error validating programs: %v\n", err)
				os.Exit(1)
			}
			for _, program := range installedPrograms {
				fmt.Println(program)
			}
		} else {
			binaryInfo, err := getBinaryInfo(trackerFile, binaryName, metadataURLs)
			if err != nil {
				fmt.Printf("%v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Name: %s\n", binaryInfo.Name)
			if binaryInfo.Description != "" {
				fmt.Printf("Description: %s\n", binaryInfo.Description)
			}
			if binaryInfo.Note != "" {
				fmt.Printf("Note: %s\n", binaryInfo.Note)
			}
			if binaryInfo.Version != "" {
				fmt.Printf("Version: %s\n", binaryInfo.Version)
			}
			if binaryInfo.DownloadURL != "" {
				fmt.Printf("Download URL: %s\n", binaryInfo.DownloadURL)
			}
			if binaryInfo.Size != "" {
				fmt.Printf("Size: %s\n", binaryInfo.Size)
			}
			if binaryInfo.Bsum != "" {
				fmt.Printf("B3SUM: %s\n", binaryInfo.Bsum)
			}
			if binaryInfo.Shasum != "" {
				fmt.Printf("SHA256: %s\n", binaryInfo.Shasum)
			}
			if binaryInfo.BuildDate != "" {
				fmt.Printf("Build Date: %s\n", binaryInfo.BuildDate)
			}
			if binaryInfo.SrcURL != "" {
				fmt.Printf("Source URL: %s\n", binaryInfo.SrcURL)
			}
			if binaryInfo.WebURL != "" {
				fmt.Printf("Web URL: %s\n", binaryInfo.WebURL)
			}
			if binaryInfo.BuildScript != "" {
				fmt.Printf("Build Script: %s\n", binaryInfo.BuildScript)
			}
			if binaryInfo.BuildLog != "" {
				fmt.Printf("Build Log: %s\n", binaryInfo.BuildLog)
			}
			if binaryInfo.Category != "" {
				fmt.Printf("Category: %s\n", binaryInfo.Category)
			}
			if binaryInfo.ExtraBins != "" {
				fmt.Printf("Extra Bins: %s\n", binaryInfo.ExtraBins)
			}
		}
	case "run":
		if len(args) < 1 {
			fmt.Println("Usage: dbin run <--transparent> [binary] <args>")
			os.Exit(1)
		}

		var transparentMode bool
		transparent := flag.Bool("transparent", false, "Run the binary from PATH if found")
		flag.CommandLine.Parse(args)

		if *transparent {
			transparentMode = true
		}

		// Ensure binary name is provided
		if len(flag.Args()) < 1 {
			fmt.Println("Usage: dbin run <--transparent> [binary] <args>")
			os.Exit(1)
		}

		RunFromCache(flag.Arg(0), flag.Args()[1:], tempDir, runCommandTrackerFile, transparentMode, verbosityLevel, repositories, metadataURLs)
	case "tldr":
		RunFromCache("tlrc", flag.Args()[1:], tempDir, runCommandTrackerFile, true, verbosityLevel, repositories, metadataURLs)
	case "update":
		var programsToUpdate []string
		if len(os.Args) > 2 {
			programsToUpdate = os.Args[2:]
		}
		if err := update(programsToUpdate, installDir, trackerFile, verbosityLevel, repositories, metadataURLs); err != nil {
			fmt.Println("Update failed:", err)
		}
	default:
		print(helpPage)
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		os.Exit(1)
	}
}
