package command

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// OCMirrorCommand wraps oc-mirror CLI execution
type OCMirrorCommand struct {
	v2              bool
	config          string
	output          string
	from            string
	cacheDir        string
	skipMissing     bool
	continueOnError bool
	skipTLS         bool
}

// CommandOutput contains the output from oc-mirror execution
type CommandOutput struct {
	Logs     []string
	Stdout   string
	Stderr   string
	ExitCode int
}

// NewOCMirrorCommand creates a new oc-mirror command wrapper
func NewOCMirrorCommand() *OCMirrorCommand {
	return &OCMirrorCommand{
		v2: false,
	}
}

// SetV2 sets the v2 flag
func (cmd *OCMirrorCommand) SetV2(v2 bool) {
	cmd.v2 = v2
}

// SetConfig sets the config file path
func (cmd *OCMirrorCommand) SetConfig(config string) {
	cmd.config = config
}

// SetOutput sets the output destination
func (cmd *OCMirrorCommand) SetOutput(output string) {
	cmd.output = output
}

// SetFrom sets the --from flag
func (cmd *OCMirrorCommand) SetFrom(from string) {
	cmd.from = from
}

// SetCacheDir sets the cache directory
func (cmd *OCMirrorCommand) SetCacheDir(cacheDir string) {
	cmd.cacheDir = cacheDir
}

// SetSkipMissing sets skip-missing flag
func (cmd *OCMirrorCommand) SetSkipMissing(skip bool) {
	cmd.skipMissing = skip
}

// SetContinueOnError sets continue-on-error flag
func (cmd *OCMirrorCommand) SetContinueOnError(continueOn bool) {
	cmd.continueOnError = continueOn
}

// SetSkipTLS sets the skip TLS verification flag (--dest-tls-verify=false)
func (cmd *OCMirrorCommand) SetSkipTLS(skip bool) {
	cmd.skipTLS = skip
}

// Execute runs the oc-mirror command
// Execute runs the oc-mirror command and returns the output
func (cmd *OCMirrorCommand) Execute() (*CommandOutput, error) {
	return cmd.ExecuteWithCallback(nil)
}

// ExecuteWithCallback runs the oc-mirror command with a callback that receives the child PID
// The callback is called immediately after the process starts, allowing external monitoring
func (cmd *OCMirrorCommand) ExecuteWithCallback(onStart func(pid int)) (*CommandOutput, error) {
	args := cmd.buildArgs()

	fmt.Printf("Executing: oc-mirror %s\n", strings.Join(args, " "))

	execCmd := exec.Command("oc-mirror", args...)

	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	// Use Start/Wait to get the PID for external monitoring
	if err := execCmd.Start(); err != nil {
		return &CommandOutput{
			Stdout:   "",
			Stderr:   err.Error(),
			ExitCode: -1,
		}, fmt.Errorf("failed to start oc-mirror: %w", err)
	}

	// Call the callback with the child process PID if provided
	if onStart != nil && execCmd.Process != nil {
		onStart(execCmd.Process.Pid)
	}

	// Wait for the command to complete
	err := execCmd.Wait()

	output := &CommandOutput{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	if execCmd.ProcessState != nil {
		output.ExitCode = execCmd.ProcessState.ExitCode()
	}

	// Combine stdout and stderr for log parsing
	combinedOutput := stdout.String() + "\n" + stderr.String()
	output.Logs = strings.Split(combinedOutput, "\n")

	if err != nil {
		return output, fmt.Errorf("oc-mirror command failed: %w\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
	}

	return output, nil
}

func (cmd *OCMirrorCommand) buildArgs() []string {
	args := []string{}

	if cmd.v2 {
		args = append(args, "--v2")
		// v2-specific flags
		if cmd.cacheDir != "" {
			args = append(args, "--cache-dir", cmd.cacheDir)
		}
	} else {
		// v1 doesn't support --cache-dir flag
		// v1 is deprecated but we still support it for comparison
		// v1 supports --skip-missing and --continue-on-error
		if cmd.skipMissing {
			args = append(args, "--skip-missing")
		}
		if cmd.continueOnError {
			args = append(args, "--continue-on-error")
		}
	}

	if cmd.config != "" {
		if cmd.v2 {
			args = append(args, "-c", cmd.config)
		} else {
			// v1 uses --config flag
			args = append(args, "--config", cmd.config)
		}
	}

	if cmd.from != "" {
		args = append(args, "--from", cmd.from)
	}

	if cmd.skipTLS {
		if cmd.v2 {
			args = append(args, "--dest-tls-verify=false")
		} else {
			args = append(args, "--dest-skip-tls=true")
		}
	}

	if cmd.output != "" {
		args = append(args, cmd.output)
	}

	return args
}

// CountSkippedImages counts images skipped due to cache
func (out *CommandOutput) CountSkippedImages() int {
	count := 0
	skipPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)skipped.*image`),
		regexp.MustCompile(`(?i)image.*skipped`),
		regexp.MustCompile(`(?i)already.*exists`),
		regexp.MustCompile(`(?i)using.*cached`),
	}

	for _, line := range out.Logs {
		for _, pattern := range skipPatterns {
			if pattern.MatchString(line) {
				count++
				break
			}
		}
	}

	return count
}

// CountCacheHits counts cache hit messages in logs
func (out *CommandOutput) CountCacheHits() int {
	count := 0
	cachePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)cache.*hit`),
		regexp.MustCompile(`(?i)using.*cache`),
		regexp.MustCompile(`(?i)cached.*image`),
		regexp.MustCompile(`(?i)found.*cache`),
	}

	for _, line := range out.Logs {
		for _, pattern := range cachePatterns {
			if pattern.MatchString(line) {
				count++
				break
			}
		}
	}

	return count
}

