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
	"github.com/jedisct1/go-minisign"
	"github.com/zeebo/blake3"
)

// downloadWithProgress handles downloading a file with progress tracking
func downloadWithProgress(ctx context.Context, bar progressbar.PB, resp *http.Response, destination string, bEntry binaryEntry, config *Config) error {
	// Create destination directory if needed
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return fmt.Errorf("failed to create parent directories for %s: %v", destination, err)
	}

	// Initialize progress bar if provided
	if bar != nil {
		bar.UpdateRange(0, resp.ContentLength)
	}

	// Create temporary file
	tempFile := destination + ".tmp"
	out, err := os.Create(tempFile)
	if err != nil {
		return err
	}
	defer out.Close()

	// Initialize buffers and hash
	buf := make([]byte, 4096)
	hash := blake3.New()

	// Download loop
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
				// Verify checksum if provided
				if bEntry.Bsum != "" && bEntry.Bsum != "!no_check" {
					calculatedChecksum := hex.EncodeToString(hash.Sum(nil))
					if calculatedChecksum != bEntry.Bsum {
						return fmt.Errorf("checksum verification failed: expected %s, got %s", bEntry.Bsum, calculatedChecksum)
					}
				}

				// Validate file type
				if err := validateFileType(tempFile); err != nil {
					_ = os.Remove(tempFile)
					return err
				}

				// Finalize the downloaded file
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
			if err != nil {
				_ = os.Remove(tempFile)
				return err
			}
		}
	}
}

// validateFileType checks if the downloaded file is a valid executable
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

// verifySignature verifies a binary against its signature using minisign
func verifySignature(binaryPath string, sigData []byte, pubKeyURL string) error {
	// Open the binary file
	file, err := os.Open(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to open binary file: %v", err)
	}
	defer file.Close()

	// Download the public key
	pubKeyResp, err := http.Get(pubKeyURL)
	if err != nil {
		return fmt.Errorf("failed to download public key: %v", err)
	}
	defer pubKeyResp.Body.Close()

	if pubKeyResp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download public key: status code %d", pubKeyResp.StatusCode)
	}

	pubKeyData, err := io.ReadAll(pubKeyResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read public key: %v", err)
	}

	// Parse the public key
	pubKey, err := minisign.NewPublicKey(string(pubKeyData))
	if err != nil {
		return fmt.Errorf("failed to parse public key: %v", err)
	}

	// Parse the signature
	sig, err := minisign.DecodeSignature(string(sigData))
	if err != nil {
		return fmt.Errorf("failed to parse signature: %v", err)
	}

	// Read the binary data for verification
	binaryData, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read binary data: %v", err)
	}

	// Verify the signature
	verified, err := pubKey.Verify(binaryData, sig)
	if err != nil {
		return fmt.Errorf("signature verification failed: %v", err)
	}
	if !verified {
		return fmt.Errorf("signature verification failed: signature is invalid")
	}

	return nil
}

// fetchBinaryFromURLToDest handles downloading a binary from a URL with optional signature verification
func fetchBinaryFromURLToDest(ctx context.Context, bar progressbar.PB, bEntry binaryEntry, destination string, config *Config) (string, error) {
	if strings.HasPrefix(bEntry.DownloadURL, "oci://") {
		bEntry.DownloadURL = strings.TrimPrefix(bEntry.DownloadURL, "oci://")
		return fetchOCIImage(ctx, bar, bEntry, destination, config)
	}

	// Check if we need to verify the signature
	var pubKeyURL string
	if bEntry.Repository.PubKeys != nil {
		pubKeyURL = bEntry.Repository.PubKeys[bEntry.Repository.Name]
	}

	// Download the binary
	req, err := http.NewRequestWithContext(ctx, "GET", bEntry.DownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for %s: %v", bEntry.DownloadURL, err)
	}
	setRequestHeaders(req)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// First save the file to disk
	if err := downloadWithProgress(ctx, bar, resp, destination, bEntry, config); err != nil {
		return "", err
	}

	// If we need to verify the signature
	if pubKeyURL != "" {
		// Download the signature file
		sigURL := bEntry.DownloadURL + ".sig"
		sigResp, err := http.Get(sigURL)
		if err != nil {
			return "", fmt.Errorf("failed to download signature file: %v", err)
		}
		defer sigResp.Body.Close()

		if sigResp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("failed to download signature file: status code %d", sigResp.StatusCode)
		}

		sigData, err := io.ReadAll(sigResp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read signature file: %v", err)
		}

		// Verify the signature
		if err := verifySignature(destination, sigData, pubKeyURL); err != nil {
			// Remove the file if verification fails
			os.Remove(destination)
			return "", err
		}
	}

	return destination, nil
}

