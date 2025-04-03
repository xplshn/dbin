package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/goccy/go-json"
	"github.com/hedzr/progressbar"
	"github.com/zeebo/blake3"
)

func downloadWithProgress(ctx context.Context, bar progressbar.PB, resp *http.Response, destination, checksum string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return fmt.Errorf("failed to create parent directories for %s: %v", destination, err)
	}

	if bar != nil {
		bar.UpdateRange(0, resp.ContentLength)
	}

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
				var writer io.Writer = io.MultiWriter(out, hash)
				if bar != nil {
					writer = io.MultiWriter(out, hash, bar)
				}
				if _, err = writer.Write(buf[:n]); err != nil {
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

	if checksum != "" && checksum != "!no_check" {
		calculatedChecksum := hex.EncodeToString(hash.Sum(nil))
		if calculatedChecksum != checksum {
			fmt.Fprintf(os.Stderr, "checksum verification failed: expected %s, got %s", checksum, calculatedChecksum)
		}
	} else {
		fmt.Println("Warning: No checksum exists for this binary in the repository index, skipping verification.")
	}

	if err := validateFileType(tempFile); err != nil {
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

func validateFileType(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	buf := make([]byte, 4)
	if _, err := file.Read(buf); err != nil {
		return err
	}

	// Check for ELF magic number
	if string(buf) == "\x7fELF" {
		return nil
	}

	// Check for shebang
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	shebangBuf := make([]byte, 128)
	if n, err := file.Read(shebangBuf); err != nil && err != io.EOF {
		return err
	} else {
		shebang := string(shebangBuf[:n])
		if strings.HasPrefix(shebang, "#!") {
			if regexp.MustCompile(`^#!\s*/nix/store/[^/]+/`).MatchString(shebang) {
				return fmt.Errorf("file contains invalid shebang (nix object/garbage): %s", shebang)
			}
			return nil
		}
	}

	return fmt.Errorf("file is neither a shell script nor an ELF. Please report this at @ https://github.com/xplshn/dbin")
}

func fetchBinaryFromURLToDest(ctx context.Context, bar progressbar.PB, bEntry binaryEntry, destination string) (string, error) {
	if strings.HasPrefix(bEntry.DownloadURL, "oci://") {
		bEntry.DownloadURL = strings.TrimPrefix(bEntry.DownloadURL, "oci://")
		return fetchOCIImage(ctx, bar, bEntry, destination)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", bEntry.DownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for %s: %v", bEntry.DownloadURL, err)
	}
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Expires", "0")
	req.Header.Set("User-Agent", fmt.Sprintf("dbin/%s", Version))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if err := downloadWithProgress(ctx, bar, resp, destination, bEntry.Bsum); err != nil {
		return "", err
	}

	return destination, nil
}

func fetchOCIImage(ctx context.Context, bar progressbar.PB, bEntry binaryEntry, destination string) (string, error) {
	parts := strings.SplitN(bEntry.DownloadURL, ":", 2)
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

	if err := downloadWithProgress(ctx, bar, resp, destination, bEntry.Bsum); err != nil {
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

func downloadManifest(ctx context.Context, registry, repository, version, token string) (map[string]interface{}, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, version)
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

		titleNoExt := filepath.Ext(title)
		titleNoExt = title[0:len(title)-len(titleNoExt)]
		if layerTitle == title || layerTitle == titleNoExt {
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