// ExtractBytesUploaded extracts bytes uploaded from logs
func (out *CommandOutput) ExtractBytesUploaded() int64 {
	// Patterns to match bytes uploaded
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(\d+)\s*(?:bytes|B)\s*(?:uploaded|transferred|sent)`),
		regexp.MustCompile(`(?i)uploaded.*?(\d+)\s*(?:bytes|B)`),
		regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(?:MB|GB|KB)`),
		regexp.MustCompile(`(?i)transferred.*?(\d+)\s*(?:bytes|B)`),
	}

	var totalBytes int64

	for _, line := range out.Logs {
		for _, pattern := range patterns {
			matches := pattern.FindStringSubmatch(line)
			if len(matches) > 1 {
				// Try to extract number
				var bytes int64
				fmt.Sscanf(matches[1], "%d", &bytes)

				// Check if it's MB/GB/KB and convert
				if strings.Contains(strings.ToLower(line), "mb") {
					bytes *= 1024 * 1024
				} else if strings.Contains(strings.ToLower(line), "gb") {
					bytes *= 1024 * 1024 * 1024
				} else if strings.Contains(strings.ToLower(line), "kb") {
					bytes *= 1024
				}

				if bytes > totalBytes {
					totalBytes = bytes
				}
			}
		}
	}

	// If we couldn't extract from logs, try to get from registry logs or docker stats
	if totalBytes == 0 {
		totalBytes = out.estimateBytesFromLogs()
	}

	return totalBytes
}

func (out *CommandOutput) estimateBytesFromLogs() int64 {
	// Fallback estimation - look for image size patterns
	sizePattern := regexp.MustCompile(`(?i)size[:\s]+(\d+(?:\.\d+)?)\s*(MB|GB|KB|bytes?)`)
	var totalBytes int64

	for _, line := range out.Logs {
		matches := sizePattern.FindStringSubmatch(line)
		if len(matches) >= 3 {
			var size float64
			fmt.Sscanf(matches[1], "%f", &size)

			unit := strings.ToLower(matches[2])
			var bytes int64

			switch {
			case strings.Contains(unit, "gb"):
				bytes = int64(size * 1024 * 1024 * 1024)
			case strings.Contains(unit, "mb"):
				bytes = int64(size * 1024 * 1024)
			case strings.Contains(unit, "kb"):
				bytes = int64(size * 1024)
			default:
				bytes = int64(size)
			}

			totalBytes += bytes
		}
	}

	return totalBytes
}

// ExtendedMetrics contains all extracted metrics from logs
type ExtendedMetrics struct {
	ImagesProcessed    int
	ImagesCopied       int
	ImagesSkipped      int
	LayersProcessed    int
	LayersCopied       int
	LayersSkipped      int
	ManifestsProcessed int
	BlobsProcessed     int
	ErrorCount         int
	RetryCount         int
	WarningCount       int
	Errors             []string
	Warnings           []string
	OperatorsFound     []string
	CatalogsMirrored   int
}

