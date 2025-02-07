package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"runtime"

	"github.com/goccy/go-json"
)

type Config struct {
	RepoURLs            []string        `json:"repo_urls" env:"DBIN_REPO_URLS"`
	InstallDir          string          `json:"install_dir" env:"DBIN_INSTALL_DIR XDG_BIN_HOME"`
	CacheDir            string          `json:"cache_dir" env:"DBIN_CACHEDIR"`
	Limit               uint            `json:"fsearch_limit"`
	ProgressbarStyle    int             `json:"progressbar_style,omitempty"`
	DisableTruncation   bool            `json:"disable_truncation" env:"DBIN_NOTRUNCATION"`
	RetakeOwnership     bool            `json:"retake_ownership" env:"DBIN_REOWN"`
	UseIntegrationHooks bool            `json:"use_integration_hooks" env:"DBIN_USEHOOKS"`
	Hooks               Hooks           `json:"integration_hooks,omitempty"`
}

type Hooks struct {
	Commands map[string]HookCommands `json:"commands"`
}

type HookCommands struct {
	IntegrationCommands   []string `json:"integration_commands"`
	DeintegrationCommands []string `json:"deintegration_commands"`
	IntegrationErrorMsg   string   `json:"integration_error_msg"`
	DeintegrationErrorMsg string   `json:"deintegration_error_msg"`
	UseRunFromCache       bool     `json:"use_run_from_cache"`
	NoOp                  bool     `json:"nop"`
}

func executeHookCommand(config *Config, cmdTemplate, binaryPath, extension string, isIntegration bool, verbosityLevel Verbosity, metadata map[string]interface{}) error {
	hookCommands, exists := config.Hooks.Commands[extension]
	if !exists {
		return fmt.Errorf("no commands found for extension: %s", extension)
	}

	if hookCommands.NoOp {
		return nil
	}

	cmd := strings.ReplaceAll(cmdTemplate, "{{binary}}", binaryPath)
	useRunFromCache := hookCommands.UseRunFromCache
	commandParts := strings.Fields(cmd)
	if len(commandParts) == 0 {
		return nil
	}

	command := commandParts[0]
	args := commandParts[1:]

	if useRunFromCache {
		return RunFromCache(config, stringToBinaryEntry(command), args, true, verbosityLevel, metadata)
	}

	cmdExec := exec.Command(command, args...)
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr
	if err := cmdExec.Run(); err != nil {
		var errorMsg string
		if isIntegration {
			errorMsg = hookCommands.IntegrationErrorMsg
		} else {
			errorMsg = hookCommands.DeintegrationErrorMsg
		}
		return fmt.Errorf(errorMsg, binaryPath, err)
	}
	return nil
}

func loadConfig() (*Config, error) {
	cfg := &Config{}

	if noConfig, _ := strconv.ParseBool(os.Getenv("DBIN_NOCONFIG")); noConfig {
		return cfg, nil
	}

	configFilePath := os.Getenv("DBIN_CONFIG_FILE")
	if configFilePath == "" {
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user config directory: %v", err)
		}
		configFilePath = filepath.Join(userConfigDir, "dbin.json")
	}

	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		if err := createDefaultConfig(); err != nil {
			return nil, fmt.Errorf("failed to create default config file: %v", err)
		}
	}

	if err := loadJSON(configFilePath, cfg); err != nil {
		return nil, fmt.Errorf("failed to load JSON file: %v", err)
	}
	overrideWithEnv(cfg)
	return cfg, nil
}

func loadJSON(filePath string, cfg *Config) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewDecoder(file).Decode(cfg)
}

func overrideWithEnv(cfg *Config) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	setFieldFromEnv := func(field reflect.Value, envVar string) bool {
		if value, exists := os.LookupEnv(envVar); exists {
			switch field.Kind() {
			case reflect.String:
				field.SetString(value)
			case reflect.Slice:
				field.Set(reflect.ValueOf(strings.Split(value, ",")))
			case reflect.Bool:
				if val, err := strconv.ParseBool(value); err == nil {
					field.SetBool(val)
				}
			case reflect.Int:
				if val, err := strconv.Atoi(value); err == nil {
					field.SetInt(int64(val))
				}
			}
			return true
		}
		return false
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		envTags := strings.Split(t.Field(i).Tag.Get("env"), " ")

		if len(envTags) > 0 && setFieldFromEnv(field, envTags[0]) {
			continue
		}

		if field.IsZero() {
			for _, envVar := range envTags[1:] {
				if setFieldFromEnv(field, envVar) {
					break
				}
			}
		}
	}
}

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

	// Set up default metadata URLs if none are provided
	config.RepoURLs = []string{
		"https://github.com/xplshn/dbin-metadata/raw/refs/heads/master/misc/cmd/modMetadata/METADATA_" + arch + ".lite.min.json.gz",
	}

	config.DisableTruncation = false
	config.Limit = 90
	config.UseIntegrationHooks = true
	config.RetakeOwnership = false
	config.ProgressbarStyle = 1
}

// createDefaultConfig creates a default configuration file.
func createDefaultConfig() error {
	cfg := &Config{}

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
			"": { // Normal static binaries don't have an extension, so we're just using a ""
				IntegrationCommands:   []string{"upx {{binary}}"},
				DeintegrationCommands: []string{""},
				IntegrationErrorMsg:   "[%s] Could not be UPXed; Error: %v",
				UseRunFromCache:       true,
				NoOp:                  true,
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

	configJSON, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON: %v", err)
	}

	if err := os.WriteFile(configFilePath, configJSON, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	fmt.Printf("Default config file created at: %s\n", configFilePath)
	return nil
}
