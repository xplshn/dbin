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
	"time"

	"github.com/goccy/go-json"
	"github.com/hedzr/progressbar"
	"github.com/jedisct1/go-minisign"
	"github.com/pkg/xattr"
	"github.com/zeebo/blake3"
	"github.com/zeebo/errs"
)

var (
	errDownloadFailed   = errs.Class("download failed")
	errSignatureVerify  = errs.Class("signature verification failed")
	errChecksumMismatch = errs.Class("checksum mismatch")
	errOCIReference     = errs.Class("invalid OCI reference")
	errAuthToken        = errs.Class("failed to get auth token")
	errManifestDownload = errs.Class("failed to download manifest")
	errOCILayerDownload = errs.Class("failed to download OCI layer")
)

type ociXAttrMeta struct {
	Offset int64  `json:"offset"`
	Digest string `json:"digest"`
}

func xattrGetOCIMeta(path string) (ociXAttrMeta, error) {
	var meta ociXAttrMeta
	raw, err := xattr.Get(path, "user.dbin.ocichunk")
	if err != nil {
		return meta, errDownloadFailed.Wrap(err)
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return meta, errDownloadFailed.Wrap(err)
	}
	return meta, nil
}
func xattrSetOCIMeta(path string, offset int64, digest string) error {
	meta := ociXAttrMeta{Offset: offset, Digest: digest}
	raw, _ := json.Marshal(meta)
	return xattr.Set(path, "user.dbin.ocichunk", raw)
}

func downloadWithProgress(ctx context.Context, bar progressbar.PB, resp *http.Response, destination string, bEntry *binaryEntry, isOCI bool, lastModified string, providedOffset int64) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return errDownloadFailed.Wrap(err)
	}
	tempFile := destination + ".tmp"

	var resumeOffset int64
	if isOCI {
		if meta, err := xattrGetOCIMeta(tempFile); err == nil {
			resumeOffset = meta.Offset
		}
	} else {
		// For plain HTTP, use the offset we calculated in the calling function
		resumeOffset = providedOffset
	}

	var out *os.File
	var err error
	if resumeOffset > 0 {
		out, err = os.OpenFile(tempFile, os.O_RDWR, 0644)
		if err != nil {
			return errDownloadFailed.Wrap(err)
		}
		if _, err := out.Seek(resumeOffset, io.SeekStart); err != nil {
			out.Close()
			return errDownloadFailed.Wrap(err)
		}
	} else {
		out, err = os.Create(tempFile)
		if err != nil {
			return errDownloadFailed.Wrap(err)
		}
	}
	defer out.Close()

	// Store Last-Modified time for future resume validation (plain HTTP only)
	if !isOCI && lastModified != "" {
		_ = xattr.Set(tempFile, "user.dbin.lastmod", []byte(lastModified))
	}

	hash := blake3.New()
	if resumeOffset > 0 {
		rf, err := os.Open(tempFile)
		if err != nil {
			return errDownloadFailed.Wrap(err)
		}
		if _, err := io.CopyN(hash, rf, resumeOffset); err != nil {
			rf.Close()
			return errDownloadFailed.Wrap(err)
		}
		rf.Close()
	}
	buf := make([]byte, 64*1024)
	written := resumeOffset

	var writer io.Writer
	if bar != nil {
		writer = io.MultiWriter(out, hash, bar)
	} else {
		writer = io.MultiWriter(out, hash)
	}

	if bar != nil {
		if resp.StatusCode == http.StatusPartialContent {
			// We're resuming - ContentLength is remaining bytes
			bar.UpdateRange(resumeOffset-written, resp.ContentLength+resumeOffset)
			bar.SetInitialValue(resumeOffset)
		} else if resp.ContentLength > 0 {
			// Full download
			bar.UpdateRange(0, resp.ContentLength)
			if resumeOffset > 0 {
				bar.SetInitialValue(resumeOffset)
			}
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
				return errDownloadFailed.Wrap(errw)
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
			return errDownloadFailed.Wrap(err)
		}
	}

	// Clean up metadata
	if isOCI {
		_ = xattr.Remove(tempFile, "user.dbin.ocichunk")
	} else {
		_ = xattr.Remove(tempFile, "user.dbin.lastmod")
	}

	if bEntry.Bsum != "" && bEntry.Bsum != "!no_check" {
		calculatedChecksum := hex.EncodeToString(hash.Sum(nil))
		if calculatedChecksum != bEntry.Bsum {
			return errChecksumMismatch.New("expected %s, got %s", bEntry.Bsum, calculatedChecksum)
		}
	}
	if err := validateFileType(tempFile); err != nil {
		return errFileTypeInvalid.Wrap(err)
	}
	if err := os.Rename(tempFile, destination); err != nil {
		return errDownloadFailed.Wrap(err)
	}
	if err := os.Chmod(destination, 0755); err != nil {
		return errDownloadFailed.Wrap(err)
	}
	return nil
}

