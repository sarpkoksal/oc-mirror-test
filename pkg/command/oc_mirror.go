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
	removeSignatures bool
	config          string
	output          string
	from            string
	cacheDir        string
	skipMissing     bool
	continueOnError bool
}

// CommandOutput contains the output from oc-mirror execution
type CommandOutput struct {
	Logs            []string
	Stdout          string
	Stderr          string
	ExitCode        int
}

// NewOCMirrorCommand creates a new oc-mirror command wrapper
func NewOCMirrorCommand() *OCMirrorCommand {
	return &OCMirrorCommand{
		v2:              false,
		removeSignatures: true,
	}
}

// SetV2 sets the v2 flag
func (cmd *OCMirrorCommand) SetV2(v2 bool) {
	cmd.v2 = v2
}

// SetRemoveSignatures sets the remove-signatures flag
func (cmd *OCMirrorCommand) SetRemoveSignatures(remove bool) {
	cmd.removeSignatures = remove
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

// Execute runs the oc-mirror command
func (cmd *OCMirrorCommand) Execute() (*CommandOutput, error) {
	args := cmd.buildArgs()

	fmt.Printf("Executing: oc-mirror %s\n", strings.Join(args, " "))

	execCmd := exec.Command("oc-mirror", args...)
	
	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	err := execCmd.Run()
	
	output := &CommandOutput{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: execCmd.ProcessState.ExitCode(),
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
		if !cmd.removeSignatures {
			args = append(args, "--remove-signatures=false")
		}
		if cmd.cacheDir != "" {
			args = append(args, "--cache-dir", cmd.cacheDir)
		}
	} else {
		// v1 doesn't support --remove-signatures or --cache-dir flags
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
		args = append(args, "-c", cmd.config)
	}

	if cmd.from != "" {
		args = append(args, "--from", cmd.from)
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
