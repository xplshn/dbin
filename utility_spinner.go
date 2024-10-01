//go:build !progressbar
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/goccy/go-json"
)

const (
	ansiColor   = "\033[36m"
	ansiNoColor = "\033[0m"
)

// Spinner struct with thread-safe mechanisms
type Spinner struct {
	frames   []rune
	index    int
	mu       sync.Mutex
	stopChan chan struct{}
	speed    time.Duration
	speedMu  sync.Mutex
}

// NewSpinner creates a new Spinner instance
func NewSpinner(frames []rune) *Spinner {
	return &Spinner{
		frames:   frames,
		index:    0,
		stopChan: make(chan struct{}),
		speed:    80 * time.Millisecond, // default interval
	}
}

// Start begins the spinner in a new goroutine
func (s *Spinner) Start() {
	go func() {
		for {
			select {
			case <-s.stopChan:
				return
			default:
				s.mu.Lock()
				fmt.Printf("%s%c%s\r", ansiColor, s.frames[s.index], ansiNoColor)
				s.index = (s.index + 1) % len(s.frames)
				s.mu.Unlock()
				time.Sleep(s.getSpeed())
			}
		}
	}()
}

// Stop stops the spinner
func (s *Spinner) Stop() {
	close(s.stopChan)
}

// SetSpeed sets the spinner speed
func (s *Spinner) SetSpeed(speed time.Duration) {
	s.speedMu.Lock()
	s.speed = speed
	s.speedMu.Unlock()
}

// getSpeed gets the spinner speed
func (s *Spinner) getSpeed() time.Duration {
	s.speedMu.Lock()
	defer s.speedMu.Unlock()
	return s.speed
}

// fetchBinaryFromURLToDest downloads the file from the given URL to the specified destination and checks the checksum if provided.
func fetchBinaryFromURLToDest(ctx context.Context, url, checksum string, destination string) (string, error) {
	// Create a new HTTP request with cache-control headers
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for %s: %v", url, err)
	}

	// Add headers to disable caching
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Expires", "0")

	// Perform the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Ensure the parent directory exists
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return "", fmt.Errorf("failed to create parent directories for %s: %v", destination, err)
	}

	tempFile := destination + ".tmp"
	out, err := os.Create(tempFile)
	if err != nil {
		return "", err
	}
	defer out.Close()

	spinner := NewSpinner([]rune{
		'⠁', '⠂', '⠄', '⡀', '⡈', '⡐', '⡠', '⣀',
		'⣁', '⣂', '⣄', '⣌', '⣔', '⣤', '⣥', '⣦',
		'⣮', '⣶', '⣷', '⣿', '⡿', '⠿', '⢟', '⠟',
		'⡛', '⠛', '⠫', '⢋', '⠋', '⠍', '⡉', '⠉',
		'⠑', '⠡', '⢁', // '🕛', '🕧', '🕐', '🕜', '🕑', '🕝', '🕒', '🕞', '🕓', '🕟', '🕔', '🕠', '🕕', '🕡', '🕖', '🕢', '🕗', '🕣', '🕘', '🕤', '🕙', '🕙', '🕚', '🕦',
	})
	spinner.Start()
	defer spinner.Stop()

	var downloaded int64
	buf := make([]byte, 4096)
	startTime := time.Now()

	hash := sha256.New()
downloadLoop:
	for {
		select {
		case <-ctx.Done():
			_ = os.Remove(tempFile)
			return "", ctx.Err()
		default:
			n, err := resp.Body.Read(buf)
			if n > 0 {
				if _, err := out.Write(buf[:n]); err != nil {
					_ = os.Remove(tempFile)
					return "", err
				}

				// Write to hash for checksum calculation
				if _, err := hash.Write(buf[:n]); err != nil {
					_ = os.Remove(tempFile)
					return "", err
				}

				downloaded += int64(n)
				elapsed := time.Since(startTime).Seconds()
				speed := float64(downloaded) / elapsed // bytes per second

				// Adjust spinner speed based on download speed
				switch {
				case speed > 1024*1024: // more than 1MB/s
					spinner.SetSpeed(150 * time.Millisecond)
				case speed > 512*1024: // more than 512KB/s
					spinner.SetSpeed(100 * time.Millisecond)
				case speed > 256*1024: // more than 256KB/s
					spinner.SetSpeed(80 * time.Millisecond)
				default:
					spinner.SetSpeed(50 * time.Millisecond)
				}
			}
			if err == io.EOF {
				break downloadLoop
			}
			if err != nil {
				_ = os.Remove(tempFile)
				return "", err
			}
		}
	}

	// Final checksum verification
	if checksum != "" {
		calculatedChecksum := hex.EncodeToString(hash.Sum(nil))
		if calculatedChecksum != checksum {
			_ = os.Remove(tempFile)
			//_ = os.Remove(tempFile)
			//return "", fmt.Errorf("checksum verification failed: expected %s, got %s", checksum, calculatedChecksum)
			fmt.Fprintf(os.Stderr, "checksum verification failed: expected %s, got %s\n", checksum, calculatedChecksum)
		}
	} else {
		fmt.Println("Warning: No checksum exists for this binary in the metadata files, skipping verification.")
	}

	// Make a few corrections in case the downloaded binary is a nix object
	if err := removeNixGarbageFoundInTheRepos(tempFile); err != nil {
		_ = os.Remove(tempFile)
		return "", err
	}

	if err := os.Rename(tempFile, destination); err != nil {
		_ = os.Remove(tempFile)
		return "", err
	}

	if err := os.Chmod(destination, 0755); err != nil {
		_ = os.Remove(destination)
		return "", fmt.Errorf("failed to set executable bit for %s: %v", destination, err)
	}

	return destination, nil
}

func fetchJSON(url string, v interface{}) error {
	spinner := NewSpinner([]rune{
		'⠁', '⠂', '⠄', '⡀', '⡈', '⡐', '⡠', '⣀',
		'⣁', '⣂', '⣄', '⣌', '⣔', '⣤', '⣥', '⣦',
		'⣮', '⣶', '⣷', '⣿', '⡿', '⠿', '⢟', '⠟',
		'⡛', '⠛', '⠫', '⢋', '⠋', '⠍', '⡉', '⠉',
		'⠑', '⠡', '⢁', // '🕛', '🕧', '🕐', '🕜', '🕑', '🕝', '🕒', '🕞', '🕓', '🕟', '🕔', '🕠', '🕕', '🕡', '🕖', '🕢', '🕗', '🕣', '🕘', '🕤', '🕙', '🕙', '🕚', '🕦',
	})
	spinner.Start()
	defer spinner.Stop()

	// Create a new HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("error creating request for %s: %v", url, err)
	}

	// Add headers to disable caching
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Expires", "0")

	// Perform the request using http.DefaultClient
	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error fetching from %s: %v", url, err)
	}
	defer response.Body.Close()

	// Read the response body
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("error reading from %s: %v", url, err)
	}

	// Unmarshal the JSON response
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("error decoding from %s: %v", url, err)
	}

	return nil
}
