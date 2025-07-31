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

func getOCIMeta(path string) (ociXAttrMeta, error) {
	var meta ociXAttrMeta
	raw, err := xattr.Get(path, "user.dbin.ocichunk")
	if err != nil {
		return meta, err
	}
	return meta, json.Unmarshal(raw, &meta)
}

func setOCIMeta(path string, offset int64, digest string) error {
	meta := ociXAttrMeta{Offset: offset, Digest: digest}
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	xattr.Set(path, "user.dbin.ocichunk", raw)
	return nil
}

func downloadWithProgress(ctx context.Context, bar progressbar.PB, resp *http.Response, destination string, bEntry *binaryEntry, isOCI bool, lastModified string, providedOffset int64) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return errDownloadFailed.Wrap(err)
	}

	tempFile := destination + ".tmp"
	resumeOffset := providedOffset
	if isOCI {
		if meta, err := getOCIMeta(tempFile); err == nil {
			resumeOffset = meta.Offset
		}
	}

	out, err := openOrCreateFile(tempFile, resumeOffset)
	if err != nil {
		return errDownloadFailed.Wrap(err)
	}
	defer out.Close()

	if !isOCI && lastModified != "" {
		xattr.Set(tempFile, "user.dbin.lastmod", []byte(lastModified))
	}

	hash, err := initializeHash(tempFile, resumeOffset)
	if err != nil {
		return errDownloadFailed.Wrap(err)
	}

	writer := setupWriter(out, hash, bar, resp, resumeOffset)

	_, err = copyWithInterruption(ctx, writer, resp.Body, hash, tempFile, isOCI, resumeOffset)
	if err != nil {
		return err
	}

	if err := cleanupMetadata(tempFile, isOCI); err != nil {
		return errDownloadFailed.Wrap(err)
	}

	if err := verifyChecksum(hash, bEntry, tempFile); err != nil {
		return err
	}

	if err := validateFileType(tempFile); err != nil {
		return err
	}

	if err := os.Rename(tempFile, destination); err != nil {
		return errDownloadFailed.Wrap(err)
	}

	return os.Chmod(destination, 0755)
}

func openOrCreateFile(path string, offset int64) (*os.File, error) {
	if offset > 0 {
		out, err := os.OpenFile(path, os.O_RDWR, 0644)
		if err != nil {
			return nil, err
		}
		if _, err := out.Seek(offset, io.SeekStart); err != nil {
			out.Close()
			return nil, err
		}
		return out, nil
	}
	return os.Create(path)
}

func initializeHash(tempFile string, resumeOffset int64) (*blake3.Hasher, error) {
	hash := blake3.New()
	if resumeOffset > 0 {
		rf, err := os.Open(tempFile)
		if err != nil {
			return nil, err
		}
		defer rf.Close()
		_, err = io.CopyN(hash, rf, resumeOffset)
		if err != nil {
			return nil, err
		}
	}
	return hash, nil
}

func setupWriter(out *os.File, hash *blake3.Hasher, bar progressbar.PB, resp *http.Response, resumeOffset int64) io.Writer {
	if bar != nil {
		writer := io.MultiWriter(out, hash, bar)
		bar.UpdateRange(0, resp.ContentLength+resumeOffset)
		bar.SetInitialValue(resumeOffset)
		return writer
	}
	return io.MultiWriter(out, hash)
}

func copyWithInterruption(ctx context.Context, writer io.Writer, reader io.Reader, hash *blake3.Hasher, tempFile string, isOCI bool, startOffset int64) (int64, error) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	written := startOffset
	buf := make([]byte, 64*1024)

	for {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		case <-sigCh:
			if isOCI {
				setOCIMeta(tempFile, written, hex.EncodeToString(hash.Sum(nil)))
			}
			os.Exit(130)
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			if _, errw := writer.Write(buf[:n]); errw != nil {
				return written, errDownloadFailed.Wrap(errw)
			}
			written += int64(n)
			if isOCI && written%524288 == 0 {
				if err := setOCIMeta(tempFile, written, hex.EncodeToString(hash.Sum(nil))); err != nil {
					return written, errDownloadFailed.Wrap(err)
				}
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			if isOCI {
				setOCIMeta(tempFile, written, hex.EncodeToString(hash.Sum(nil)))
			}
			return written, errDownloadFailed.Wrap(err)
		}
	}

	return written, nil
}

