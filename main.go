package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/xplshn/a-utils/pkg/ccmd"
)

// Define Silent type
type Silent byte

const (
	unsupportedArchMsg               = "Unsupported architecture: "
	version                          = "1.7sid"
	indicator                        = "...>"
	maxCacheSize                     = 10
	binariesToDelete                 = 5
	normalVerbosity           Silent = iota + 1 // 1
	silentVerbosityWithErrors                   // 0
	disabledVerbosity                           // -1
)

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
	installDir := getEnvVar("DBIN_INSTALL_DIR", filepath.Join(homeDir, ".local/bin"))

	disableTruncationStr := getEnvVar("DBIN_DISABLETRUNCATION", "false")
	disableTruncation, err := strconv.ParseBool(disableTruncationStr)
	if err != nil {
		return "", "", "", nil, nil, false, fmt.Errorf("failed to parse DBIN_DISABLETRUNCATION: %v", err)
	}

	// The repo, its like this...
	determineArch := func() (string, error) {
		arch := runtime.GOARCH + "_" + runtime.GOOS
		switch arch {
		case "amd64_linux":
			return "x86_64_Linux", nil
		case "arm64_linux":
			return "aarch64_arm64_Linux", nil
		case "arm64_android":
			return "arm64_v8a_Android", nil
		default:
			return "", fmt.Errorf(unsupportedArchMsg + arch)
		}
	}

	getRepositories := func(arch string) []string {
		return []string{
			"https://bin.ajam.dev/" + arch + "/",
			"https://bin.ajam.dev/" + arch + "/Baseutils/",
		}
	}

	getMetadataURLs := func(arch string) []string {
		return []string{
			"https://raw.githubusercontent.com/xplshn/dbin/master/misc/cmd/modMetadata/Toolpacks.dbin_" + arch + ".json",
			"https://raw.githubusercontent.com/xplshn/dbin/master/misc/cmd/modMetadata/Baseutils.dbin_" + arch + ".json",
		}
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
		Name:        "dbin",
		Synopsis:    "<|--silent|> [|add/del [binaries]|update <binaries>|run [binary] <args>|info <binary>|list|search <searchTerm>]",
		Description: "The easy to use, easy to get, software distribution system",
	}

	helpPage, err := cmdInfo.GenerateHelpPage()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error generating help page:", err)
		os.Exit(1)
	}
	flag.Usage = func() {
		fmt.Print(helpPage)
	}

	// Parse the flags
	silent := flag.Bool("silent", false, "Run in silent mode")
	flag.Parse()

	var silentMode Silent
	if *silent {
		silentMode = silentVerbosityWithErrors
	} else {
		silentMode = normalVerbosity
	}

	args := flag.Args()

	if len(args) < 1 {
		fmt.Println("No command provided.")
		os.Exit(1)
	}

	command := args[0]
	args = args[1:]

	trackerFile, installDir, tempDir, repositories, metadataURLs, disableTruncation, err := setupEnvironment()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	switch command {
	case "findurl", "f":
		if len(args) < 1 {
			fmt.Println("No binary name provided for findurl command.")
			os.Exit(1)
		}
		binaryName := args[0]
		url, _ := findURL(binaryName, trackerFile, repositories, metadataURLs)
		fmt.Println(url)
	case "install", "add", "a":
		if len(args) < 1 {
			fmt.Println("No binary name provided for install command.")
			os.Exit(1)
		}
		binaries := args
		err := installCommand(binaries, installDir, trackerFile, silentMode, repositories, metadataURLs)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Installation completed successfully.")
	case "list", "l":
		if len(os.Args) == 3 {
			if os.Args[2] == "--described" || os.Args[2] == "-d" {
				// Call fSearch with an empty query and a large limit to list all described binaries
				fSearch(metadataURLs, installDir, "", disableTruncation, 99999)
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
	case "search", "s":
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
					fmt.Printf("Error: 'limit' value is not an int: %v\n", err)
					os.Exit(1)
				}
				queryIndex += 2
			} else {
				fmt.Println("Error: Missing 'limit' value.")
				os.Exit(1)
			}
		}

		if len(args) <= queryIndex {
			fmt.Println("Usage: dbin search <--limit||-l [int]> [query]")
			os.Exit(1)
		}

		query := args[queryIndex]
		err := fSearch(metadataURLs, installDir, query, disableTruncation, limit)
		if err != nil {
			fmt.Printf("Error searching binaries: %v\n", err)
			os.Exit(1)
		}
	case "info", "i":
		var binaryName string
		if len(args) > 0 {
			binaryName = args[0]
		}

		if binaryName == "" {
			installedPrograms, err := validateProgramsFrom(installDir, trackerFile, metadataURLs, nil)
			if err != nil {
				fmt.Printf("Error validating programs: %v\n", err)
				os.Exit(1)
			}
			for _, program := range installedPrograms {
				fmt.Println(program)
			}
		} else {
			binaryInfo, err := getBinaryInfo(trackerFile, binaryName, metadataURLs)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Name: %s\n", binaryInfo.Name)
			if binaryInfo.Description != "" {
				fmt.Printf("Description: %s\n", binaryInfo.Description)
			}
			if binaryInfo.Repo != "" {
				fmt.Printf("Repo: %s\n", binaryInfo.Repo)
			}
			if binaryInfo.Updated != "" {
				fmt.Printf("Updated: %s\n", binaryInfo.Updated)
			}
			if binaryInfo.Version != "" {
				fmt.Printf("Version: %s\n", binaryInfo.Version)
			}
			if binaryInfo.Size != "" {
				fmt.Printf("Size: %s\n", binaryInfo.Size)
			}
			if binaryInfo.Source != "" {
				fmt.Printf("Source: %s\n", binaryInfo.Source)
			}
			if binaryInfo.SHA256 != "" {
				fmt.Printf("SHA256: %s\n", binaryInfo.SHA256)
			}
		}
	case "run", "r":
		if len(args) < 1 {
			fmt.Println("Usage: dbin run <--silent, --transparent> [binary] <args>")
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
			fmt.Println("Usage: dbin run <--silent, --transparent> [binary] <args>")
			os.Exit(1)
		}

		binaryName := flag.Arg(0)
		runArgs := flag.Args()[1:]

		err := RunFromCache(binaryName, runArgs, tempDir, trackerFile, transparentMode, silentMode, repositories, metadataURLs)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "update", "u":
		var programsToUpdate []string
		if len(os.Args) > 2 {
			programsToUpdate = os.Args[2:]
		}
		if err := update(programsToUpdate, installDir, trackerFile, silentMode, repositories, metadataURLs); err != nil {
			fmt.Println("Update failed:", err)
		}
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}