// setRequestHeaders sets common headers for HTTP requests
func setRequestHeaders(req *http.Request) {
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Expires", "0")
	req.Header.Set("User-Agent", fmt.Sprintf("dbin/%.1f", Version))
}

// fetchOCIImage handles downloading an OCI image with optional signature verification
func fetchOCIImage(ctx context.Context, bar progressbar.PB, bEntry binaryEntry, destination string, config *Config) (string, error) {
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
	binaryResp, sigResp, err := downloadLayerWithSignature(ctx, registry, repository, manifest, token, title)
	if err != nil {
		return "", fmt.Errorf("failed to get layer: %v", err)
	}
	defer binaryResp.Body.Close()
	if sigResp != nil {
		defer sigResp.Body.Close()
	}

	if err := downloadWithProgress(ctx, bar, binaryResp, destination, bEntry, config); err != nil {
		return "", err
	}

	var pubKeyURL string
	if bEntry.Repository.PubKeys != nil {
		pubKeyURL = bEntry.Repository.PubKeys[bEntry.Repository.Name]
	}

	if pubKeyURL != "" && sigResp != nil {
		sigData, err := io.ReadAll(sigResp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read signature data: %v", err)
		}

		// Verify the signature
		if err := verifySignature(destination, sigData, pubKeyURL); err != nil {
			os.Remove(destination)
			return "", fmt.Errorf("signature does not match: %v", err)
		}
	}

	return destination, nil
}

// parseImage parses an OCI image reference into registry and repository
func parseImage(image string) (string, string) {
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 1 {
		return "docker.io", "library/" + parts[0]
	}
	return parts[0], parts[1]
}

// getAuthToken gets an authentication token for an OCI registry
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

// downloadManifest downloads an OCI manifest
func downloadManifest(ctx context.Context, registry, repository, version, token string) (map[string]any, error) {
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

	var manifest map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

// downloadLayerWithSignature downloads an OCI layer and its signature if available
func downloadLayerWithSignature(ctx context.Context, registry, repository string, manifest map[string]any, token, title string) (*http.Response, *http.Response, error) {
	titleNoExt := filepath.Ext(title)
	titleNoExt = title[0 : len(title)-len(titleNoExt)]

	layers, ok := manifest["layers"].([]any)
	if !ok {
		return nil, nil, fmt.Errorf("invalid manifest structure")
	}

	var binaryResp, sigResp *http.Response
	var binaryDigest, sigDigest string

	// First find the binary layer
	for _, layer := range layers {
		layerMap, ok := layer.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("invalid layer structure")
		}

		annotations := layerMap["annotations"].(map[string]any)
		layerTitle := annotations["org.opencontainers.image.title"].(string)

		if layerTitle == title || layerTitle == titleNoExt {
			binaryDigest = layerMap["digest"].(string)
			break
		}
	}

	// Then find the signature layer
	for _, layer := range layers {
		layerMap, ok := layer.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("invalid layer structure")
		}

		annotations := layerMap["annotations"].(map[string]any)
		layerTitle := annotations["org.opencontainers.image.title"].(string)

		if layerTitle == title+".sig" || layerTitle == titleNoExt+".sig" {
			sigDigest = layerMap["digest"].(string)
			break
		}
	}

	if binaryDigest == "" {
		return nil, nil, fmt.Errorf("file with title '%s' not found in manifest", title)
	}

	// Download the binary
	binaryURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repository, binaryDigest)
	req, err := http.NewRequestWithContext(ctx, "GET", binaryURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	binaryResp, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}

	// Download the signature if we found one
	if sigDigest != "" {
		sigURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repository, sigDigest)
		sigReq, err := http.NewRequestWithContext(ctx, "GET", sigURL, nil)
		if err != nil {
			return binaryResp, nil, err
		}
		sigReq.Header.Set("Authorization", "Bearer "+token)

		sigResp, err = http.DefaultClient.Do(sigReq)
		if err != nil {
			return binaryResp, nil, err
		}
	}

	return binaryResp, sigResp, nil
}
