package client

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Downloader handles downloading and installing OpenShift client tools
type Downloader struct {
	OCPVersion   string
	BaseURL      string
	BinDir       string
	DownloadDir  string
	Arch         string
	OS           string
	RHELVersion  string
	HTTPClient   *http.Client
	mu           sync.Mutex
	progressFunc func(tool string, downloaded, total int64)
}

// Tool represents a client tool to download
type Tool struct {
	Name         string
	DownloadPath string
	ExtractPath  string
	BinaryName   string
}

// DownloadResult represents the result of a download operation
type DownloadResult struct {
	Tool    string
	Success bool
	Version string
	Path    string
	Error   error
}

// NewDownloader creates a new downloader instance
func NewDownloader(ocpVersion, binDir string) (*Downloader, error) {
	arch, osName, rhelVersion, err := detectSystem()
	if err != nil {
		return nil, fmt.Errorf("failed to detect system: %w", err)
	}

	// Create directories
	downloadDir := filepath.Join(binDir, "downloads")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bin directory: %w", err)
	}
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create download directory: %w", err)
	}

	return &Downloader{
		OCPVersion:  ocpVersion,
		BaseURL:     "https://mirror.openshift.com/pub/openshift-v4/x86_64/clients",
		BinDir:      binDir,
		DownloadDir: downloadDir,
		Arch:        arch,
		OS:          osName,
		RHELVersion: rhelVersion,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Minute,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost:  5,
				IdleConnTimeout:      90 * time.Second,
				DisableCompression:   false,
			},
		},
	}, nil
}

// SetProgressFunc sets a callback function for download progress
func (d *Downloader) SetProgressFunc(fn func(tool string, downloaded, total int64)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.progressFunc = fn
}

// detectSystem detects the system architecture, OS, and RHEL version
func detectSystem() (arch, osName, rhelVersion string, err error) {
	// Detect architecture
	arch = runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return "", "", "", fmt.Errorf("unsupported architecture: %s", arch)
	}

	// Detect OS
	osName = runtime.GOOS
	switch osName {
	case "linux":
		osName = "linux"
	case "darwin":
		osName = "mac"
	default:
		return "", "", "", fmt.Errorf("unsupported OS: %s", osName)
	}

	// Detect RHEL version (Linux only)
	rhelVersion = "rhel9" // Default
	if osName == "linux" {
		rhelVersion = detectRHELVersion()
	}

	return arch, osName, rhelVersion, nil
}

// detectRHELVersion detects RHEL version from /etc/os-release or /etc/redhat-release
func detectRHELVersion() string {
	// Try /etc/os-release first
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		content := string(data)
		lines := strings.Split(content, "\n")
		
		var id, idLike, versionID string
		for _, line := range lines {
			if strings.HasPrefix(line, "ID=") {
				id = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
			} else if strings.HasPrefix(line, "ID_LIKE=") {
				idLike = strings.Trim(strings.TrimPrefix(line, "ID_LIKE="), "\"")
			} else if strings.HasPrefix(line, "VERSION_ID=") {
				versionID = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
			}
		}

		// Check for RHEL or Fedora
		if id == "rhel" || strings.Contains(idLike, "rhel") || strings.Contains(idLike, "fedora") {
			if versionID != "" {
				parts := strings.Split(versionID, ".")
				if len(parts) > 0 {
					majorVersion := parts[0]
					if majorVersion >= "9" {
						return "rhel9"
					} else if majorVersion == "8" {
						return "rhel8"
					}
				}
			}
		} else if id == "fedora" {
			if versionID != "" {
				if versionID >= "38" {
					return "rhel9"
				}
				return "rhel8"
			}
		}
	}

	// Fallback to /etc/redhat-release
	if data, err := os.ReadFile("/etc/redhat-release"); err == nil {
		content := strings.ToLower(string(data))
		if strings.Contains(content, "release 9") {
			return "rhel9"
		} else if strings.Contains(content, "release 8") {
			return "rhel8"
		}
	}

	return "rhel9" // Default
}

// DownloadAll downloads all client tools concurrently
func (d *Downloader) DownloadAll(ctx context.Context, tools []string) ([]DownloadResult, error) {
	var wg sync.WaitGroup
	results := make([]DownloadResult, len(tools))
	resultChan := make(chan DownloadResult, len(tools))

	// Download tools concurrently
	for i, toolName := range tools {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			result := d.DownloadTool(ctx, name)
			resultChan <- result
		}(i, toolName)
	}

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	idx := 0
	for result := range resultChan {
		results[idx] = result
		idx++
	}

	return results, nil
}

