package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sort"

	"github.com/urfave/cli/v3"
)

type DirectoryInfo struct {
	Suffix     string `json:"suffix"`
	Path       string `json:"path"`
	FullPath   string `json:"full_path"`
	IsCommand  bool   `json:"is_command"`
	IsInternal bool   `json:"is_internal"`
}

type DetectionResult struct {
	RootDir     string          `json:"root_dir"`
	Directories []*DirectoryInfo `json:"directories"`
}

func main() {
	app := &cli.Command{
		Name:  "goScrap",
		Usage: "Detects Go CLI programs and generates appropriate go build or install commands",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "Enable verbose output to stderr",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "detect",
				Usage: "Detect Go CLI programs and output go build commands",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   "Specify output directory or file for go build commands",
						Value:   "",
					},
					&cli.BoolFlag{
						Name:    "json",
						Aliases: []string{"j"},
						Usage:   "Output results in JSON format",
						Value:   false,
					},
					&cli.BoolFlag{
						Name:  "relative",
						Usage: "Use relative paths in build commands (default: absolute paths)",
						Value: false,
					},
				},
				Action: detectAction,
			},
			{
				Name:  "install",
				Usage: "Generate go install commands for detected CLI programs",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "target",
						Aliases: []string{"t"},
						Usage:   "Specify target version or branch (latest, main, master)",
						Value:   "latest",
					},
					&cli.BoolFlag{
						Name:    "json",
						Aliases: []string{"j"},
						Usage:   "Output results in JSON format",
						Value:   false,
					},
				},
				Action: installAction,
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func detectAction(ctx context.Context, c *cli.Command) error {
	verbose := c.Bool("verbose")
	output := c.String("output")
	useRelative := c.Bool("relative")
	useJSON := c.Bool("json")
	rootDir := c.Args().First()
	if rootDir == "" {
		var err error
		rootDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	absRootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	info, err := os.Stat(absRootDir)
	if err != nil {
		return fmt.Errorf("invalid root directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", absRootDir)
	}

	result, err := detectGoCLIs(absRootDir, currentDir, output, useRelative, verbose)
	if err != nil {
		return err
	}

	if len(result.Directories) == 0 {
		return fmt.Errorf("no valid Go CLI programs found in %s", absRootDir)
	}

	if useJSON {
		outputJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(outputJSON))
		return nil
	}

	for _, dir := range result.Directories {
		outputPath := generateOutputPath(dir.FullPath, output, filepath.Base(dir.FullPath), absRootDir, useRelative)
		fmt.Println(generateBuildCommand(dir.FullPath, outputPath, useRelative))
	}
	return nil
}

func installAction(ctx context.Context, c *cli.Command) error {
	verbose := c.Bool("verbose")
	target := c.String("target")
	useJSON := c.Bool("json")
	rootDir := c.Args().First()
	if rootDir == "" {
		var err error
		rootDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	absRootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	info, err := os.Stat(absRootDir)
	if err != nil {
		return fmt.Errorf("invalid root directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", absRootDir)
	}

	result, err := detectGoCLIs(absRootDir, absRootDir, "", false, verbose)
	if err != nil {
		return err
	}

	if len(result.Directories) == 0 {
		return fmt.Errorf("no valid Go CLI programs found in %s", absRootDir)
	}

	goModPath, err := findGoModPath(absRootDir)
	if err != nil {
		return fmt.Errorf("failed to find go.mod: %w", err)
	}

	if useJSON {
		outputJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(outputJSON))
		return nil
	}

	for _, dir := range result.Directories {
		fmt.Println(generateInstallCommand(goModPath, dir.FullPath, target))
	}
	return nil
}

