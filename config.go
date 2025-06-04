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
	"github.com/zeebo/errs"
)

var (
	errConfigLoad       = errs.Class("config load error")
	errConfigCreate     = errs.Class("config create error")
	errConfigFileAccess = errs.Class("config file access error")
	errCommandExecution = errs.Class("command execution error")
	errSplitArgs        = errs.Class("split args error")
)

type repository struct {
	Name         string            `yaml:"Name,omitempty"`
	URL          string            `yaml:"URL" description:"URL of the repository."`
	PubKeys      map[string]string `yaml:"pubKeys" description:"URLs to the public keys for signature verification."`
	SyncInterval time.Duration     `yaml:"syncInterval" description:"Interval for syncing this repository."`
}

type config struct {
	Repositories        []repository `yaml:"Repositories" env:"DBIN_REPO_URLS" description:"List of repositories to fetch binaries from."`
	InstallDir          string       `yaml:"InstallDir" env:"DBIN_INSTALL_DIR XDG_BIN_HOME" description:"Directory where binaries will be installed."`
	CacheDir            string       `yaml:"CacheDir" env:"DBIN_CACHE_DIR" description:"Directory where cached binaries will be stored."`
	Limit               uint         `yaml:"SearchResultsLimit" description:"Limit the number of search results displayed."`
	ProgressbarStyle    int          `yaml:"PbarStyle,omitempty" description:"Style of the progress bar."`
	DisableTruncation   bool         `yaml:"Truncation" env:"DBIN_NOTRUNCATION" description:"Disable truncation of output."`
	RetakeOwnership     bool         `yaml:"RetakeOwnership" env:"DBIN_REOWN" description:"Retake ownership of installed binaries."`
	UseIntegrationHooks bool         `yaml:"IntegrationHooks" env:"DBIN_USEHOOKS" description:"Use integration hooks for binaries."`
	DisableProgressbar  bool         `yaml:"DisablePbar,omitempty" env:"DBIN_NOPBAR" description:"Disable the progress bar."`
	NoConfig            bool         `yaml:"-" env:"DBIN_NOCONFIG" description:"Disable configuration file usage."`
	ProgressbarFIFO     bool         `yaml:"-" env:"DBIN_PB_FIFO" description:"Use FIFO for progress bar."`
	Hooks               hooks        `yaml:"Hooks,omitempty"`
}

type hooks struct {
	Commands map[string]hookCommands `yaml:"commands" description:"Commands for hooks."`
}

type hookCommands struct {
	IntegrationCommand   string `yaml:"integrationCommand" description:"Command to run for integration."`
	DeintegrationCommand string `yaml:"deintegrationCommand" description:"Command to run for deintegration."`
	UseRunFromCache      bool   `yaml:"runFromCache" description:"Use run from cache for hooks."`
	NoOp                 bool   `yaml:"nop" description:"No operation flag for hooks."`
	Silent               bool   `yaml:"silent" description:"Do not notify user about the hook, at all"`
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
		Action: func(_ context.Context, c *cli.Command) error {
			if c.Bool("new") {
				return createDefaultConfig()
			} else if c.Bool("show") {
				config, err := loadConfig()
				if err != nil {
					return errConfigLoad.Wrap(err)
				}

				printConfig(config)
				return nil
			}

			return cli.ShowSubcommandHelp(c)
		},
	}
}

