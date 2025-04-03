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

	"github.com/goccy/go-yaml"
)

type Config struct {
	RepoURLs            []string `yaml:"RepoURLs" env:"DBIN_REPO_URLS"`
	InstallDir          string   `yaml:"InstallDir" env:"DBIN_INSTALL_DIR XDG_BIN_HOME"`
	CacheDir            string   `yaml:"CacheDir" env:"DBIN_CACHEDIR"`
	Limit               uint     `yaml:"SearchResultsLimit"`
	ProgressbarStyle    int      `yaml:"PbarStyle,omitempty"`
	DisableTruncation   bool     `yaml:"Truncation" env:"DBIN_NOTRUNCATION"`
	RetakeOwnership     bool     `yaml:"RetakeOwnership" env:"DBIN_REOWN"`
	UseIntegrationHooks bool     `yaml:"IntegrationHooks" env:"DBIN_USEHOOKS"`
	DisableProgressbar  bool     `yaml:"DisablePbar,omitempty" env:"DBIN_NOPBAR"`
	NoConfig            bool     `yaml:"NoConfig" env:"DBIN_NOCONFIG"`
	Hooks               Hooks    `yaml:"Hooks,omitempty"`
}

type Hooks struct {
	Commands map[string]HookCommands `yaml:"commands"`
}

type HookCommands struct {
	IntegrationCommands   []string `yaml:"integrationCommands"`
	DeintegrationCommands []string `yaml:"deintegrationCommands"`
	IntegrationErrorMsg   string   `yaml:"integrationErrorMsg"`
	DeintegrationErrorMsg string   `yaml:"deintegrationErrorMsg"`
	UseRunFromCache       bool     `yaml:"RunFromCache"`
	NoOp                  bool     `yaml:"nop"`
}

func executeHookCommand(config *Config, cmdTemplate, bEntryPath, extension string, isIntegration bool, verbosityLevel Verbosity, uRepoIndex []binaryEntry) error {
	hookCommands, exists := config.Hooks.Commands[extension]
	if !exists {
		return fmt.Errorf("no commands found for extension: %s", extension)
	}

	if hookCommands.NoOp {
		return nil
	}

	cmd := strings.ReplaceAll(cmdTemplate, "{{binary}}", bEntryPath)
	commandParts := strings.Fields(cmd)
	if len(commandParts) == 0 {
		return nil
	}

	command := commandParts[0]
	args := commandParts[1:]

	if hookCommands.UseRunFromCache {
		return runFromCache(config, stringToBinaryEntry(command), args, true, verbosityLevel)
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
		return fmt.Errorf(errorMsg, bEntryPath, err)
	}
	return nil
}

func loadConfig() (*Config, error) {
	cfg := Config{}
	setDefaultValues(&cfg)

	if !cfg.NoConfig {
		configFilePath := os.Getenv("DBIN_CONFIG_FILE")
		if configFilePath == "" {
			userConfigDir, err := os.UserConfigDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get user config directory: %v", err)
			}
			configFilePath = filepath.Join(userConfigDir, "dbin.yaml")
		}
		if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
			if err := createDefaultConfig(); err != nil {
				return nil, fmt.Errorf("failed to create default config file: %v", err)
			}
		}
		if err := loadYAML(configFilePath, &cfg); err != nil {
			return nil, fmt.Errorf("failed to load YAML file: %v", err)
		}
	}

	overrideWithEnv(&cfg)
	return &cfg, nil
}

func loadYAML(filePath string, cfg *Config) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	return yaml.NewDecoder(file).Decode(cfg)
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("failed to get user's Home directory: %v\n", err)
		return
	}
	config.InstallDir = filepath.Join(homeDir, ".local/bin")
	tempDir, err := os.UserCacheDir()
	if err != nil {
		fmt.Printf("failed to get user's Cache directory: %v\n", err)
		return
	}
	config.CacheDir = filepath.Join(tempDir, "dbin_cache")
	arch := runtime.GOARCH + "_" + runtime.GOOS
	config.RepoURLs = []string{
		"https://raw.githubusercontent.com/xplshn/dbin-metadata/refs/heads/master/misc/cmd/" + version + "/" + arch + ".lite.cbor.zst",
	}
	config.DisableTruncation = false
	config.Limit = 90
	config.UseIntegrationHooks = true
	config.RetakeOwnership = false
	config.ProgressbarStyle = 1
	config.DisableProgressbar = false
	config.NoConfig = false
}

func createDefaultConfig() error {
	cfg := Config{}
	setDefaultValues(&cfg)
	//overrideWithEnv(&cfg)

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
			"": {
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
	configFilePath := filepath.Join(userConfigDir, "dbin.yaml")

	if err := os.MkdirAll(userConfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	configYAML, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %v", err)
	}

	if err := os.WriteFile(configFilePath, configYAML, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	fmt.Printf("Default config file created at: %s\n", configFilePath)
	return nil
}
