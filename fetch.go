package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/goccy/go-json"
	"github.com/hedzr/progressbar"
	"github.com/jedisct1/go-minisign"
	"github.com/pkg/xattr"
	"github.com/zeebo/blake3"
)

type ociXAttrMeta struct {
	Offset int64  `json:"offset"`
	Digest string `json:"digest"`
}

func xattrOCIKey() string { return "user.dbin.ocichunk" }
func xattrGetOCIMeta(path string) (ociXAttrMeta, error) {
	var meta ociXAttrMeta
	raw, err := xattr.Get(path, xattrOCIKey())
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return meta, err
	}
	return meta, nil
}
func xattrSetOCIMeta(path string, offset int64, digest string) error {
	meta := ociXAttrMeta{Offset: offset, Digest: digest}
	raw, _ := json.Marshal(meta)
	return xattr.Set(path, xattrOCIKey(), raw)
}
func xattrRemoveOCIMeta(path string) error {
	return xattr.Remove(path, xattrOCIKey())
}

// downloadWithProgress downloads from resp to destination, supporting resume via xattr for OCI and .tmp for HTTP.
// Always resumes .tmp files if found. For OCI, saves xattr progress. On interrupt (CTRL+C), saves progress before exit.
// It enforces checksum verification, but it only verificaties signatures if a pubkey is available for the repo
func downloadWithProgress(ctx context.Context, bar progressbar.PB, resp *http.Response, destination string, bEntry *binaryEntry, config *Config, isOCI bool) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return fmt.Errorf("failed to create parent directories for %s: %v", destination, err)
	}
	tempFile := destination + ".tmp"

	var resumeOffset int64
	if isOCI {
		if meta, err := xattrGetOCIMeta(tempFile); err == nil {
			resumeOffset = meta.Offset
		}
	} else {
		if fi, err := os.Stat(tempFile); err == nil {
			resumeOffset = fi.Size()
		}
	}

	var out *os.File
	var err error
	if resumeOffset > 0 {
		out, err = os.OpenFile(tempFile, os.O_RDWR, 0644)
		if err != nil {
			return err
		}
		if _, err := out.Seek(resumeOffset, io.SeekStart); err != nil {
			out.Close()
			return err
		}
	} else {
		out, err = os.Create(tempFile)
		if err != nil {
			return err
		}
	}
	defer out.Close()

	hash := blake3.New()
	if resumeOffset > 0 {
		rf, err := os.Open(tempFile)
		if err != nil {
			return err
		}
		if _, err := io.CopyN(hash, rf, resumeOffset); err != nil {
			rf.Close()
			return err
		}
		rf.Close()
	}
	buf := make([]byte, 32*1024)
	var written int64 = resumeOffset

	var writer io.Writer
	if bar != nil {
		writer = io.MultiWriter(out, hash, bar)
	} else {
		writer = io.MultiWriter(out, hash)
	}

	if bar != nil && resp.ContentLength > 0 {
		bar.UpdateRange(resumeOffset, resp.ContentLength+resumeOffset)
		if resumeOffset > 0 {
			bar.SetInitialValue(resumeOffset)
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	exit := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-sigCh:
			if isOCI {
				_ = xattrSetOCIMeta(tempFile, written, hex.EncodeToString(hash.Sum(nil)))
			}
			_ = out.Sync()
			close(exit)
			os.Exit(130)
		case <-exit:
		}
	}()
	defer close(exit)

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, errw := writer.Write(buf[:n]); errw != nil {
				return errw
			}
			written += int64(n)
			if isOCI && written%524288 == 0 {
				_ = xattrSetOCIMeta(tempFile, written, hex.EncodeToString(hash.Sum(nil)))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			if isOCI {
				_ = xattrSetOCIMeta(tempFile, written, hex.EncodeToString(hash.Sum(nil)))
			}
			return err
		}
	}

	if isOCI {
		_ = xattrRemoveOCIMeta(tempFile)
	}

	if bEntry.Bsum != "" && bEntry.Bsum != "!no_check" {
		calculatedChecksum := hex.EncodeToString(hash.Sum(nil))
		if calculatedChecksum != bEntry.Bsum {
			return fmt.Errorf("checksum verification failed: expected %s, got %s", bEntry.Bsum, calculatedChecksum)
		}
	}
	if err := validateFileType(tempFile); err != nil {
		return err
	}
	if err := os.Rename(tempFile, destination); err != nil {
		return err
	}
	if err := os.Chmod(destination, 0755); err != nil {
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
	if string(buf) == "\x7fELF" {
		return nil
	}
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

func verifySignature(binaryPath string, sigData []byte, bEntry *binaryEntry, cfg *Config) error {
	file, err := os.Open(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to open binary file: %v", err)
	}
	defer file.Close()
	if pubKeyURL := bEntry.Repository.PubKeys[bEntry.Repository.Name]; pubKeyURL != "" {
		pubKeyData, err := accessCachedOrFetch(pubKeyURL, bEntry.Repository.Name+".minisign", cfg)
		if err != nil {
			return fmt.Errorf("failed to download public key: %v", err)
		}
		pubKey, err := minisign.NewPublicKey(string(pubKeyData))
		if err != nil {
			return fmt.Errorf("failed to parse public key: %v", err)
		}
		sig, err := minisign.DecodeSignature(string(sigData))
		if err != nil {
			return fmt.Errorf("failed to parse signature: %v", err)
		}
		binaryData, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("failed to read binary data: %v", err)
		}
		verified, err := pubKey.Verify(binaryData, sig)
		if err != nil {
			return fmt.Errorf("signature verification failed: %v", err)
		}
		if !verified {
			return fmt.Errorf("signature verification failed: signature is invalid")
		}
		return nil
	}
	return nil
}