func printConfig(config *config) {
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

func splitArgs(cmd string) ([]string, error) {
	var args []string
	var arg []rune
	var inQuote rune
	escaped := false
	for _, c := range cmd {
		switch {
		case escaped:
			arg = append(arg, c)
			escaped = false
		case c == '\\':
			escaped = true
		case c == '"' || c == '\'':
			if inQuote == 0 {
				inQuote = c
			} else if inQuote == c {
				inQuote = 0
			} else {
				arg = append(arg, c)
			}
		case c == ' ' || c == '\t':
			if inQuote != 0 {
				arg = append(arg, c)
			} else if len(arg) > 0 {
				args = append(args, string(arg))
				arg = nil
			}
		default:
			arg = append(arg, c)
		}
	}
	if len(arg) > 0 {
		args = append(args, string(arg))
	}
	if inQuote != 0 {
		return nil, errSplitArgs.New("unterminated quote")
	}
	return args, nil
}

func executeHookCommand(config *config, cmdTemplate, bEntryPath, extension string, isIntegration bool) error {
	hookCommands, exists := config.Hooks.Commands[extension]
	if !exists {
		return errCommandExecution.New("no commands found for extension: %s", extension)
	}

	if hookCommands.NoOp {
		return nil
	}

	cmd := strings.ReplaceAll(cmdTemplate, "{{binary}}", bEntryPath)
	commandParts, err := splitArgs(cmd)
	if err != nil {
		return errCommandExecution.Wrap(err)
	}
	if len(commandParts) == 0 {
		return nil
	}

	command := commandParts[0]
	args := commandParts[1:]

	env := os.Environ()
	env = append(env, fmt.Sprintf("DBIN_INSTALL_DIR=%s", config.InstallDir))
	env = append(env, fmt.Sprintf("DBIN_CACHE_DIR=%s", config.CacheDir))
	env = append(env, fmt.Sprintf("DBIN=%s", os.Args[0]))
	env = append(env, fmt.Sprintf("DBIN_HOOK_BINARY=%s", bEntryPath))
	if isIntegration {
		env = append(env, "DBIN_HOOK_TYPE=integration")
	} else {
		env = append(env, "DBIN_HOOK_TYPE=deintegration")
	}

	if hookCommands.Silent {
		verbosityLevel = silentVerbosityWithErrors
	}

	if hookCommands.UseRunFromCache {
		return runFromCache(config, stringToBinaryEntry(command), args, true, env)
	}

	cmdExec := exec.Command(command, args...)
	cmdExec.Env = env
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr
	if err := cmdExec.Run(); err != nil {
		return errCommandExecution.Wrap(err)
	}
	return nil
}

func loadConfig() (*config, error) {
	cfg := config{}
	setDefaultValues(&cfg)

	if nocfg, ok := os.LookupEnv("DBIN_NOCONFIG"); ok && (nocfg == "1" || strings.ToLower(nocfg) == "true" || nocfg == "yes") {
		cfg.NoConfig = true
		overrideWithEnv(&cfg)
		return &cfg, nil
	}

	configFilePath := os.Getenv("DBIN_CONFIG_FILE")
	if configFilePath == "" {
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			return nil, errConfigFileAccess.Wrap(err)
		}
		configFilePath = filepath.Join(userConfigDir, "dbin.yaml")
	}

	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		if err := createDefaultConfigAt(configFilePath); err != nil {
			return nil, errConfigCreate.Wrap(err)
		}
	}

	if err := loadYAML(configFilePath, &cfg); err != nil {
		return nil, errConfigLoad.Wrap(err)
	}

	arch := runtime.GOARCH + "_" + runtime.GOOS
	for v := version - 0.1; v >= version-0.3; v -= 0.1 {
		url := fmt.Sprintf("https://github.com/xplshn/dbin-metadata/raw/refs/heads/master/misc/cmd/%.1f/%s%s", v, arch, ".lite.cbor.zst")
		for _, repo := range cfg.Repositories {
			if repo.URL == url {
				fmt.Printf("Warning: Your config may be outdated. Your repoURL matches version: %.1f, but we're in version: %.1f\n", v, version)
			}
		}
	}

	overrideWithEnv(&cfg)

	return &cfg, nil
}

func loadYAML(filePath string, cfg *config) error {
	file, err := os.Open(filePath)
	if err != nil {
		return errConfigFileAccess.Wrap(err)
	}
	defer file.Close()
	return yaml.NewDecoder(file).Decode(cfg)
}

func overrideWithEnv(cfg *config) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	setFieldFromEnv := func(field reflect.Value, envVars []string) bool {
		for _, envVar := range envVars {
			if value, exists := os.LookupEnv(envVar); exists && value != "" {
				switch field.Kind() {
				case reflect.String:
					field.SetString(value)
				case reflect.Slice:
					// Special handling for []Repository
					if field.Type() == reflect.TypeOf([]repository{}) {
						urls := strings.Split(value, ",")
						var repos []repository
						for _, url := range urls {
							repos = append(repos, repository{
								URL: strings.TrimSpace(url),
							})
						}
						field.Set(reflect.ValueOf(repos))
					} else {
						field.Set(reflect.ValueOf(strings.Split(value, ",")))
					}
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

func setDefaultValues(config *config) {
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

	config.Repositories = []repository{
		{
			URL: fmt.Sprintf("https://raw.githubusercontent.com/xplshn/dbin-metadata/refs/heads/master/misc/cmd/%.1f/%s%s", version, arch, ".lite.cbor.zst"),
			PubKeys: map[string]string{
				"bincache": "https://meta.pkgforge.dev/bincache/minisign.pub",
				"pkgcache": "https://meta.pkgforge.dev/pkgcache/minisign.pub",
			},
			SyncInterval: 6 * time.Hour,
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
	configFilePath := os.Getenv("DBIN_CONFIG_FILE")
	if configFilePath == "" {
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			return errConfigFileAccess.Wrap(err)
		}
		configFilePath = filepath.Join(userConfigDir, "dbin.yaml")
	}
	return createDefaultConfigAt(configFilePath)
}

func createDefaultConfigAt(configFilePath string) error {
	cfg := config{}
	setDefaultValues(&cfg)

	cfg.Hooks = hooks{
		Commands: map[string]hookCommands{
			"": {
				IntegrationCommand:   "sh -c \"$DBIN info > ${DBIN_CACHE_DIR}/.info\"",
				DeintegrationCommand: "sh -c \"$DBIN info > ${DBIN_CACHE_DIR}/.info\"",
				UseRunFromCache:      true,
				Silent:               true,
				NoOp:                 false,
			},
		},
	}

	dir := filepath.Dir(configFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errConfigCreate.Wrap(err)
	}
	configYAML, err := yaml.Marshal(cfg)
	if err != nil {
		return errConfigCreate.Wrap(err)
	}

	if err := os.WriteFile(configFilePath, configYAML, 0644); err != nil {
		return errConfigCreate.Wrap(err)
	}

	fmt.Printf("Default config file created at: %s\n", configFilePath)
	return nil
}