func detectGoCLIs(rootDir, currentDir, output string, useRelative, verbose bool) (*DetectionResult, error) {
	var result DetectionResult
	result.RootDir = rootDir
	hasGoFiles := false

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			if isExcludedDir(info.Name()) {
				return filepath.SkipDir
			}
			if isValidGoCLIDir(path, verbose) {
				hasGoFiles = true
				dirInfo := &DirectoryInfo{
					FullPath:  path,
					IsCommand: true,
				}
				if strings.Contains(path, "/internal/") || strings.HasPrefix(filepath.Base(path), "internal") {
					dirInfo.IsInternal = true
				}
				relPath, err := filepath.Rel(rootDir, path)
				if err != nil {
					return fmt.Errorf("failed to get relative path from %s to %s: %w", rootDir, path, err)
				}
				dirInfo.Path = relPath
				dirInfo.Suffix = strings.TrimPrefix(relPath, string(os.PathSeparator))
				if dirInfo.Path == "." {
					dirInfo.Path = filepath.Base(path)
					dirInfo.Suffix = filepath.Base(path)
				}
				result.Directories = append(result.Directories, dirInfo)
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking directory: %w", err)
	}

	if !hasGoFiles {
		return &result, nil
	}

	for i, dir := range result.Directories {
		if dir.Suffix == "." {
			result.Directories[i].Suffix = filepath.Base(dir.Path)
		}
	}
	sort.Slice(result.Directories, func(i, j int) bool {
		return result.Directories[i].Suffix < result.Directories[j].Suffix
	})

	return &result, nil
}

func isExcludedDir(name string) bool {
	excluded := []string{".git", "vendor", "test", "tests", "example", "examples"}
	for _, dir := range excluded {
		if strings.EqualFold(name, dir) {
			return true
		}
	}
	return false
}

func isValidGoCLIDir(dir string, verbose bool) bool {
	hasMain := false
	hasFuncMain := false
	hasValidGoFiles := false

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && path != dir {
			return filepath.SkipDir
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			hasValidGoFiles = true
			content, err := os.ReadFile(path)
			if err != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Warning: could not read %s: %v\n", path, err)
				}
				return nil
			}
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "package main") {
					hasMain = true
				}
				if strings.Contains(trimmed, "func main()") {
					hasFuncMain = true
				}
				if hasMain && hasFuncMain {
					return filepath.SkipDir
				}
			}
		}
		return nil
	})

	if err != nil && verbose {
		fmt.Fprintf(os.Stderr, "Warning: error scanning directory %s: %v\n", dir, err)
	}
	return hasMain && hasFuncMain && hasValidGoFiles
}

func findGoModPath(rootDir string) (string, error) {
	var goModPath string
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "go.mod" {
			goModPath = path
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error searching for go.mod: %w", err)
	}
	if goModPath == "" {
		return "", fmt.Errorf("no go.mod file found in %s or its subdirectories", rootDir)
	}

	content, err := os.ReadFile(goModPath)
	if err != nil {
		return "", fmt.Errorf("failed to read go.mod: %w", err)
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("no module path found in go.mod")
}

func generateOutputPath(dir, output, dirName, rootDir string, useRelative bool) string {
	if output == "" {
		return filepath.Join(dir, dirName)
	}
	if isDir(output) {
		if useRelative {
			relPath, _ := filepath.Rel(rootDir, dir)
			return filepath.Join(output, relPath, dirName)
		}
		return filepath.Join(output, dirName)
	}
	return output
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}
	return info.IsDir()
}

func generateBuildCommand(cmdPath, outputPath string, useRelative bool) string {
	if useRelative && cmdPath == "" {
		return fmt.Sprintf("go build -o %s", outputPath)
	}
	return fmt.Sprintf("go build -C %s -o %s", cmdPath, outputPath)
}

func generateInstallCommand(modulePath, cmdPath, target string) string {
	suffix := strings.TrimPrefix(cmdPath, filepath.Dir(modulePath))
	if suffix == "" || suffix == "." {
		suffix = "/cmd/" + filepath.Base(cmdPath)
	} else {
		suffix = "/cmd/" + strings.TrimPrefix(suffix, string(os.PathSeparator))
	}
	return fmt.Sprintf("go install %s%s@%s", modulePath, suffix, target)
}
