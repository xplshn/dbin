package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
)

func main() {
	app := &cli.Command{
		Name:  "goScrap",
		Usage: "Detects Go CLI programs and generates appropriate go build commands",
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
						Name:  "relative",
						Usage: "Use relative paths in build commands (default: absolute paths)",
						Value: false,
					},
				},
				Action: detectAction,
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

	var buildCommands []string
	hasGoFiles := false
	err = filepath.Walk(absRootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			if isExcludedDir(info.Name()) {
				return filepath.SkipDir
			}
			if isValidGoCLIDir(path, verbose) {
				hasGoFiles = true
				// use relative path or not?
				var cmdPath string
				if useRelative {
					cmdPath, err = filepath.Rel(currentDir, path)
					if err != nil {
						return fmt.Errorf("failed to get relative path from %s to %s: %w", currentDir, path, err)
					}
					if cmdPath == "." {
						cmdPath = ""
					}
				} else {
					cmdPath = path
				}
				outputPath := generateOutputPath(path, output, info.Name(), absRootDir, useRelative)
				cmd := generateBuildCommand(cmdPath, outputPath, useRelative)
				buildCommands = append(buildCommands, cmd)
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking directory: %w", err)
	}

	if !hasGoFiles {
		return fmt.Errorf("no valid Go CLI programs found in %s", absRootDir)
	}

	for _, cmd := range buildCommands {
		fmt.Println(cmd)
	}
	return nil
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
		// dont check subdirs
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
				// overkill? Maybe...
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