// ExtractExtendedMetrics extracts comprehensive metrics from command output
func (out *CommandOutput) ExtractExtendedMetrics() ExtendedMetrics {
	metrics := ExtendedMetrics{
		Errors:         make([]string, 0),
		Warnings:       make([]string, 0),
		OperatorsFound: make([]string, 0),
	}

	// Patterns for counting
	imagePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)copying\s+image`),
		regexp.MustCompile(`(?i)mirroring\s+image`),
		regexp.MustCompile(`(?i)processing\s+image`),
		regexp.MustCompile(`(?i)image.*copied`),
	}

	layerPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)copying\s+blob`),
		regexp.MustCompile(`(?i)layer\s+sha256`),
		regexp.MustCompile(`(?i)blob\s+sha256`),
		regexp.MustCompile(`(?i)uploading.*blob`),
	}

	manifestPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)copying\s+manifest`),
		regexp.MustCompile(`(?i)manifest.*copied`),
		regexp.MustCompile(`(?i)writing\s+manifest`),
	}

	errorPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)^error:`),
		regexp.MustCompile(`(?i)\berror\b.*:`),
		regexp.MustCompile(`(?i)failed\s+to`),
		regexp.MustCompile(`(?i)unable\s+to`),
	}

	retryPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)retry`),
		regexp.MustCompile(`(?i)retrying`),
		regexp.MustCompile(`(?i)attempt\s+\d+`),
	}

	warningPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)^warn`),
		regexp.MustCompile(`(?i)^W\d+`),
		regexp.MustCompile(`(?i)warning:`),
	}

	skipPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)skipping`),
		regexp.MustCompile(`(?i)already\s+exists`),
		regexp.MustCompile(`(?i)exists.*skipping`),
	}

	operatorPattern := regexp.MustCompile(`(?i)operator[:\s]+([a-zA-Z0-9_-]+)`)
	catalogPattern := regexp.MustCompile(`(?i)catalog.*mirrored|mirroring.*catalog`)

	for _, line := range out.Logs {
		// Count images
		for _, p := range imagePatterns {
			if p.MatchString(line) {
				metrics.ImagesProcessed++
				if !containsSkip(line) {
					metrics.ImagesCopied++
				}
				break
			}
		}

		// Count layers/blobs
		for _, p := range layerPatterns {
			if p.MatchString(line) {
				metrics.LayersProcessed++
				if !containsSkip(line) {
					metrics.LayersCopied++
				} else {
					metrics.LayersSkipped++
				}
				break
			}
		}

		// Count manifests
		for _, p := range manifestPatterns {
			if p.MatchString(line) {
				metrics.ManifestsProcessed++
				break
			}
		}

		// Count blobs
		if strings.Contains(strings.ToLower(line), "blob") {
			metrics.BlobsProcessed++
		}

		// Count errors
		for _, p := range errorPatterns {
			if p.MatchString(line) {
				metrics.ErrorCount++
				metrics.Errors = append(metrics.Errors, truncateString(line, 200))
				break
			}
		}

		// Count retries
		for _, p := range retryPatterns {
			if p.MatchString(line) {
				metrics.RetryCount++
				break
			}
		}

		// Count warnings
		for _, p := range warningPatterns {
			if p.MatchString(line) {
				metrics.WarningCount++
				if len(metrics.Warnings) < 20 { // Limit stored warnings
					metrics.Warnings = append(metrics.Warnings, truncateString(line, 200))
				}
				break
			}
		}

		// Count skipped
		for _, p := range skipPatterns {
			if p.MatchString(line) {
				if strings.Contains(strings.ToLower(line), "image") {
					metrics.ImagesSkipped++
				}
				break
			}
		}

		// Extract operator names
		if matches := operatorPattern.FindStringSubmatch(line); len(matches) > 1 {
			opName := matches[1]
			if !containsString(metrics.OperatorsFound, opName) {
				metrics.OperatorsFound = append(metrics.OperatorsFound, opName)
			}
		}

		// Count catalogs
		if catalogPattern.MatchString(line) {
			metrics.CatalogsMirrored++
		}
	}

	return metrics
}

// PrintSummary prints a summary of extended metrics
func (m *ExtendedMetrics) PrintSummary() {
	fmt.Printf("  │ ─── Image/Layer Metrics ──────────────────────────────────────\n")
	// Only print if we have non-zero values (log parsing often doesn't capture these)
	if m.ImagesProcessed > 0 || m.ImagesCopied > 0 || m.ImagesSkipped > 0 {
		fmt.Printf("  │   Images: %d processed | %d copied | %d skipped\n",
			m.ImagesProcessed, m.ImagesCopied, m.ImagesSkipped)
	}
	if m.LayersProcessed > 0 || m.LayersCopied > 0 || m.LayersSkipped > 0 {
		fmt.Printf("  │   Layers/Blobs: %d processed | %d copied | %d skipped\n",
			m.LayersProcessed, m.LayersCopied, m.LayersSkipped)
	}
	if m.ManifestsProcessed > 0 || m.CatalogsMirrored > 0 {
		fmt.Printf("  │   Manifests: %d | Catalogs: %d\n", m.ManifestsProcessed, m.CatalogsMirrored)
	}
	// Always print errors/retries/warnings as they're important
	fmt.Printf("  │   Errors: %d | Retries: %d | Warnings: %d\n",
		m.ErrorCount, m.RetryCount, m.WarningCount)
	// Note: Operator count from log parsing can be inaccurate - oc-mirror describe provides accurate counts
}

func containsSkip(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "skip") ||
		strings.Contains(lower, "exists") ||
		strings.Contains(lower, "cached")
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
