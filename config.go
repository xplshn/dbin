package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/urfave/cli/v3"
)

type Repository struct {
	Name         string            `yaml:"Name,omitempty"`
	URL          string            `yaml:"URL" env:"DBIN_REPO_URLs" description:"URL of the repository."`
	PubKeys      map[string]string `yaml:"pubKeys" description:"URLs to the public keys for signature verification."`
	SyncInterval time.Duration     `yaml:"syncInterval" description:"Interval for syncing this repository."`
}

type Config struct {
	Repositories        []Repository `yaml:"Repositories" env:"DBIN_REPO_URLS" description:"List of repositories to fetch binaries from."`
	InstallDir          string       `yaml:"InstallDir" env:"DBIN_INSTALL_DIR XDG_BIN_HOME" description:"Directory where binaries will be installed."`
	CacheDir            string       `yaml:"CacheDir" env:"DBIN_CACHEDIR" description:"Directory where cached binaries will be stored."`
	Limit               uint         `yaml:"SearchResultsLimit" description:"Limit the number of search results displayed."`
	ProgressbarStyle    int          `yaml:"PbarStyle,omitempty" description:"Style of the progress bar."`
	DisableTruncation   bool         `yaml:"Truncation" env:"DBIN_NOTRUNCATION" description:"Disable truncation of output."`
	RetakeOwnership     bool         `yaml:"RetakeOwnership" env:"DBIN_REOWN" description:"Retake ownership of installed binaries."`
	UseIntegrationHooks bool         `yaml:"IntegrationHooks" env:"DBIN_USEHOOKS" description:"Use integration hooks for binaries."`
	DisableProgressbar  bool         `yaml:"DisablePbar,omitempty" env:"DBIN_NOPBAR" description:"Disable the progress bar."`
	NoConfig            bool         `yaml:"NoConfig" env:"DBIN_NOCONFIG" description:"Disable configuration file usage."`
	ProgressbarFIFO     bool         `env:"DBIN_PB_FIFO" description:"Use FIFO for progress bar."`
	Hooks               Hooks        `yaml:"Hooks,omitempty"`
}

type Hooks struct {
	Commands map[string]HookCommands `yaml:"commands" description:"Commands for hooks."`
}

type HookCommands struct {
	IntegrationCommands   []string `yaml:"integrationCommands" description:"Commands to run for integration."`
	DeintegrationCommands []string `yaml:"deintegrationCommands" description:"Commands to run for deintegration."`
	IntegrationErrorMsg   string   `yaml:"integrationErrorMsg" description:"Error message for integration failures."`
	DeintegrationErrorMsg string   `yaml:"deintegrationErrorMsg" description:"Error message for deintegration failures."`
	UseRunFromCache       bool     `yaml:"RunFromCache" description:"Use run from cache for hooks."`
	NoOp                  bool     `yaml:"nop" description:"No operation flag for hooks."`
}

func configCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "Manage configuration options",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "new",
				Usage: "Create a new configuration file",
			},
			&cli.BoolFlag{
				Name:  "show",
				Usage: "Show the current configuration",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.Bool("new") {
				return createDefaultConfig()
			} else if c.Bool("show") {
				config, err := loadConfig()
				if err != nil {
					return err
				}

				printConfig(config)
				return nil
			} else {
				return cli.ShowSubcommandHelp(c)
			}
		},
	}
}

func printConfig(config *Config) {
	v := reflect.ValueOf(config).Elem()
	t := v.Type()

	for i := range v.NumField() {
		field := v.Field(i)
		fieldType := t.Field(i)
		description := fieldType.Tag.Get("description")
		if description != "" {
			fmt.Printf("%s: %v\nDescription: %s\n\n", fieldType.Name, field.Interface(), description)
		}
	}
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

	// Tell user their repoUrls _may_ be outdated
	arch := runtime.GOARCH + "_" + runtime.GOOS
	for v := Version - 0.1; v >= Version-0.3; v -= 0.1 {
		url := fmt.Sprintf("https://github.com/xplshn/dbin-metadata/raw/refs/heads/master/misc/cmd/%.1f/%s%s", v, arch, ".lite.cbor.zst")
		for _, repo := range cfg.Repositories {
			if repo.URL == url {
				fmt.Printf("Warning: Your config may be outdated. Your repoURL matches version: %.1f, but we're in version: %.1f\n", v, Version)
			}
		}
	}

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

	setFieldFromEnv := func(field reflect.Value, envVars []string) bool {
		for _, envVar := range envVars {
			if value, exists := os.LookupEnv(envVar); exists && value != "" {
				switch field.Kind() {
				case reflect.String:
					field.SetString(value)
				case reflect.Slice:
					// TODO: The bareminimum for a repository is having a URL. The env variable DBIN_REPO_URLs should allow overriding the config's repos
					field.Set(reflect.ValueOf(strings.Split(value, ",")))
				case reflect.Bool:
					if val, err := strconv.ParseBool(value); err == nil {
						field.SetBool(val)
					}
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					if val, err := strconv.Atoi(value); err == nil {
						field.SetInt(int64(val))
					}
				case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
					if val, err := strconv.ParseUint(value, 10, 64); err == nil {
						field.SetUint(val)
					}
				}
				return true
			}
		}
		return false
	}

	for i := range v.NumField() {
		field := v.Field(i)
		envTags := strings.Fields(t.Field(i).Tag.Get("env"))

		if len(envTags) > 0 {
			setFieldFromEnv(field, envTags)
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

	// Set default repositories
	config.Repositories = []Repository{
		{
			URL:             fmt.Sprintf("https://raw.githubusercontent.com/xplshn/dbin-metadata/refs/heads/master/misc/cmd/%.1f/%s%s", Version, arch, ".lite.cbor.zst"),
			PubKeys: map[string]string{
				"bincache": "https://meta.pkgforge.dev/bincache/minisign.pub",
				"pkgcache": "https://meta.pkgforge.dev/pkgcache/minisign.pub",
			},
			SyncInterval:     6 * time.Hour,
		},
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