func cleanupMetadata(tempFile string, isOCI bool) error {
	if isOCI {
		xattr.Remove(tempFile, "user.dbin.ocichunk")
		return nil
	}
	xattr.Remove(tempFile, "user.dbin.lastmod")
	return nil
}

func verifyChecksum(hash *blake3.Hasher, bEntry *binaryEntry, destination string) error {
	if bEntry.Bsum != "" && bEntry.Bsum != "!no_check" {
		calculatedChecksum := hex.EncodeToString(hash.Sum(nil))
		if calculatedChecksum != bEntry.Bsum {
			fmt.Fprintf(os.Stderr, "expected %s, got %s\n", bEntry.Bsum, calculatedChecksum)
			//os.Remove(destination)
			//return errChecksumMismatch.New("expected %s, got %s", bEntry.Bsum, calculatedChecksum)
		}
	}
	return nil
}

func validateFileType(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return errFileTypeInvalid.Wrap(err)
	}
	defer file.Close()

	buf := make([]byte, 128)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return errFileTypeInvalid.Wrap(err)
	}

	// Check for ELF
	if n >= 4 && string(buf[:4]) == "\x7fELF" {
		return nil
	}

	content := string(buf[:n])
	// Check for bad responses
	if strings.HasPrefix(content, "<!DOCTYPE html>") || strings.HasPrefix(content, "<html>") {
		return errFileTypeInvalid.New("file looks like HTML: %s", strings.TrimLeft(content, " \t\r\n"))
	}
	// Check for Nix Objects/Nix Garbage
	if strings.HasPrefix(content, "#!") {
		firstLine := content
		if i := strings.IndexByte(content, '\n'); i >= 0 {
			firstLine = content[:i]
		}
		if regexp.MustCompile(`^#!\s*/nix/store/[^/]+/`).MatchString(firstLine) {
			return errFileTypeInvalid.New("file contains invalid shebang (nix object/garbage): [%s]", firstLine)
		}
		if strings.Count(content, "\n") < 5 {
			return errFileTypeInvalid.New("file with shebang is less than 5 lines long. (nix object/garbage): \n---\n%s\n---", content)
		}
		return nil
	}

	return errFileTypeInvalid.New("file is neither a shell script nor an ELF. Please report this at @ https://github.com/xplshn/dbin")
}

func verifySignature(binaryPath string, sigData []byte, bEntry *binaryEntry, cfg *config) error {
	pubKeyURL := bEntry.Repository.PubKeys[bEntry.Repository.Name]
	if pubKeyURL == "" {
		return nil
	}

	pubKeyData, err := accessCachedOrFetch([]string{pubKeyURL}, bEntry.Repository.Name+".minisign", cfg, bEntry.Repository.SyncInterval)
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

	binaryData, err := os.ReadFile(binaryPath)
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

func createHTTPRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Expires", "0")
	req.Header.Set("User-Agent", fmt.Sprintf("dbin/%.1f", version))

	return req, nil
}