// DownloadTool downloads a specific tool
func (d *Downloader) DownloadTool(ctx context.Context, toolName string) DownloadResult {
	result := DownloadResult{
		Tool: toolName,
	}

	// Check if tool already exists
	toolPath := filepath.Join(d.BinDir, toolName)
	if info, err := os.Stat(toolPath); err == nil && info.Mode().IsRegular() {
		// Tool exists, verify it
		if version, err := d.verifyTool(toolPath, toolName); err == nil {
			result.Success = true
			result.Version = version
			result.Path = toolPath
			return result
		}
	}

	// Determine download URL based on tool
	var downloadURL string
	var extractBinaryName string

	switch toolName {
	case "oc":
		downloadURL = fmt.Sprintf("%s/ocp/stable-%s/openshift-client-%s-%s-%s.tar.gz",
			d.BaseURL, d.OCPVersion, d.OS, d.Arch, d.RHELVersion)
		extractBinaryName = "oc"
	case "opm":
		downloadURL = fmt.Sprintf("%s/ocp/stable-%s/opm-%s-%s.tar.gz",
			d.BaseURL, d.OCPVersion, d.OS, d.RHELVersion)
		extractBinaryName = "opm"
	case "oc-mirror":
		downloadURL = fmt.Sprintf("%s/ocp/stable-%s/oc-mirror.tar.gz",
			d.BaseURL, d.OCPVersion)
		extractBinaryName = "oc-mirror"
	default:
		result.Error = fmt.Errorf("unknown tool: %s", toolName)
		return result
	}

	// Try fallback URLs if primary fails
	fallbackURLs := []string{
		downloadURL,
		fmt.Sprintf("%s/ocp/latest/%s", d.BaseURL, filepath.Base(downloadURL)),
	}

	var downloadErr error
	for _, url := range fallbackURLs {
		if err := d.downloadAndExtract(ctx, url, toolName, extractBinaryName); err != nil {
			downloadErr = err
			continue
		}

		// Verify installation
		if version, err := d.verifyTool(toolPath, toolName); err == nil {
			result.Success = true
			result.Version = version
			result.Path = toolPath
			return result
		}
	}

	result.Error = fmt.Errorf("failed to download %s: %w", toolName, downloadErr)
	return result
}

// downloadAndExtract downloads and extracts a tool
func (d *Downloader) downloadAndExtract(ctx context.Context, url, toolName, extractBinaryName string) error {
	tempFile := filepath.Join(d.DownloadDir, fmt.Sprintf("%s.tar.gz", toolName))
	defer os.Remove(tempFile)

	// Download file
	if err := d.downloadFile(ctx, url, tempFile, toolName); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Extract binary
	if err := d.extractBinary(tempFile, extractBinaryName, toolName); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	return nil
}

// downloadFile downloads a file with progress reporting
func (d *Downloader) downloadFile(ctx context.Context, url, destPath, toolName string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Copy with progress reporting using io.Copy for better performance
	total := resp.ContentLength
	var downloaded int64
	
	// Use io.Copy with custom writer for progress tracking
	writer := &progressWriter{
		writer: file,
		onWrite: func(n int64) {
			downloaded += n
			if d.progressFunc != nil {
				d.progressFunc(toolName, downloaded, total)
			}
		},
	}

	// Copy in background with context cancellation
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(writer, resp.Body)
		done <- err
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return err
		}
	}

	return nil
}

// extractBinary extracts a binary from a tar.gz file
func (d *Downloader) extractBinary(tarPath, extractBinaryName, toolName string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var found bool
	var binaryData []byte
	var binarySize int64

	// Possible binary names to look for
	possibleNames := []string{
		extractBinaryName,
		fmt.Sprintf("%s-%s", extractBinaryName, d.RHELVersion),
		"oc-mirror-rhel9",
		"oc-mirror-rhel8",
		"opm-rhel9",
		"opm-rhel8",
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Look for the binary or RHEL-specific variants
		name := filepath.Base(header.Name)
		for _, possibleName := range possibleNames {
			if name == possibleName {
				if header.Typeflag == tar.TypeReg {
					binarySize = header.Size
					// Pre-allocate buffer for better performance
					binaryData = make([]byte, binarySize)
					
					// Read in chunks for better memory management
					var totalRead int64
					for totalRead < binarySize {
						n, err := tr.Read(binaryData[totalRead:])
						if err != nil && err != io.EOF {
							return err
						}
						if n == 0 {
							break
						}
						totalRead += int64(n)
					}
					
					found = true
					break
				}
			}
		}
		if found {
			break
		}
	}

	if !found {
		return fmt.Errorf("binary %s not found in archive", extractBinaryName)
	}

	// Write binary to destination
	destPath := filepath.Join(d.BinDir, toolName)
	if err := os.WriteFile(destPath, binaryData, 0755); err != nil {
		return err
	}

	return nil
}

// progressWriter wraps an io.Writer to track progress
type progressWriter struct {
	writer  io.Writer
	onWrite func(int64)
}

func (pw *progressWriter) Write(p []byte) (n int, err error) {
	n, err = pw.writer.Write(p)
	if pw.onWrite != nil {
		pw.onWrite(int64(n))
	}
	return n, err
}

// verifyTool verifies a tool installation by running version command
func (d *Downloader) verifyTool(toolPath, toolName string) (string, error) {
	// Check if file exists and is executable
	info, err := os.Stat(toolPath)
	if err != nil {
		return "", err
	}
	if info.Mode().IsDir() {
		return "", fmt.Errorf("path is a directory, not a file")
	}
	if info.Mode()&0111 == 0 {
		// Make executable
		if err := os.Chmod(toolPath, 0755); err != nil {
			return "", fmt.Errorf("failed to make executable: %w", err)
		}
	}

	var cmd *exec.Cmd
	switch toolName {
	case "oc":
		cmd = exec.Command(toolPath, "version", "--client")
	case "opm", "oc-mirror":
		cmd = exec.Command(toolPath, "version")
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	version := strings.TrimSpace(string(output))
	if version == "" {
		version = "unknown"
	}

	return version, nil
}

// CheckToolInPath checks if a tool is available in PATH
func CheckToolInPath(toolName string) (string, error) {
	path, err := exec.LookPath(toolName)
	if err != nil {
		return "", err
	}
	return path, nil
}

// Cleanup removes temporary download directory
func (d *Downloader) Cleanup() error {
	return os.RemoveAll(d.DownloadDir)
}

