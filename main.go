// usr/bin/env go run findURL.go fsearch.go info.go install.go listBinaries.go main.go remove.go run.go update.go utility.go fetch.go config.go "$@"; exit $?
// dbin - 📦 Poor man's package manager. The easy to use, easy to get, suckless software distribution system
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/xplshn/a-utils/pkg/ccmd"
)

// Verbosity is used along with >= and <= to determine which messages to hide when using `--silent` and which messages to display when using `--verbose`
type Verbosity int8

const (
	unsupportedArchMsg                  = "Unsupported architecture: "
	indicator                           = "...>"
	Version                             = "0.7"
	maxCacheSize                        = 10
	binariesToDelete                    = 5
	normalVerbosity           Verbosity = 1
	extraVerbose              Verbosity = 2
	silentVerbosityWithErrors Verbosity = -1
	extraSilent               Verbosity = -2
)

func main() {
	cmdInfo := &ccmd.CmdInfo{
		Authors:     []string{"xplshn"},
		Repository:  "https://github.com/xplshn/dbin",
		Name:        "dbin",
		Synopsis:    "[-v|-h] [list|install|remove|update|run|info|search|tldr|eget2] <-args->",
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
tldr              Equivalent to "--silent run --transparent tlrc"
eget2             Equivalent to "--silent run --transparent eget2"`,
			"3_Variables": `DBIN_CACHEDIR      If present, it must contain a valid directory path
DBIN_INSTALL_DIR   If present, it must contain a valid directory path
DBIN_NOTRUNCATION  If present, and set to ONE (1), string truncation will be disabled
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
		fmt.Println(helpPage, "dbin version", Version)
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

	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// function to fetch the metadata ONCE
	var metadata map[string]interface{}
	var alreadyFetched bool
	fetchMetadata := func() map[string]interface{} {
		if alreadyFetched != true {
			for _, url := range config.MetadataURLs {
				err := fetchJSON(url, &metadata)
				if err != nil {
					fmt.Printf("failed to fetch and decode binary information from %s: %v\n", url, err)
					continue
				}
			}
			alreadyFetched = true
		} else {
			fmt.Println("fetchMetadata was re-triggered.")
		}
		return metadata
	}

	switch command {
	case "findurl":
		if len(args) < 1 {
			fmt.Println("No binary names provided for findurl command.")
			os.Exit(1)
		}
		binaryNames := args
		fetchMetadata()
		urls, _, err := findURL(config, binaryNames, verbosityLevel, metadata)
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
	case "fullname":
		if len(args) < 1 {
			fmt.Println("No binary name was provided for fullname")
			os.Exit(1)
		}
		fullName, _ := getFullName(args[0])
		fmt.Println("fullName of", args[0], "is", fullName)
	case "install", "add":
		if len(args) < 1 {
			fmt.Println("No binary name provided for install command.")
			os.Exit(1)
		}
		binaries := args
		fetchMetadata()
		err := installCommand(config, binaries, verbosityLevel, metadata)
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
		err := removeBinaries(config, binaries, verbosityLevel, metadata)
		if err != nil {
			fmt.Printf("%v\n", err)
			os.Exit(1)
		}
		// fmt.Println("Removal completed successfully.")
	case "list":
		if len(os.Args) == 3 {
			if os.Args[2] == "--described" || os.Args[2] == "-d" {
				// Call fSearch with an empty query and a large limit to list all described binaries
				fetchMetadata()
				fSearch(config, "", metadata)
			} else {
				errorOut("dbin: Unknown command.\n")
			}
		} else {
			fetchMetadata()
			binaries, err := listBinaries(metadata)
			if err != nil {
				fmt.Println("Error listing binaries:", err)
				os.Exit(1)
			}
			for _, binary := range binaries {
				fmt.Println(binary)
			}
		}
	case "search":
		queryIndex := 0

		if len(args) < queryIndex+1 {
			fmt.Println("Usage: dbin search <--limit||-l [int]> [query]")
			os.Exit(1)
		}

		if len(args) > 0 && (args[queryIndex] == "--limit" || args[queryIndex] == "-l") {
			if len(args) > queryIndex+1 {
				var err error
				config.Limit, err = strconv.Atoi(args[queryIndex+1])
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
		fetchMetadata()
		err := fSearch(config, query, metadata)
		if err != nil {
			fmt.Printf("error searching binaries: %v\n", err)
			os.Exit(1)
		}
	case "info":
		var binaryName string
		var remote bool

		// Check for flags: --remote or -r
		for _, arg := range args {
			if arg == "--remote" || arg == "-r" {
				remote = true
			} else if binaryName == "" { // The first non-flag argument is treated as binaryName
				binaryName = arg
			}
		}

		if binaryName == "" {
			if !remote {
				// Get the list of files in the installation directory
				files, err := listFilesInDir(config.InstallDir)
				if err != nil {
					fmt.Printf("error listing files in %s: %v\n", config.InstallDir, err)
					os.Exit(1)
				}

				installedPrograms := make([]string, 0)

				// Loop over the files and check if they are installed
				for _, file := range files {
					fullBinaryName := listInstalled(file)
					if fullBinaryName != "" {
						installedPrograms = append(installedPrograms, fullBinaryName)
					}
				}

				// Print the installed programs
				for _, program := range installedPrograms {
					fmt.Println(program)
				}
			} else {
				// Validate programs from the remote source
				fetchMetadata()
				installedPrograms, err := validateProgramsFrom(config, nil, metadata)
				if err != nil {
					fmt.Printf("error validating programs: %v\n", err)
					os.Exit(1)
				}
				// Print the installed programs
				for _, program := range installedPrograms {
					fmt.Println(program)
				}
			}
		} else {
			fetchMetadata()
			binaryInfo, err := getBinaryInfo(config, binaryName, metadata)
			if err != nil {
				fmt.Printf("%v\n", err)
				os.Exit(1)
			}

			// Define the fields to print
			fields := []struct {
				label string
				value string
			}{
				{"Name", binaryInfo.RealName},
				{"Description", binaryInfo.Description},
				{"Note", binaryInfo.Note},
				{"Version", binaryInfo.Version},
				{"Download URL", binaryInfo.DownloadURL},
				{"Size", binaryInfo.Size},
				{"B3SUM", binaryInfo.Bsum},
				{"SHA256", binaryInfo.Shasum},
				{"Build Date", binaryInfo.BuildDate},
				{"Source URL", binaryInfo.SrcURL},
				{"Web URL", binaryInfo.WebURL},
				{"Build Script", binaryInfo.BuildScript},
				{"Build Log", binaryInfo.BuildLog},
				{"Category", binaryInfo.Category},
				{"Extra Bins", binaryInfo.ExtraBins},
			}

			// Print detailed binary information
			for _, field := range fields {
				if field.value != "" {
					truncatePrintf(config.DisableTruncation, "\033[48;5;4m%s\033[0m: %s\n", field.label, field.value)
				}
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

		fetchMetadata()
		RunFromCache(config, flag.Arg(0), flag.Args()[1:], transparentMode, verbosityLevel, metadata)
	case "tldr":
		RunFromCache(config, "tlrc", flag.Args()[1:], true, verbosityLevel, metadata)
	case "eget2":
		RunFromCache(config, "eget2", flag.Args()[1:], true, verbosityLevel, metadata)
	case "update":
		var programsToUpdate []string
		if len(os.Args) > 2 {
			programsToUpdate = os.Args[2:]
		}
		fetchMetadata()
		if err := update(config, programsToUpdate, verbosityLevel, metadata); err != nil {
			fmt.Println("Update failed:", err)
		}
	default:
		print(helpPage)
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		os.Exit(1)
	}
}
