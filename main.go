package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/xplshn/a-utils/pkg/ccmd"
)

type Verbosity int8

const (
	unsupportedArchMsg                  = "Unsupported architecture: "
	indicator                           = "...>"
	Version                             = "1.0"
	maxCacheSize                        = 15
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
search            Search for a binary by suppliyng one or more search terms
tldr              Equivalent to "--silent run --transparent tlrc"`,
			"3_Variables": `DBIN_CACHEDIR      If present, it must contain a valid directory path
DBIN_INSTALL_DIR   If present, it must contain a valid directory path
DBIN_CONFIG_FILE   If present, it must contain a valid file path, pointing to a valid dbin config file
DBIN_REPO_URLS     If present, it must contain one or more repository URLS ended in / separated by ;
DBIN_METADATA_URLS If present, it must contain one or more repository's metadata url separated by ;
DBIN_NOTRUNCATION  If present, and set to ONE (1), string truncation will be disabled
DBIN_NOCONFIG      If present, and set to ONE (1), dbin will not create nor read its config. Using default values instead
DBIN_REOWN         If present, and set to ONE (1), it makes dbin update programs that may not have been installed by dbin`,
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
dbin run firefox "https://www.paypal.com/donate/?hosted_button_id=77G7ZFXVZ44EE" # Donate?`,
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

	if (*verboseFlag && *silentFlag) || (*verboseFlag && *extraSilentFlag) || (*silentFlag && *extraSilentFlag) {
		fmt.Fprintln(os.Stderr, "error: Conflicting verbose flags provided.")
		os.Exit(1)
	}

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

	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	var uRepoIndex []binaryEntry
	fetchRepoIndex := func() []binaryEntry {
		for _, url := range config.RepoURLs {
			repoIndex, err := decodeRepoIndex(url)
			if err != nil {
				fmt.Printf("failed to fetch and decode binary information from %s: %v\n", url, err)
				continue
			}
			uRepoIndex = append(uRepoIndex, repoIndex...)
		}
		return uRepoIndex
	}

	switch command {
	case "install", "add":
		if len(args) < 1 {
			fmt.Println("No binary name provided for install command.")
			os.Exit(1)
		}
		fetchRepoIndex()
		err := installCommand(config, arrStringToArrBinaryEntry(removeDuplicates(args)), verbosityLevel, uRepoIndex)
		if err != nil {
			fmt.Printf("%v\n", err)
			os.Exit(1)
		}
	case "remove", "del":
		if len(args) < 1 {
			fmt.Println("No binary name provided for remove command.")
			os.Exit(1)
		}
		bEntries := arrStringToArrBinaryEntry(removeDuplicates(args))
		err := removeBinaries(config, bEntries, verbosityLevel, uRepoIndex)
		if err != nil {
			fmt.Printf("%v\n", err)
			os.Exit(1)
		}
	case "list":
		if len(os.Args) == 3 {
			if os.Args[2] == "--described" || os.Args[2] == "-d" {
				fetchRepoIndex()
				fSearch(config, []string{""}, uRepoIndex)
			} else {
				errorOut("dbin: Unknown command.\n")
			}
		} else {
			fetchRepoIndex()
			bEntries, err := listBinaries(uRepoIndex)
			if err != nil {
				fmt.Println("Error listing binaries:", err)
				os.Exit(1)
			}
			for _, binary := range binaryEntriesToArrString(bEntries, true) {
				fmt.Println(binary)
			}
		}
	case "search":
		queryIndex := 0

		if len(args) < queryIndex+1 {
			fmt.Println("Usage: dbin search <--limit||-l [int]> [query...]")
			os.Exit(1)
		}

		if len(args) > 0 && (args[queryIndex] == "--limit" || args[queryIndex] == "-l") {
			if len(args) > queryIndex+1 {
				var err error
				limit, err := strconv.Atoi(args[queryIndex+1])
				config.Limit = uint(limit)
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
			fmt.Println("Usage: dbin search <--limit||-l [int]> [query...]")
			os.Exit(1)
		}

		query := args[queryIndex:]
		fetchRepoIndex()
		err := fSearch(config, query, uRepoIndex)
		if err != nil {
			fmt.Printf("error searching binaries: %v\n", err)
			os.Exit(1)
		}
	case "info":
		var bEntry binaryEntry
		var remote bool

		if config.RetakeOwnership {
			remote = true
		}

		if len(args) == 1 && args[0] != "" {
			bEntry = stringToBinaryEntry(args[0])
		}

		if bEntry.Name == "" {
			if !remote {
				files, err := listFilesInDir(config.InstallDir)
				if err != nil {
					fmt.Printf("error listing files in %s: %v\n", config.InstallDir, err)
					os.Exit(1)
				}

				installedPrograms := make([]string, 0)

				for _, file := range files {
					trackedBEntry := bEntryOfinstalledBinary(file)
					if trackedBEntry.Name != "" {
						installedPrograms = append(installedPrograms, parseBinaryEntry(trackedBEntry, true))
					}
				}

				for _, program := range installedPrograms {
					fmt.Println(program)
				}
			} else {
				fetchRepoIndex()
				installedPrograms, err := validateProgramsFrom(config, nil, uRepoIndex)
				if err != nil {
					fmt.Printf("error validating programs: %v\n", err)
					os.Exit(1)
				}
				for _, program := range installedPrograms {
					fmt.Println(program.Name)
				}
			}
		} else {
			fetchRepoIndex()
			binaryInfo, err := getBinaryInfo(config, bEntry, uRepoIndex)
			if err != nil {
				fmt.Printf("%v\n", err)
				os.Exit(1)
			}

			fields := []struct {
				label string
				value interface{}
			}{
				{"Name", binaryInfo.Name + "#" + binaryInfo.PkgId},
				{"Pkg ID", binaryInfo.PkgId},
				{"Pretty Name", binaryInfo.PrettyName},
				{"Description", binaryInfo.Description},
				{"Version", binaryInfo.Version},
				{"Ghcr Blob", binaryInfo.GhcrBlob},
				{"Download URL", binaryInfo.DownloadURL},
				{"Size", binaryInfo.Size},
				{"B3SUM", binaryInfo.Bsum},
				{"SHA256", binaryInfo.Shasum},
				{"Build Date", binaryInfo.BuildDate},
				{"Build Script", binaryInfo.BuildScript},
				{"Build Log", binaryInfo.BuildLog},
				{"Categories", binaryInfo.Categories},
				{"Rank", binaryInfo.Rank},
				{"Extra Bins", binaryInfo.ExtraBins},
			}
			for _, field := range fields {
				switch v := field.value.(type) {
				case []string:
					for n, str := range v {
						prefix := "\033[48;5;4m" + field.label + "\033[0m"
						if n > 0 {
							prefix = "         "
						}
						fmt.Printf("%s: %s\n", prefix, str)
					}
				default:
					if v != "" && v != 0 {
						fmt.Printf("\033[48;5;4m%s\033[0m: %v\n", field.label, v)
					}
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

		if len(flag.Args()) < 1 {
			fmt.Println("Usage: dbin run <--transparent> [binary] <args>")
			os.Exit(1)
		}

		fetchRepoIndex()
		RunFromCache(config, stringToBinaryEntry(flag.Arg(0)), args, transparentMode, verbosityLevel, uRepoIndex)
	case "tldr":
		RunFromCache(config, stringToBinaryEntry("tlrc"), args, true, verbosityLevel, uRepoIndex)
	case "eget2":
		RunFromCache(config, stringToBinaryEntry("eget2"), args, true, verbosityLevel, uRepoIndex)
	case "update":
		fetchRepoIndex()
		if err := update(config, arrStringToArrBinaryEntry(removeDuplicates(args)), verbosityLevel, uRepoIndex); err != nil {
			fmt.Println("Update failed:", err)
		}
	case "findurl":
		if len(args) < 1 {
			fmt.Println("No binary names provided for findurl command.")
			os.Exit(1)
		}
		bEntries := arrStringToArrBinaryEntry(removeDuplicates(args))
		fetchRepoIndex()
		urls, _, err := findURL(config, bEntries, verbosityLevel, uRepoIndex)
		if err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "%v", err)
			}
			os.Exit(1)
		}
		if verbosityLevel >= normalVerbosity {
			for i, url := range urls {
				fmt.Printf("URL for %s: %s\n", bEntries[i].Name, url)
			}
		}
	case "readEmbeddedMetadata":
		if len(args) < 1 {
			fmt.Println("No binary name was provided for fullname")
			os.Exit(1)
		}
		trackedBEntry, _ := readEmbeddedBEntry(args[0])
		fmt.Println("BEntry of installed ", args[0], "is", parseBinaryEntry(trackedBEntry, false))
	default:
		print(helpPage)
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		os.Exit(1)
	}
}