func fetchBinaryFromURLToDest(ctx context.Context, bar progressbar.PB, bEntry *binaryEntry, destination string, cfg *Config) (string, error) {
	if strings.HasPrefix(bEntry.DownloadURL, "oci://") {
		bEntry.DownloadURL = strings.TrimPrefix(bEntry.DownloadURL, "oci://")
		return fetchOCIImage(ctx, bar, bEntry, destination, cfg)
	}
	var pubKeyURL string
	if bEntry.Repository.PubKeys != nil {
		pubKeyURL = bEntry.Repository.PubKeys[bEntry.Repository.Name]
	}
	tempFile := destination + ".tmp"
	var resumeOffset int64
	if fi, err := os.Stat(tempFile); err == nil {
		resumeOffset = fi.Size()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", bEntry.DownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for %s: %v", bEntry.DownloadURL, err)
	}
	if resumeOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}
	setRequestHeaders(req)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := downloadWithProgress(ctx, bar, resp, destination, bEntry, cfg, false); err != nil {
		return "", err
	}
	if pubKeyURL != "" {
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
		if err := verifySignature(destination, sigData, bEntry, cfg); err != nil {
			os.Remove(destination)
			return "", err
		}
	}
	return destination, nil
}

func setRequestHeaders(req *http.Request) {
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Expires", "0")
	req.Header.Set("User-Agent", fmt.Sprintf("dbin/%.1f", Version))
}

// fetchOCIImage downloads an OCI image layer with resume support and xattr.
func fetchOCIImage(ctx context.Context, bar progressbar.PB, bEntry *binaryEntry, destination string, cfg *Config) (string, error) {
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
	binaryResp, sigResp, err := downloadLayerWithSignatureOCI(ctx, registry, repository, manifest, token, title, destination+".tmp")
	if err != nil {
		return "", fmt.Errorf("failed to get layer: %v", err)
	}
	defer binaryResp.Body.Close()
	if sigResp != nil {
		defer sigResp.Body.Close()
	}
	if err := downloadWithProgress(ctx, bar, binaryResp, destination, bEntry, cfg, true); err != nil {
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
		if err := verifySignature(destination, sigData, bEntry, cfg); err != nil {
			os.Remove(destination)
			return "", fmt.Errorf("signature does not match: %v", err)
		}
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

// downloadLayerWithSignatureOCI supports resuming via xattr and .tmp.
func downloadLayerWithSignatureOCI(ctx context.Context, registry, repository string, manifest map[string]any, token, title, tmpPath string) (*http.Response, *http.Response, error) {
	titleNoExt := strings.TrimSuffix(title, filepath.Ext(title))
	layers, ok := manifest["layers"].([]any)
	if !ok {
		return nil, nil, fmt.Errorf("invalid manifest structure")
	}
	var binaryDigest, sigDigest string
	for _, layer := range layers {
		layerMap, ok := layer.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("invalid layer structure")
		}
		annotations, ok := layerMap["annotations"].(map[string]any)
		if !ok {
			continue
		}
		layerTitle, ok := annotations["org.opencontainers.image.title"].(string)
		if !ok {
			continue
		}
		if layerTitle == title || layerTitle == titleNoExt {
			binaryDigest = layerMap["digest"].(string)
		}
		if layerTitle == title+".sig" || layerTitle == titleNoExt+".sig" {
			sigDigest = layerMap["digest"].(string)
		}
	}
	if binaryDigest == "" {
		return nil, nil, fmt.Errorf("file with title '%s' not found in manifest", title)
	}
	binaryURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repository, binaryDigest)
	req, err := http.NewRequestWithContext(ctx, "GET", binaryURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	var resumeOffset int64
	if meta, err := xattrGetOCIMeta(tmpPath); err == nil && meta.Offset > 0 {
		resumeOffset = meta.Offset
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}
	binaryResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	var sigResp *http.Response
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
