package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-json"
	"github.com/hedzr/progressbar"
	"github.com/zeebo/blake3"
)

// downloadWithProgress handles the common download logic with progress tracking and checksum verification
func downloadWithProgress(ctx context.Context, bar progressbar.PB, resp *http.Response, destination, checksum string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return fmt.Errorf("failed to create parent directories for %s: %v", destination, err)
	}

	bar.UpdateRange(0, resp.ContentLength)

	// Create temp file
	tempFile := destination + ".tmp"
	out, err := os.Create(tempFile)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, 4096)
	hash := blake3.New()

downloadLoop:
	for {
		select {
		case <-ctx.Done():
			_ = os.Remove(tempFile)
			return ctx.Err()
		default:
			n, err := resp.Body.Read(buf)
			if n > 0 {
				if _, err = io.MultiWriter(out, hash, bar).Write(buf[:n]); err != nil {
					_ = os.Remove(tempFile)
					return err
				}
			}
			if err == io.EOF {
				break downloadLoop
			}
			if err != nil {
				_ = os.Remove(tempFile)
				return err
			}
		}
	}

	// Checksum verification
	if checksum != "" && checksum != "null" {
		calculatedChecksum := hex.EncodeToString(hash.Sum(nil))
		if calculatedChecksum != checksum {
			fmt.Fprintf(os.Stderr, "checksum verification failed: expected %s, got %s", checksum, calculatedChecksum)
		}
	} else {
		fmt.Println("Warning: No checksum exists for this binary in the metadata files, skipping verification.")
	}

	// Handle nix objects
	if err := removeNixGarbageFoundInTheRepos(tempFile); err != nil {
		_ = os.Remove(tempFile)
		return err
	}

	if err := os.Rename(tempFile, destination); err != nil {
		_ = os.Remove(tempFile)
		return err
	}

	if err := os.Chmod(destination, 0755); err != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("failed to set executable bit for %s: %v", destination, err)
	}

	return nil
}

func fetchBinaryFromURLToDest(ctx context.Context, bar progressbar.PB, url, checksum, destination string) (string, error) {
	// Check if the URL is an OCI reference
	if strings.HasPrefix(url, "ghcr.io/") {
		return fetchOCIImage(ctx, bar, url, checksum, destination)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for %s: %v", url, err)
	}
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Expires", "0")
	req.Header.Set("User-Agent", fmt.Sprintf("dbin/%s", Version))

	// Perform the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if err := downloadWithProgress(ctx, bar, resp, destination, checksum); err != nil {
		return "", err
	}

	return destination, nil
}

func fetchOCIImage(ctx context.Context, bar progressbar.PB, ref, checksum, destination string) (string, error) {
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid OCI reference format")
	}
	image, tag := parts[0], parts[1]

	registry, repository := parseImage(image)

	token, err := getAuthToken(registry, repository)
	if err != nil {
		return "", fmt.Errorf("failed to get auth token: %v", err)
	}

	manifest, err := downloadManifest(ctx, registry, repository, tag, token)
	if err != nil {
		return "", fmt.Errorf("failed to get manifest: %v", err)
	}

	title := filepath.Base(destination)
	resp, err := downloadLayer(ctx, registry, repository, manifest, token, title)
	if err != nil {
		return "", fmt.Errorf("failed to get layer: %v", err)
	}
	defer resp.Body.Close()

	if err := downloadWithProgress(ctx, bar, resp, destination, checksum); err != nil {
		return "", err
	}

	return destination, nil
}

func parseImage(image string) (string, string) {
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 1 {
		return "docker.io", "library/" + parts[0]
	}
	return parts[0], parts[1]
}

func getAuthToken(registry, repository string) (string, error) {
	url := fmt.Sprintf("https://%s/token?service=%s&scope=repository:%s:pull", registry, registry, repository)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tokenResponse struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", err
	}
	return tokenResponse.Token, nil
}

func downloadManifest(ctx context.Context, registry, repository, tag, token string) (map[string]interface{}, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, tag)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var manifest map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func downloadLayer(ctx context.Context, registry, repository string, manifest map[string]interface{}, token, title string) (*http.Response, error) {
	layers, ok := manifest["layers"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid manifest structure")
	}

	for _, layer := range layers {
		layerMap, ok := layer.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid layer structure")
		}

		annotations := layerMap["annotations"].(map[string]interface{})
		layerTitle := annotations["org.opencontainers.image.title"].(string)

		if layerTitle == title {
			digest := layerMap["digest"].(string)
			url := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repository, digest)

			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			return http.DefaultClient.Do(req)
		}
	}

	return nil, fmt.Errorf("file with title '%s' not found in manifest", title)
}