func fetchBinaryFromURLToDest(ctx context.Context, bar progressbar.PB, bEntry *binaryEntry, destination string, cfg *config) error {
	if strings.HasPrefix(bEntry.DownloadURL, "oci://") {
		bEntry.DownloadURL = strings.TrimPrefix(bEntry.DownloadURL, "oci://")
		return fetchOCIImage(ctx, bar, bEntry, destination, cfg)
	}

	client := &http.Client{}

	// Check for signature and license file existence
	hasSignature, hasLicense, err := httpCheckSignatureAndLicense(ctx, client, bEntry.DownloadURL)
	if err != nil {
		return errDownloadFailed.Wrap(err)
	}

	resumeOffset, lastModified, err := checkPartialDownload(destination + ".tmp")
	if err != nil {
		return errDownloadFailed.Wrap(err)
	}

	// Validate resume capability
	if err := validateResume(ctx, client, bEntry.DownloadURL); err != nil {
		return err
	}

	resp, actualOffset, err := createDownloadRequest(ctx, client, bEntry.DownloadURL, resumeOffset, lastModified)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := downloadWithProgress(ctx, bar, resp, destination, bEntry, false, resp.Header.Get("Last-Modified"), actualOffset); err != nil {
		return err
	}

	// Handle signature verification if signature exists
	if hasSignature {
		if err := handleSignatureVerification(bEntry, destination, cfg); err != nil {
			return err
		}
	}

	// Handle license file download if license exists and CreateLicenses is enabled
	if hasLicense && cfg.CreateLicenses {
		licenseDest := filepath.Join(cfg.LicenseDir, filepath.Base(destination)+".LICENSE")
		licenseResp, err := http.Get(bEntry.DownloadURL + ".LICENSE")
		if err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "Warning: Failed to fetch license file for %s: %v\n", destination, err)
			}
			return nil
		}
		defer licenseResp.Body.Close()

		if licenseResp.StatusCode != http.StatusOK {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "Warning: License file request returned status %d\n", licenseResp.StatusCode)
			}
			return nil
		}

		if err := saveLicenseFile(ctx, licenseResp, licenseDest); err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "Warning: Failed to save license file for %s: %v\n", destination, err)
			}
			return nil
		}

		xattr.Set(licenseDest, "user.dbin.binary", []byte(destination))
		xattr.Set(destination, "user.dbin.license", []byte(licenseDest))

		if verbosityLevel >= extraVerbose {
			fmt.Printf("Saved license file for %s to %s\n", destination, licenseDest)
		}
	}

	return nil
}

func checkPartialDownload(tempFile string) (int64, string, error) {
	fi, err := os.Stat(tempFile)
	if err != nil {
		return 0, "", nil // No partial download
	}

	resumeOffset := fi.Size()
	var lastModified string
	if modTimeBytes, err := xattr.Get(tempFile, "user.dbin.lastmod"); err == nil {
		lastModified = string(modTimeBytes)
	}

	return resumeOffset, lastModified, nil
}

func validateResume(ctx context.Context, client *http.Client, url string) error {
	req, err := createHTTPRequest(ctx, "HEAD", url)
	if err != nil {
		return errDownloadFailed.Wrap(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return errDownloadFailed.Wrap(err)
	}
	resp.Body.Close()

	return nil
}

func createDownloadRequest(ctx context.Context, client *http.Client, url string, resumeOffset int64, lastModified string) (*http.Response, int64, error) {
	req, err := createHTTPRequest(ctx, "GET", url)
	if err != nil {
		return nil, 0, errDownloadFailed.Wrap(err)
	}

	if resumeOffset > 0 {
		if lastModified != "" {
			req.Header.Set("If-Range", lastModified)
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, errDownloadFailed.Wrap(err)
	}

	actualOffset := resumeOffset
	// Reset if server sends full file instead of range
	if resumeOffset > 0 && resp.StatusCode == http.StatusOK {
		os.Remove(req.URL.Path + ".tmp")
		actualOffset = 0
	}

	return resp, actualOffset, nil
}

func handleSignatureVerification(bEntry *binaryEntry, destination string, cfg *config) error {
	pubKeyURL := bEntry.Repository.PubKeys[bEntry.Repository.Name]
	if pubKeyURL == "" {
		return nil
	}

	sigResp, err := http.Get(bEntry.DownloadURL + ".sig")
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
		return err
	}

	return nil
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
		return err
	}

	manifest, err := downloadManifest(ctx, registry, repository, tag, token)
	if err != nil {
		return err
	}

	title := filepath.Base(destination)
	binaryResp, sigResp, licenseResp, err := downloadOCILayer(ctx, registry, repository, manifest, token, title, destination+".tmp", cfg)
	if err != nil {
		return err
	}
	defer closeResponses(binaryResp, sigResp, licenseResp)

	if err := downloadWithProgress(ctx, bar, binaryResp, destination, bEntry, true, "", 0); err != nil {
		return err
	}

	if err := handleOCISignature(bEntry, destination, cfg, sigResp); err != nil {
		return err
	}

	return handleOCILicense(cfg, licenseResp, title, destination)
}

