package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	"github.com/goccy/go-json"
)

// Config structure holding configuration settings
type Config struct {
	RepoURLs            []string        `json:"repo_urls" env:"DBIN_REPO_URLS"`
	MetadataURLs        []string        `json:"metadata_urls" env:"DBIN_METADATA_URLS"`
	InstallDir          string          `json:"install_dir" env:"DBIN_INSTALL_DIR"`
	CacheDir            string          `json:"cache_dir" env:"DBIN_CACHEDIR"`
	Limit               int             `json:"fsearch_limit"`
	DisableTruncation   bool            `json:"disable_truncation" env:"DBIN_NOTRUNCATION"`
	RetakeOwnership     bool            `json:"retake_ownership" env:"DBIN_REOWN"`
	UseIntegrationHooks bool            `json:"use_integration_hooks" env:"DBIN_USEHOOKS"`
	Hooks               Hooks           `json:"integration_hooks,omitempty"`
	CorrectionHooks     CorrectionHooks `json:"correction_hooks,omitempty"`
}

// Hooks structure holding user-defined commands per extension
type Hooks struct {
	Commands map[string]HookCommands `json:"commands"`
}

// HookCommands structure for integration and deintegration commands
type HookCommands struct {
	IntegrationCommands   []string `json:"integration_commands"`
	DeintegrationCommands []string `json:"deintegration_commands"`
	IntegrationErrorMsg   string   `json:"integration_error_msg"`
	DeintegrationErrorMsg string   `json:"deintegration_error_msg"`
	UseRunFromCache       bool     `json:"use_run_from_cache"`
	NoOp                  bool     `json:"nop"`
}

// CorrectionHooks structure holding user-defined correction hooks
type CorrectionHooks struct {
	Commands map[string]CorrectionCommand `json:"commands"`
}

// CorrectionCommand structure for correction commands
type CorrectionCommand struct {
	Command string `json:"command"`
}

func executeHookCommand(config *Config, cmdTemplate, binaryPath, extension string, isIntegration bool, verbosityLevel Verbosity) error {
	hookCommands, exists := config.Hooks.Commands[extension]
	if !exists {
		return fmt.Errorf("no commands found for extension: %s", extension)
	}

	// Check for the NoOp flag
	if hookCommands.NoOp {
		return nil
	}

	// Replace {{binary}} with the actual binary path in the command template
	cmd := strings.ReplaceAll(cmdTemplate, "{{binary}}", binaryPath)

	// Determine whether to use RunFromCache based on the config
	useRunFromCache := hookCommands.UseRunFromCache

	// Split command into command name and arguments
	commandParts := strings.Fields(cmd)
	if len(commandParts) == 0 {
		//return fmt.Errorf("no command to execute for extension: %s", extension)
		return nil
	}

	command := commandParts[0] // First part is the command
	args := commandParts[1:]   // Remaining parts are arguments

	if useRunFromCache {
		// Directly call RunFromCache with the full command string
		return RunFromCache(config, command, args, true, verbosityLevel)
	} else {
		// Create a new command
		cmdExec := exec.Command(command, args...)

		// Set the command's output to the same as the current process' output
		cmdExec.Stdout = os.Stdout
		cmdExec.Stderr = os.Stderr

		// Execute the command
		if err := cmdExec.Run(); err != nil {
			// Use the appropriate error message based on integration or deintegration
			var errorMsg string
			if isIntegration {
				errorMsg = hookCommands.IntegrationErrorMsg
			} else {
				errorMsg = hookCommands.DeintegrationErrorMsg
			}
			// Format the error message with the provided error message template
			return fmt.Errorf(errorMsg, binaryPath, err)
		}
	}

	return nil
}

// LoadConfig loads the configuration from the JSON file and handles environment variables automatically.
func LoadConfig() (*Config, error) {
	// Create a new config instance
	cfg := &Config{}

	// Get the user config directory
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user config directory: %v", err)
	}
	configFilePath := filepath.Join(userConfigDir, "dbin.json")

	// Check if the JSON file exists
	if _, err := os.Stat(configFilePath); err == nil {
		// Load the JSON file if it exists
		if err := loadJSON(configFilePath, cfg); err != nil {
			return nil, fmt.Errorf("failed to load JSON file: %v", err)
		}
	} else if !os.IsNotExist(err) {
		// Return an error if there's another issue with the file
		return nil, fmt.Errorf("failed to access JSON file: %v", err)
	} else {
		// Create a default config file if it does not exist
		if err := CreateDefaultConfig(); err != nil {
			return nil, fmt.Errorf("failed to create default config file: %v", err)
		}
		// Reload the configuration after creating the default file
		if err := loadJSON(configFilePath, cfg); err != nil {
			return nil, fmt.Errorf("failed to load JSON file: %v", err)
		}
	}

	// Override configuration with environment variables
	overrideWithEnv(cfg)

	// Set default values if needed
	setDefaultValues(cfg)

	return cfg, nil
}

// loadJSON loads configuration from a JSON file.
func loadJSON(filePath string, cfg *Config) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	return decoder.Decode(cfg)
}

