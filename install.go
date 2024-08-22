package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
)

func installBinary(ctx context.Context, binaryName, installDir, trackerFile string, silent Silent, repositories, metadataURLs []string) error {
	url, err := findURL(binaryName, trackerFile, repositories, metadataURLs)
	if err != nil {
		if silent != disabledVerbosity {
			fmt.Printf("Error finding URL for %s: %v\n", binaryName, err)
		}
		return err
	}

	destination := filepath.Join(installDir, filepath.Base(binaryName))
	_, err = fetchBinaryFromUrlToDest(ctx, url, destination)
	if err != nil {
		if silent != disabledVerbosity {
			fmt.Printf("Error fetching binary %s: %v\n", binaryName, err)
		}
		return err
	}

	if silent == normalVerbosity {
		fmt.Printf("Successfully installed %s\n", destination)
	}

	if err := addToTrackerFile(trackerFile, binaryName); err != nil {
		fmt.Printf("failed to update tracker file for %s: %v", binaryName, err)
	}

	return nil
}

func multipleInstall(ctx context.Context, binaries []string, installDir, trackerFile string, silent Silent, repositories, metadataURLs []string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(binaries))

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for _, binaryName := range binaries {
		wg.Add(1)
		go func(binaryName string) {
			defer wg.Done()
			if err := installBinary(ctx, binaryName, installDir, trackerFile, silent, repositories, metadataURLs); err != nil {
				errChan <- err
			}
		}(binaryName)
	}

	var finalErr error
	for err := range errChan {
		if finalErr == nil {
			finalErr = err
		} else {
			finalErr = fmt.Errorf("%v; %v", finalErr, err)
		}
	}

	if silent == normalVerbosity && finalErr != nil {
		fmt.Printf("Final errors: %v\n", finalErr)
	}

	return finalErr
}

func installCommand(binaries []string, installDir, trackerFile string, silent Silent, repositories, metadataURLs []string) error {
	if len(binaries) == 1 {
		return installBinary(context.Background(), binaries[0], installDir, trackerFile, silent, repositories, metadataURLs)
	} else if len(binaries) > 1 {
		// Remove duplicates before processing
		binaries = removeDuplicates(binaries)
		return multipleInstall(context.Background(), binaries, installDir, trackerFile, silent, repositories, metadataURLs)
	}
	return nil
}

func removeDuplicates(binaries []string) []string {
	seen := make(map[string]struct{})
	result := []string{}
	for _, binary := range binaries {
		if _, ok := seen[binary]; !ok {
			seen[binary] = struct{}{}
			result = append(result, binary)
		}
	}
	return result
}