func closeResponses(responses ...*http.Response) {
	for _, resp := range responses {
		if resp != nil {
			resp.Body.Close()
		}
	}
}

func handleOCISignature(bEntry *binaryEntry, destination string, cfg *config, sigResp *http.Response) error {
	if bEntry.Repository.PubKeys[bEntry.Repository.Name] == "" || sigResp == nil {
		return nil
	}

	sigData, err := io.ReadAll(sigResp.Body)
	if err != nil {
		return errSignatureVerify.Wrap(err)
	}

	if err := verifySignature(destination, sigData, bEntry, cfg); err != nil {
		os.Remove(destination)
		return err
	}

	return nil
}

func handleOCILicense(cfg *config, licenseResp *http.Response, title, destination string) error {
	if !cfg.CreateLicenses || licenseResp == nil {
		return nil
	}

	licenseDest := filepath.Join(cfg.LicenseDir, title+".LICENSE")
	if err := saveLicenseFile(context.Background(), licenseResp, licenseDest); err != nil {
		if verbosityLevel >= silentVerbosityWithErrors {
			fmt.Fprintf(os.Stderr, "Warning: Failed to save license file for %s: %v\n", title, err)
		}
		return nil
	}

	if verbosityLevel >= extraVerbose {
		fmt.Printf("Saved license file for %s to %s\n", title, licenseDest)
		xattr.Set(licenseDest, "user.dbin.binary", []byte(destination))
		xattr.Set(destination, "user.dbin.license", []byte(licenseDest))
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

func saveLicenseFile(ctx context.Context, resp *http.Response, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return errDownloadFailed.Wrap(err)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return errDownloadFailed.Wrap(err)
	}

	tempFile := destination + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return errDownloadFailed.Wrap(err)
	}

	if err := os.Rename(tempFile, destination); err != nil {
		return errDownloadFailed.Wrap(err)
	}

	return os.Chmod(destination, 0644)
}

func downloadOCILayer(ctx context.Context, registry, repository string, manifest map[string]interface{}, token, title, tmpPath string, cfg *config) (*http.Response, *http.Response, *http.Response, error) {
	titleNoExt := strings.TrimSuffix(title, filepath.Ext(title))
	layers, ok := manifest["layers"].([]interface{})
	if !ok {
		return nil, nil, nil, errOCILayerDownload.New("invalid manifest structure")
	}

	digests := findLayerDigests(layers, title, titleNoExt)
	if digests.binary == "" {
		return nil, nil, nil, errOCILayerDownload.New("file with title '%s' not found in manifest", title)
	}

	binaryResp, err := downloadOCIBlob(ctx, registry, repository, digests.binary, token, tmpPath)
	if err != nil {
		return nil, nil, nil, err
	}

	var sigResp, licenseResp *http.Response
	if digests.signature != "" {
		sigResp, err = downloadOCIBlob(ctx, registry, repository, digests.signature, token, "")
		if err != nil {
			binaryResp.Body.Close()
			return nil, nil, nil, err
		}
	}

	if cfg.CreateLicenses && digests.license != "" {
		licenseResp, err = downloadOCIBlob(ctx, registry, repository, digests.license, token, "")
		if err != nil {
			closeResponses(binaryResp, sigResp)
			return nil, nil, nil, err
		}
	}

	return binaryResp, sigResp, licenseResp, nil
}