func validateFileType(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return errFileTypeInvalid.Wrap(err)
	}
	defer file.Close()
	buf := make([]byte, 4)
	if _, err := file.Read(buf); err != nil {
		return errFileTypeInvalid.Wrap(err)
	}
	if string(buf) == "\x7fELF" {
		return nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return errFileTypeInvalid.Wrap(err)
	}
	shebangBuf := make([]byte, 128)

	n, err := file.Read(shebangBuf)
	if err != nil && err != io.EOF {
		return errFileTypeInvalid.Wrap(err)
	}

	shebang := string(shebangBuf[:n])
	if strings.HasPrefix(shebang, "#!") {
		if regexp.MustCompile(`^#!\s*/nix/store/[^/]+/`).MatchString(shebang) {
			return errFileTypeInvalid.New("file contains invalid shebang (nix object/garbage): %s", shebang)
		}
		return nil
	}

	return errFileTypeInvalid.New("file is neither a shell script nor an ELF. Please report this at @ https://github.com/xplshn/dbin")
}

func verifySignature(binaryPath string, sigData []byte, bEntry *binaryEntry, cfg *config) error {
	file, err := os.Open(binaryPath)
	if err != nil {
		return errSignatureVerify.Wrap(err)
	}
	defer file.Close()
	if pubKeyURL := bEntry.Repository.PubKeys[bEntry.Repository.Name]; pubKeyURL != "" {
		pubKeyData, err := accessCachedOrFetch(pubKeyURL, bEntry.Repository.Name+".minisign", cfg)
		if err != nil {
			return errSignatureVerify.Wrap(err)
		}
		pubKey, err := minisign.NewPublicKey(string(pubKeyData))
		if err != nil {
			return errSignatureVerify.Wrap(err)
		}
		sig, err := minisign.DecodeSignature(string(sigData))
		if err != nil {
			return errSignatureVerify.Wrap(err)
		}
		binaryData, err := io.ReadAll(file)
		if err != nil {
			return errSignatureVerify.Wrap(err)
		}
		verified, err := pubKey.Verify(binaryData, sig)
		if err != nil {
			return errSignatureVerify.Wrap(err)
		}
		if !verified {
			return errSignatureVerify.New("signature is invalid")
		}
		return nil
	}
	return nil
}

func fetchBinaryFromURLToDest(ctx context.Context, bar progressbar.PB, bEntry *binaryEntry, destination string, cfg *config) error {
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
	var lastModified string

	// Check if we have a partial download
	if fi, err := os.Stat(tempFile); err == nil {
		resumeOffset = fi.Size()
		// Get the modification time we stored when we started the download
		if modTimeBytes, err := xattr.Get(tempFile, "user.dbin.lastmod"); err == nil {
			lastModified = string(modTimeBytes)
		}
	}

	// Create initial request to check if resume is valid
	req, err := http.NewRequestWithContext(ctx, "HEAD", bEntry.DownloadURL, nil)
	if err != nil {
		return errDownloadFailed.Wrap(err)
	}

	setRequestHeaders(req)
	client := &http.Client{}
	headResp, err := client.Do(req)
	if err != nil {
		return errDownloadFailed.Wrap(err)
	}
	headResp.Body.Close()
	// Now make the actual download request
	req, err = http.NewRequestWithContext(ctx, "GET", bEntry.DownloadURL, nil)
	if err != nil {
		return errDownloadFailed.Wrap(err)
	}

	if resumeOffset > 0 {
		if lastModified != "" {
			req.Header.Set("If-Range", lastModified)
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}

	setRequestHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return errDownloadFailed.Wrap(err)
	}
	defer resp.Body.Close()

	// If we requested a range but got 200 instead of 206, file changed
	if resumeOffset > 0 && resp.StatusCode == http.StatusOK {
		// Server is sending full file, remove temp and start fresh
		os.Remove(tempFile)
		resumeOffset = 0
	}

	// Store Last-Modified for future resume attempts
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		lastModified = lm
	}

	if err := downloadWithProgress(ctx, bar, resp, destination, bEntry, false, lastModified, resumeOffset); err != nil {
		return errDownloadFailed.Wrap(err)
	}

	if pubKeyURL != "" {
		sigURL := bEntry.DownloadURL + ".sig"
		sigResp, err := http.Get(sigURL)
		if err != nil {
			return errSignatureVerify.Wrap(err)
		}
		defer sigResp.Body.Close()
		if sigResp.StatusCode != http.StatusOK {
			return errSignatureVerify.New("status code %d", sigResp.StatusCode)
		}
		sigData, err := io.ReadAll(sigResp.Body)
		if err != nil {
			return errSignatureVerify.Wrap(err)
		}
		if err := verifySignature(destination, sigData, bEntry, cfg); err != nil {
			os.Remove(destination)
			return errSignatureVerify.Wrap(err)
		}
	}
	return nil
}

func setRequestHeaders(req *http.Request) {
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Expires", "0")
	req.Header.Set("User-Agent", fmt.Sprintf("dbin/%.1f", version))
}