// overrideWithEnv overrides configuration with environment variables.
func overrideWithEnv(cfg *Config) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		envVar := t.Field(i).Tag.Get("env")

		if value, exists := os.LookupEnv(envVar); exists {
			switch field.Kind() {
			case reflect.String:
				field.SetString(value)
			case reflect.Slice:
				field.Set(reflect.ValueOf(parseStringSlice(value)))
			case reflect.Bool:
				if val, err := parseBool(value); err == nil {
					field.SetBool(val)
				}
			case reflect.Int:
				if val, err := strconv.Atoi(value); err == nil {
					field.SetInt(int64(val))
				}
			}
		}
	}
}

// parseStringSlice splits a string by commas into a slice.
func parseStringSlice(s string) []string {
	return strings.Split(s, ",")
}

// parseBool converts a string to a boolean value.
func parseBool(s string) (bool, error) {
	return strconv.ParseBool(s)
}

// setDefaultValues sets default values for specific configuration entries only if they are not already set.
func setDefaultValues(config *Config) {
	// Setting default InstallDir if not defined
	if config.InstallDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("failed to get user's Home directory: %v\n", err)
			return
		}
		config.InstallDir = filepath.Join(homeDir, ".local/bin")
	}

	// Load cache dir from the user's cache directory
	tempDir, err := os.UserCacheDir()
	if err != nil {
		fmt.Printf("failed to get user's Cache directory: %v\n", err)
		return
	}
	if config.CacheDir == "" {
		config.CacheDir = filepath.Join(tempDir, "dbin_cache")
	}

	// Determine architecture and set default repositories and metadata URLs
	arch := runtime.GOARCH + "_" + runtime.GOOS

	// Set up default repositories if none are provided
	if len(config.RepoURLs) == 0 {
		config.RepoURLs = []string{
			"https://bin.ajam.dev/" + arch + "/",
			"https://bin.ajam.dev/" + arch + "/Baseutils/",
			"https://pkg.ajam.dev/" + arch + "/",
		}
	}

	// Set up default metadata URLs if none are provided
	if len(config.MetadataURLs) == 0 {
		config.MetadataURLs = []string{
			"https://github.com/xplshn/dbin-metadata/raw/refs/heads/master/misc/cmd/modMetadataAIO/unifiedAIO_" + arch + ".dbin.min.json",
		}
	}

	if config.Limit == 0 {
		config.Limit = 90
	}
	if !config.UseIntegrationHooks {
		config.UseIntegrationHooks = true
	}
	if !config.RetakeOwnership {
		config.RetakeOwnership = false
	}
}

// CreateDefaultConfig creates a default configuration file.
func CreateDefaultConfig() error {
	// Create a new config instance with default values
	cfg := &Config{}
	setDefaultValues(cfg)

	// Set default hooks
	cfg.Hooks = Hooks{
		Commands: map[string]HookCommands{
			".AppBundle": {
				IntegrationCommands:   []string{"pelfd --integrate {{binary}}"},
				DeintegrationCommands: []string{"pelfd --deintegrate {{binary}}"},
				IntegrationErrorMsg:   "[%s] Could not integrate with the system via pelfd; Error: %v",
				DeintegrationErrorMsg: "[%s] Could not deintegrate from the system via pelfd; Error: %v",
				UseRunFromCache:       true,
			},
			".AppImage": {
				IntegrationCommands:   []string{"pelfd --integrate {{binary}}"},
				DeintegrationCommands: []string{"pelfd --deintegrate {{binary}}"},
				IntegrationErrorMsg:   "[%s] Could not integrate with the system via pelfd; Error: %v",
				DeintegrationErrorMsg: "[%s] Could not deintegrate from the system via pelfd; Error: %v",
				UseRunFromCache:       true,
			},
			".NixAppImage": {
				IntegrationCommands:   []string{"pelfd --integrate {{binary}}"},
				DeintegrationCommands: []string{"pelfd --deintegrate {{binary}}"},
				IntegrationErrorMsg:   "[%s] Could not integrate with the system via pelfd; Error: %v",
				DeintegrationErrorMsg: "[%s] Could not deintegrate from the system via pelfd; Error: %v",
				UseRunFromCache:       true,
			},
			"": { // Normal static binaries don't need a handler, so we're just using a NoOp
				IntegrationCommands:   []string{"upx {{binary}}"},
				DeintegrationCommands: []string{""},
				IntegrationErrorMsg:   "[%s] Could not be UPXed; Error: %v",
				UseRunFromCache:       true,
				NoOp:                  true,
			},
		},
	}

	// Set default correction hooks
	cfg.CorrectionHooks = CorrectionHooks{
		Commands: map[string]CorrectionCommand{
			".no_strip": {
				Command: "{{trimSuffix}} {{binary}} .no_strip",
			},
		},
	}

	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get user config directory: %v", err)
	}
	configFilePath := filepath.Join(userConfigDir, "dbin.json")

	if err := os.MkdirAll(userConfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// Marshal the config to JSON
	configJSON, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON: %v", err)
	}

	// Write the JSON to the config file
	if err := os.WriteFile(configFilePath, configJSON, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	fmt.Printf("Default config file created at: %s\n", configFilePath)
	return nil
}
