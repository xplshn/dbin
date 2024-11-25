package main

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// differ lists programs that are installed but need an update/differ from the repo's b3sum of it
func differ(config *Config, programs []string, verbosityLevel Verbosity, metadata map[string]interface{}) error {
	var installedPrograms []string

	if programs == nil {
		var err error
		programs, err = listFilesInDir(config.InstallDir)
		if err != nil {
			return fmt.Errorf("error listing files in %s: %v", config.InstallDir, err)
		}
	}

	// Check which programs were actually installed by us
	for _, file := range programs {
		if fullBinaryName := listInstalled(file); fullBinaryName != "" {
			installedPrograms = append(installedPrograms, fullBinaryName)
		}
	}

	var (
		checked         uint32
		differsFromRepo uint32
		outputMutex     sync.Mutex
		wg              sync.WaitGroup
	)

	// Check installed programs for differences
	for _, program := range installedPrograms {
		wg.Add(1)

		go func(program string) {
			defer wg.Done()

			installPath := filepath.Join(config.InstallDir, filepath.Base(program))

			if !fileExists(installPath) { // Skip if not installed
				return
			}

			// Get local B3sum
			localB3sum, err := calculateChecksum(installPath)
			if err != nil { // skip
				return
			}

			// Fetch remote metadata
			binaryInfo, err := getBinaryInfo(config, program, metadata)
			if binaryInfo.Bsum == "" { // skip
				return
			}

			// Compare checksums
			if localB3sum != binaryInfo.Bsum {
				atomic.AddUint32(&differsFromRepo, 1)
				outputMutex.Lock()
				fmt.Println(program)
				outputMutex.Unlock()
			}

			atomic.AddUint32(&checked, 1)
		}(program)
	}

	wg.Wait()

	if verbosityLevel > normalVerbosity {
		fmt.Printf("Checked: %d, Needs Update: %d\n", atomic.LoadUint32(&checked), atomic.LoadUint32(&differsFromRepo))
	}

	return nil
}