func fetchOCIImage(ctx context.Context, bar progressbar.PB, bEntry *binaryEntry, destination string, cfg *config) error {
	parts := strings.SplitN(bEntry.DownloadURL, ":", 2)
	if len(parts) != 2 {
		return errOCIReference.New("invalid OCI reference format")
	}
	image, tag := parts[0], parts[1]
	registry, repository := parseImage(image)
	token, err := getAuthToken(registry, repository)
	if err != nil {
		return errAuthToken.Wrap(err)
	}
	manifest, err := downloadManifest(ctx, registry, repository, tag, token)
	if err != nil {
		return errManifestDownload.Wrap(err)
	}
	title := filepath.Base(destination)
	binaryResp, sigResp, err := downloadOCILayer(ctx, registry, repository, manifest, token, title, destination+".tmp")
	if err != nil {
		return errOCILayerDownload.Wrap(err)
	}
	defer binaryResp.Body.Close()
	if sigResp != nil {
		defer sigResp.Body.Close()
	}
	if err := downloadWithProgress(ctx, bar, binaryResp, destination, bEntry, true, "", 0); err != nil {
		return errDownloadFailed.Wrap(err)
	}
	var pubKeyURL string
	if bEntry.Repository.PubKeys != nil {
		pubKeyURL = bEntry.Repository.PubKeys[bEntry.Repository.Name]
	}
	if pubKeyURL != "" && sigResp != nil {
		sigData, err := io.ReadAll(sigResp.Body)
		if err != nil {
			return errSignatureVerify.Wrap(err)
		}
		if err := verifySignature(destination, sigData, bEntry, cfg); err != nil {
			os.Remove(destination)
			return errSignatureVerify.Wrap(err)
		}
	}
	return nil
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
		return "", errAuthToken.Wrap(err)
	}
	defer resp.Body.Close()
	var tokenResponse struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", errAuthToken.Wrap(err)
	}
	return tokenResponse.Token, nil
}

func downloadManifest(ctx context.Context, registry, repository, version, token string) (map[string]any, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, version)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errManifestDownload.Wrap(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errManifestDownload.Wrap(err)
	}
	defer resp.Body.Close()
	var manifest map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, errManifestDownload.Wrap(err)
	}
	return manifest, nil
}

func downloadOCILayer(ctx context.Context, registry, repository string, manifest map[string]any, token, title, tmpPath string) (*http.Response, *http.Response, error) {
	titleNoExt := strings.TrimSuffix(title, filepath.Ext(title))
	layers, ok := manifest["layers"].([]any)
	if !ok {
		return nil, nil, errOCILayerDownload.New("invalid manifest structure")
	}
	var binaryDigest, sigDigest string
	for _, layer := range layers {
		layerMap, ok := layer.(map[string]any)
		if !ok {
			return nil, nil, errOCILayerDownload.New("invalid layer structure")
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
		return nil, nil, errOCILayerDownload.New("file with title '%s' not found in manifest", title)
	}
	binaryURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repository, binaryDigest)
	req, err := http.NewRequestWithContext(ctx, "GET", binaryURL, nil)
	if err != nil {
		return nil, nil, errOCILayerDownload.Wrap(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	var resumeOffset int64
	if meta, err := xattrGetOCIMeta(tmpPath); err == nil && meta.Offset > 0 {
		resumeOffset = meta.Offset
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}
	binaryResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, errOCILayerDownload.Wrap(err)
	}
	var sigResp *http.Response
	if sigDigest != "" {
		sigURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repository, sigDigest)
		sigReq, err := http.NewRequestWithContext(ctx, "GET", sigURL, nil)
		if err != nil {
			return binaryResp, nil, errOCILayerDownload.Wrap(err)
		}
		sigReq.Header.Set("Authorization", "Bearer "+token)
		sigResp, err = http.DefaultClient.Do(sigReq)
		if err != nil {
			return binaryResp, nil, errOCILayerDownload.Wrap(err)
		}
	}
	return binaryResp, sigResp, nil
}

// cleanTempFiles removes .tmp files in the InstallDir that haven't been accessed in over a day.
func cleanInstallCache(installDir string) error {
    const oneDay = 24 * time.Hour
    now := time.Now()

    entries, err := os.ReadDir(installDir)
    if err != nil {
        return errFileAccess.Wrap(err)
    }

    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmp") {
            continue
        }

        filePath := filepath.Join(installDir, entry.Name())
        fileInfo, err := os.Stat(filePath)
        if err != nil {
            if verbosityLevel >= silentVerbosityWithErrors {
                fmt.Fprintf(os.Stderr, "Error accessing file info for %s: %v\n", filePath, err)
            }
            continue
        }
        var atime time.Time
        if sysInfo, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
            atime = time.Unix(sysInfo.Atim.Sec, sysInfo.Atim.Nsec)
        } else {
            if verbosityLevel >= extraVerbose {
                fmt.Fprintf(os.Stderr, "Warning: ATime not supported for %s, skipping cleanup\n", filePath)
            }
            continue
        }

        if now.Sub(atime) > oneDay {
            if err := os.Remove(filePath); err != nil {
                if verbosityLevel >= silentVerbosityWithErrors {
                    fmt.Fprintf(os.Stderr, "Error removing old .tmp file %s: %v\n", filePath, err)
                }
            } else if verbosityLevel >= extraVerbose {
                fmt.Printf("Removed old .tmp file: %s\n", filePath)
            }
        }
    }

    return nil
}