type layerDigests struct {
	binary, signature, license string
}

func findLayerDigests(layers []interface{}, title, titleNoExt string) layerDigests {
	var digests layerDigests

	for _, layer := range layers {
		layerMap, ok := layer.(map[string]interface{})
		if !ok {
			continue
		}
		annotations, ok := layerMap["annotations"].(map[string]interface{})
		if !ok {
			continue
		}
		layerTitle, ok := annotations["org.opencontainers.image.title"].(string)
		if !ok {
			continue
		}

		digest := layerMap["digest"].(string)
		switch layerTitle {
		case title, titleNoExt:
			digests.binary = digest
		case title+".sig", titleNoExt+".sig":
			digests.signature = digest
		case "LICENSE", titleNoExt+".LICENSE":
			digests.license = digest
		}
	}

	return digests
}

func downloadOCIBlob(ctx context.Context, registry, repository, digest, token, tmpPath string) (*http.Response, error) {
	url := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repository, digest)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errOCILayerDownload.Wrap(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	if tmpPath != "" {
		if meta, err := getOCIMeta(tmpPath); err == nil && meta.Offset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", meta.Offset))
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errOCILayerDownload.Wrap(err)
	}

	return resp, nil
}

func cleanInstallCache(cfg *config) error {
	targets := []struct {
		dir       string
		threshold time.Duration
	}{
		{cfg.InstallDir, 24 * time.Hour},
		{cfg.LicenseDir, 10 * time.Second},
	}

	now := time.Now()
	for _, target := range targets {
		if err := cleanDirectory(target.dir, target.threshold, now); err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "Error cleaning directory %s: %v\n", target.dir, err)
			}
		}
	}

	return nil
}

func cleanDirectory(dir string, threshold time.Duration, now time.Time) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		if err := removeOldTempFile(filePath, threshold, now); err != nil {
			if verbosityLevel >= silentVerbosityWithErrors {
				fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", filePath, err)
			}
		}
	}

	return nil
}

func removeOldTempFile(filePath string, threshold time.Duration, now time.Time) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		if verbosityLevel >= extraVerbose {
			fmt.Fprintf(os.Stderr, "No ATime for %s, skipping\n", filePath)
		}
		return nil
	}

	if now.Sub(ATime(stat)) > threshold {
		if err := os.Remove(filePath); err != nil {
			return err
		}
		if verbosityLevel >= extraVerbose {
			fmt.Printf("Removed .tmp file: %s\n", filePath)
		}
	}

	return nil
}


func httpCheckSignatureAndLicense(ctx context.Context, client *http.Client, url string) (hasSignature, hasLicense bool, err error) {
	// Check for signature file (.sig)
	sigReq, err := createHTTPRequest(ctx, "HEAD", url+".sig")
	if err != nil {
		// Only return error if we can't create the request
		return false, false, errDownloadFailed.Wrap(err)
	}

	sigResp, err := client.Do(sigReq)
	if err != nil {
		// HTTP request failed - treat as no signature file available
		hasSignature = false
	} else {
		sigResp.Body.Close()
		hasSignature = sigResp.StatusCode == http.StatusOK
	}

	// Check for license file (.LICENSE)
	licenseReq, err := createHTTPRequest(ctx, "HEAD", url+".LICENSE")
	if err != nil {
		// Only return error if we can't create the request
		return hasSignature, false, errDownloadFailed.Wrap(err)
	}

	licenseResp, err := client.Do(licenseReq)
	if err != nil {
		// HTTP request failed - treat as no license file available
		hasLicense = false
	} else {
		licenseResp.Body.Close()
		hasLicense = licenseResp.StatusCode == http.StatusOK
	}

	return hasSignature, hasLicense, nil
}
