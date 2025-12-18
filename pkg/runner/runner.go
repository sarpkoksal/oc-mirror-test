package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/telco-core/ngc-495/internal/config"
	"github.com/telco-core/ngc-495/pkg/command"
	"github.com/telco-core/ngc-495/pkg/monitor"
)

// TestRunner orchestrates test execution
type TestRunner struct {
	config  *Config
	results []TestResult
}

// NewTestRunner creates a new test runner
func NewTestRunner(cfg *Config) *TestRunner {
	if cfg.Iterations < 2 {
		cfg.Iterations = 2
	}
	return &TestRunner{
		config:  cfg,
		results: make([]TestResult, 0),
	}
}

// Run executes all test iterations
func (tr *TestRunner) Run() error {
	fmt.Printf("╔═══════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║        OC Mirror Test Automation - Metrics Collection        ║\n")
	fmt.Printf("╚═══════════════════════════════════════════════════════════════╝\n\n")
	fmt.Printf("Registry URL: %s\n", tr.config.RegistryURL)
	fmt.Printf("Iterations: %d\n", tr.config.Iterations)
	if tr.config.CompareV1V2 {
		fmt.Printf("V1/V2 Comparison: Enabled\n")
	}
	fmt.Printf("\n")

	// Create necessary directories
	if err := tr.setupDirectories(); err != nil {
		return fmt.Errorf("failed to setup directories: %w", err)
	}

	// Create imageset-config files for v1 and v2
	// v1 uses v1alpha2 API version, v2 uses v2alpha1
	if err := config.CreateImageSetConfigWithVersion("oc-mirror-clone/imagesetconfiguration_operators-v1.yaml", "v1alpha2"); err != nil {
		return fmt.Errorf("failed to create v1 imageset-config: %w", err)
	}
	if err := config.CreateImageSetConfigWithVersion("oc-mirror-clone/imagesetconfiguration_operators-v2.yaml", "v2alpha1"); err != nil {
		return fmt.Errorf("failed to create v2 imageset-config: %w", err)
	}
	// Also create default for backward compatibility
	if err := config.CreateImageSetConfig("oc-mirror-clone/imagesetconfiguration_operators.yaml"); err != nil {
		return fmt.Errorf("failed to create imageset-config: %w", err)
	}

	if tr.config.CompareV1V2 {
		return tr.runV1V2Comparison()
	}

	return tr.runStandardTest()
}

func (tr *TestRunner) runStandardTest() error {
	// Run iterations
	for i := 0; i < tr.config.Iterations; i++ {
		isCleanRun := i == 0
		fmt.Printf("\n╔═══════════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║  Iteration %d/%d (%s)                                          ║\n", i+1, tr.config.Iterations, map[bool]string{true: "CLEAN", false: "CACHED"}[isCleanRun])
		fmt.Printf("╚═══════════════════════════════════════════════════════════════╝\n")

		result, err := tr.runIteration(i+1, isCleanRun, "v2")
		if err != nil {
			return fmt.Errorf("iteration %d failed: %w", i+1, err)
		}

		tr.results = append(tr.results, result)
		tr.printIterationSummary(result)
	}

	// Compare results
	tr.compareCleanVsCached()

	// Save results to JSON
	if err := tr.saveResults(); err != nil {
		return fmt.Errorf("failed to save results: %w", err)
	}

	return nil
}

func (tr *TestRunner) runV1V2Comparison() error {
	fmt.Printf("\n╔═══════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║              V1 vs V2 Comparison Test                          ║\n")
	fmt.Printf("╚═══════════════════════════════════════════════════════════════╝\n\n")

	// Run v1 tests
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Running V1 Tests\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	var v1Results []TestResult
	for i := 0; i < tr.config.Iterations; i++ {
		isCleanRun := i == 0
		fmt.Printf("\n[V1] Iteration %d/%d (%s)\n", i+1, tr.config.Iterations, map[bool]string{true: "CLEAN", false: "CACHED"}[isCleanRun])

		result, err := tr.runIteration(i+1, isCleanRun, "v1")
		if err != nil {
			return fmt.Errorf("v1 iteration %d failed: %w", i+1, err)
		}
		v1Results = append(v1Results, result)
	}

	// Clean workspace for v2
	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Cleaning workspace for V2 tests...\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	if err := tr.cleanWorkspace(); err != nil {
		return fmt.Errorf("failed to clean workspace for v2: %w", err)
	}

	// Run v2 tests
	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Running V2 Tests\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	var v2Results []TestResult
	for i := 0; i < tr.config.Iterations; i++ {
		isCleanRun := i == 0
		fmt.Printf("\n[V2] Iteration %d/%d (%s)\n", i+1, tr.config.Iterations, map[bool]string{true: "CLEAN", false: "CACHED"}[isCleanRun])

		result, err := tr.runIteration(i+1, isCleanRun, "v2")
		if err != nil {
			return fmt.Errorf("v2 iteration %d failed: %w", i+1, err)
		}
		v2Results = append(v2Results, result)
	}

	// Store all results
	tr.results = append(v1Results, v2Results...)

	// Compare v1 vs v2
	tr.compareV1VsV2(v1Results, v2Results)

	// Save results to JSON
	if err := tr.saveResults(); err != nil {
		return fmt.Errorf("failed to save results: %w", err)
	}

	return nil
}

func (tr *TestRunner) setupDirectories() error {
	dirs := []string{
		"oc-mirror-clone",
		"mirror/operators",
		"mirror/operators-v1",
		"mirror/operators-v2",
		"platform",
		"platform/mirror",
		"operators",    // v2 cache directory
		"operators-v1", // v1 cache directory
		"operators-v2", // v2 cache directory
		"results",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

func (tr *TestRunner) runIteration(iterationNum int, isCleanRun bool, version string) (TestResult, error) {
	result := TestResult{
		Iteration:  iterationNum,
		IsCleanRun: isCleanRun,
		Version:    version,
	}

	// Clean workspace if this is a clean run
	if isCleanRun {
		if err := tr.cleanWorkspaceForVersion(version); err != nil {
			return result, fmt.Errorf("failed to clean workspace: %w", err)
		}
	}

	// Start network monitoring
	networkMonitor := monitor.NewNetworkMonitor()
	if err := networkMonitor.Start(); err != nil {
		fmt.Printf("Warning: Failed to start network monitoring: %v\n", err)
	}

	// Start overall resource monitoring for the entire iteration
	overallResourceMonitor := monitor.NewResourceMonitor()
	if err := overallResourceMonitor.Start(); err != nil {
		fmt.Printf("Warning: Failed to start overall resource monitoring: %v\n", err)
	}

	// Run download phase
	fmt.Printf("\n  ┌─ Download Phase (%s) ───────────────────────────────────────┐\n", version)
	downloadMetrics, err := tr.runDownloadPhase(isCleanRun, version)
	if err != nil {
		networkMonitor.Stop()
		overallResourceMonitor.Stop()
		return result, fmt.Errorf("download phase failed: %w", err)
	}
	result.DownloadPhase = downloadMetrics
	fmt.Printf("  └─────────────────────────────────────────────────────────────┘\n")

	// Start network monitoring for upload phase
	uploadNetworkMonitor := monitor.NewNetworkMonitor()
	if err := uploadNetworkMonitor.Start(); err != nil {
		fmt.Printf("Warning: Failed to start network monitoring for upload: %v\n", err)
	}

	// Stop download network monitoring and get metrics
	downloadNetworkMetrics := networkMonitor.Stop()
	result.NetworkMetrics = downloadNetworkMetrics

	// Run upload phase
	fmt.Printf("\n  ┌─ Upload Phase (%s) ─────────────────────────────────────────┐\n", version)
	uploadMetrics, err := tr.runUploadPhase(version)
	if err != nil {
		uploadNetworkMonitor.Stop()
		overallResourceMonitor.Stop()
		return result, fmt.Errorf("upload phase failed: %w", err)
	}
	result.UploadPhase = uploadMetrics
	fmt.Printf("  └─────────────────────────────────────────────────────────────┘\n")

	// Stop upload network monitoring
	uploadNetworkMetrics := uploadNetworkMonitor.Stop()
	// Combine network metrics
	result.NetworkMetrics.TotalBytesTransferred += uploadNetworkMetrics.TotalBytesTransferred
	if uploadNetworkMetrics.PeakBandwidthMbps > result.NetworkMetrics.PeakBandwidthMbps {
		result.NetworkMetrics.PeakBandwidthMbps = uploadNetworkMetrics.PeakBandwidthMbps
	}
	result.NetworkMetrics.AverageBandwidthMbps = (result.NetworkMetrics.AverageBandwidthMbps + uploadNetworkMetrics.AverageBandwidthMbps) / 2

	// Stop overall resource monitoring
	result.ResourceMetrics = overallResourceMonitor.Stop()

	// Analyze output directory
	var mirrorPath string
	if version == "v1" {
		mirrorPath = "mirror/operators-v1"
	} else {
		mirrorPath = "mirror/operators-v2"
	}
	fmt.Printf("\n  ┌─ Output Analysis (%s) ───────────────────────────────────────┐\n", version)
	outputVerifier := monitor.NewOutputVerifier(mirrorPath)
	outputMetrics, err := outputVerifier.Analyze()
	if err != nil {
		fmt.Printf("  │ Warning: Failed to analyze output: %v\n", err)
	} else {
		result.OutputMetrics = outputMetrics
		outputMetrics.PrintSummary()
	}

	// Get accurate image/layer counts from oc-mirror describe
	describeMetrics, err := command.DescribeMirror(mirrorPath + "/")
	if err != nil {
		fmt.Printf("  │ Warning: Failed to run oc-mirror describe: %v\n", err)
	} else {
		result.DescribeMetrics = describeMetrics
		describeMetrics.PrintSummary()
	}
	fmt.Printf("  └─────────────────────────────────────────────────────────────┘\n")

	// Generate summary
	result.Summary = tr.generateSummary(result)

	return result, nil
}

func (tr *TestRunner) cleanWorkspace() error {
	dirsToClean := []string{
		"mirror/operators",
		"mirror/operators-v1",
		"mirror/operators-v2",
		"platform/mirror",
	}

	for _, dir := range dirsToClean {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

func (tr *TestRunner) cleanWorkspaceForVersion(version string) error {
	var mirrorDir string
	if version == "v1" {
		mirrorDir = "mirror/operators-v1"
	} else {
		mirrorDir = "mirror/operators-v2"
	}

	dirsToClean := []string{
		mirrorDir,
		"platform/mirror",
	}

	for _, dir := range dirsToClean {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// Keep cache directory for subsequent runs
	return nil
}

func (tr *TestRunner) runDownloadPhase(isCleanRun bool, version string) (PhaseMetrics, error) {
	metrics := PhaseMetrics{}

	var mirrorDir string
	var mirrorPath string // Path for download monitoring (without file:// prefix)
	if version == "v1" {
		mirrorDir = "file://mirror/operators-v1"
		mirrorPath = "mirror/operators-v1"
	} else {
		mirrorDir = "file://mirror/operators-v2"
		mirrorPath = "mirror/operators-v2"
	}

	// Ensure the mirror directory exists
	if err := os.MkdirAll(mirrorPath, 0755); err != nil {
		return metrics, fmt.Errorf("failed to create mirror directory: %w", err)
	}

	// Start download monitoring for the mirror directory
	downloadMonitor := monitor.NewDownloadMonitor(mirrorPath)
	downloadMonitor.SetPollInterval(1 * time.Second)
	if err := downloadMonitor.Start(); err != nil {
		fmt.Printf("  │ Warning: Failed to start download monitoring: %v\n", err)
	}

	// Prepare resource monitor for oc-mirror process (will be started when we get the PID)
	resourceMonitor := monitor.NewResourceMonitor()
	resourceMonitor.SetPollInterval(500 * time.Millisecond) // More frequent sampling for child process

	cmd := command.NewOCMirrorCommand()
	cmd.SetV2(version == "v2")
	cmd.SetSkipTLS(tr.config.SkipTLS)

	// Use version-specific config file
	var configFile string
	if version == "v1" {
		configFile = "oc-mirror-clone/imagesetconfiguration_operators-v1.yaml"
		// v1: Skip missing packages and continue on errors
		cmd.SetSkipMissing(true)
		cmd.SetContinueOnError(true)
	} else {
		configFile = "oc-mirror-clone/imagesetconfiguration_operators-v2.yaml"
	}
	cmd.SetConfig(configFile)
	cmd.SetOutput(mirrorDir)
	if version == "v2" {
		cmd.SetCacheDir("operators-v2")
	}

	startTime := time.Now()

	// Execute with callback to get oc-mirror process PID for monitoring
	output, err := cmd.ExecuteWithCallback(func(pid int) {
		// Set target PID to monitor the oc-mirror process, not the test runner
		resourceMonitor.SetTargetPID(pid)
		if startErr := resourceMonitor.Start(); startErr != nil {
			fmt.Printf("  │ Warning: Failed to start resource monitoring for oc-mirror (PID %d): %v\n", pid, startErr)
		} else {
			fmt.Printf("  │ Monitoring oc-mirror process (PID: %d)\n", pid)
		}
	})
	metrics.WallTime = time.Since(startTime)

	// Stop all monitors and collect metrics
	downloadMetrics := downloadMonitor.Stop()
	metrics.DownloadMetrics = downloadMetrics

	resourceMetrics := resourceMonitor.Stop()
	metrics.ResourceMetrics = resourceMetrics

	// Extract extended metrics from logs
	extendedMetrics := output.ExtractExtendedMetrics()
	metrics.ExtendedMetrics = extendedMetrics

	if err != nil {
		// Still collect metrics even on error
		fmt.Printf("  │ Download failed but collected metrics\n")
		return metrics, fmt.Errorf("oc-mirror download failed: %w", err)
	}

	// Parse logs for cache hits and skipped images
	metrics.Logs = output.Logs
	metrics.ImagesSkipped = output.CountSkippedImages()
	metrics.CacheHits = output.CountCacheHits()

	// Print comprehensive download summary
	fmt.Printf("  │ Download completed in %v\n", metrics.WallTime)
	fmt.Printf("  │ Images skipped: %d | Cache hits: %d\n", metrics.ImagesSkipped, metrics.CacheHits)
	downloadMetrics.PrintSummary()
	resourceMetrics.PrintSummary()
	extendedMetrics.PrintSummary()

	return metrics, nil
}

func (tr *TestRunner) runUploadPhase(version string) (PhaseMetrics, error) {
	metrics := PhaseMetrics{}

	// Ensure registry URL has a scheme prefix
	registryURL := tr.config.RegistryURL
	if !strings.Contains(registryURL, "://") {
		// Default to docker:// if no scheme is provided
		registryURL = "docker://" + registryURL
	}

	// Prepare resource monitor for oc-mirror process (will be started when we get the PID)
	resourceMonitor := monitor.NewResourceMonitor()
	resourceMonitor.SetPollInterval(500 * time.Millisecond) // More frequent sampling for child process

	cmd := command.NewOCMirrorCommand()
	cmd.SetV2(version == "v2")
	cmd.SetSkipTLS(tr.config.SkipTLS)

	if version == "v1" {
		// v1: Use platform config with --from flag to upload from local mirror
		platformConfigPath := "platform/platform_config-v1.yaml"
		if err := config.CreatePlatformConfigWithVersion(platformConfigPath, "v1alpha2"); err != nil {
			return metrics, fmt.Errorf("failed to create platform config: %w", err)
		}
		cmd.SetConfig(platformConfigPath)
		cmd.SetFrom("mirror/operators-v1/")
		cmd.SetOutput(registryURL)
	} else {
		// v2: Use original imageset config with --cache-dir, output directly to registry
		// Command: oc-mirror --v2 --cache-dir operators-v2 -c <config> --dest-tls-verify=false docker://registry
		cmd.SetConfig("oc-mirror-clone/imagesetconfiguration_operators-v2.yaml")
		cmd.SetCacheDir("operators-v2")
		cmd.SetOutput(registryURL)
		// Note: v2 does NOT use --from flag
	}

	startTime := time.Now()

	// Execute with callback to get oc-mirror process PID for monitoring
	output, err := cmd.ExecuteWithCallback(func(pid int) {
		// Set target PID to monitor the oc-mirror process, not the test runner
		resourceMonitor.SetTargetPID(pid)
		if startErr := resourceMonitor.Start(); startErr != nil {
			fmt.Printf("  │ Warning: Failed to start resource monitoring for oc-mirror (PID %d): %v\n", pid, startErr)
		} else {
			fmt.Printf("  │ Monitoring oc-mirror process (PID: %d)\n", pid)
		}
	})
	metrics.WallTime = time.Since(startTime)

	// Stop resource monitoring
	resourceMetrics := resourceMonitor.Stop()
	metrics.ResourceMetrics = resourceMetrics

	// Extract extended metrics from logs
	extendedMetrics := output.ExtractExtendedMetrics()
	metrics.ExtendedMetrics = extendedMetrics

	if err != nil {
		// Still show metrics on error
		fmt.Printf("  │ Upload failed but collected metrics\n")
		return metrics, fmt.Errorf("oc-mirror upload failed: %w", err)
	}

	// Parse logs for bytes uploaded
	metrics.Logs = output.Logs
	metrics.BytesUploaded = output.ExtractBytesUploaded()
	metrics.ImagesSkipped = output.CountSkippedImages()
	metrics.CacheHits = output.CountCacheHits()

	// Print comprehensive upload summary
	fmt.Printf("  │ Upload completed in %v\n", metrics.WallTime)
	fmt.Printf("  │ Bytes uploaded: %s\n", monitor.FormatBytesHuman(metrics.BytesUploaded))
	fmt.Printf("  │ Images skipped: %d | Cache hits: %d\n", metrics.ImagesSkipped, metrics.CacheHits)
	resourceMetrics.PrintSummary()
	extendedMetrics.PrintSummary()

	return metrics, nil
}

func (tr *TestRunner) printIterationSummary(result TestResult) {
	fmt.Printf("\n╔═══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  Iteration %d Summary (%s) - %s                                               ║\n",
		result.Iteration, result.Version, map[bool]string{true: "CLEAN RUN", false: "CACHED RUN"}[result.IsCleanRun])
	fmt.Printf("╠═══════════════════════════════════════════════════════════════════════════════╣\n")

	// Timing
	fmt.Printf("║  TIMING                                                                       ║\n")
	fmt.Printf("║    Download: %-65v ║\n", result.DownloadPhase.WallTime)
	fmt.Printf("║    Upload:   %-65v ║\n", result.UploadPhase.WallTime)
	fmt.Printf("║    Total:    %-65v ║\n", result.DownloadPhase.WallTime+result.UploadPhase.WallTime)

	// Data Transfer
	fmt.Printf("║  DATA TRANSFER                                                                ║\n")
	fmt.Printf("║    Downloaded: %-63s ║\n", monitor.FormatBytesHuman(result.DownloadPhase.DownloadMetrics.TotalBytesDownloaded))
	fmt.Printf("║    Avg Speed:  %.2f MB/s | Peak: %.2f MB/s                                    ║\n",
		result.DownloadPhase.DownloadMetrics.AverageSpeedMBs, result.DownloadPhase.DownloadMetrics.PeakSpeedMBs)

	// Resource Usage
	fmt.Printf("║  RESOURCE USAGE                                                               ║\n")
	fmt.Printf("║    CPU:    Avg %.2f%% | Peak %.2f%%                                            ║\n",
		result.ResourceMetrics.CPUAvgPercent, result.ResourceMetrics.CPUPeakPercent)
	fmt.Printf("║    Memory: Avg %.2f MB | Peak %.2f MB                                         ║\n",
		result.ResourceMetrics.MemoryAvgMB, result.ResourceMetrics.MemoryPeakMB)

	// Network
	fmt.Printf("║  NETWORK                                                                      ║\n")
	fmt.Printf("║    Bandwidth: Avg %.2f Mbps | Peak %.2f Mbps                                  ║\n",
		result.NetworkMetrics.AverageBandwidthMbps, result.NetworkMetrics.PeakBandwidthMbps)

	// Image/Layer Processing (from oc-mirror describe)
	fmt.Printf("║  MIRROR CONTENT                                                               ║\n")
	if result.DescribeMetrics != nil {
		fmt.Printf("║    Images: %d | Layers: %d | Manifests: %d                                    ║\n",
			result.DescribeMetrics.TotalImages, result.DescribeMetrics.TotalLayers, result.DescribeMetrics.TotalManifests)
		fmt.Printf("║    Operator Packages: %d | Associations: %d                                   ║\n",
			result.DescribeMetrics.OperatorPackages, result.DescribeMetrics.TotalAssociations)
	} else {
		fmt.Printf("║    (oc-mirror describe not available)                                        ║\n")
	}
	fmt.Printf("║    Cache Hits: %d | Errors: %d | Retries: %d                                  ║\n",
		result.DownloadPhase.CacheHits,
		result.DownloadPhase.ExtendedMetrics.ErrorCount+result.UploadPhase.ExtendedMetrics.ErrorCount,
		result.DownloadPhase.ExtendedMetrics.RetryCount+result.UploadPhase.ExtendedMetrics.RetryCount)

	// Output
	fmt.Printf("║  OUTPUT                                                                       ║\n")
	fmt.Printf("║    Total Size: %-63s ║\n", monitor.FormatBytesHuman(result.OutputMetrics.TotalSize))
	fmt.Printf("║    Files: %d | Directories: %d                                                ║\n",
		result.OutputMetrics.TotalFiles, result.OutputMetrics.TotalDirs)

	fmt.Printf("╚═══════════════════════════════════════════════════════════════════════════════╝\n")
}

func (tr *TestRunner) compareCleanVsCached() {
	if len(tr.results) < 2 {
		return
	}

	fmt.Printf("\n╔═══════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  Comparison: Clean vs Cached                                  ║\n")
	fmt.Printf("╠═══════════════════════════════════════════════════════════════╣\n")

	cleanResult := tr.results[0]
	var cachedResults []TestResult
	for i := 1; i < len(tr.results); i++ {
		cachedResults = append(cachedResults, tr.results[i])
	}

	// Calculate averages for cached runs
	var avgCachedDownloadTime time.Duration
	var avgCachedUploadTime time.Duration
	var avgCachedBytes int64
	var avgCachedCacheHits int

	for _, r := range cachedResults {
		avgCachedDownloadTime += r.DownloadPhase.WallTime
		avgCachedUploadTime += r.UploadPhase.WallTime
		avgCachedBytes += r.UploadPhase.BytesUploaded
		avgCachedCacheHits += r.DownloadPhase.CacheHits
	}

	if len(cachedResults) > 0 {
		avgCachedDownloadTime /= time.Duration(len(cachedResults))
		avgCachedUploadTime /= time.Duration(len(cachedResults))
		avgCachedBytes /= int64(len(cachedResults))
		avgCachedCacheHits /= len(cachedResults)
	}

	fmt.Printf("║  Download Time:                                                 ║\n")
	fmt.Printf("║    Clean:  %-52v ║\n", cleanResult.DownloadPhase.WallTime)
	fmt.Printf("║    Cached: %-52v ║\n", avgCachedDownloadTime)
	if avgCachedDownloadTime > 0 {
		improvement := float64(cleanResult.DownloadPhase.WallTime-avgCachedDownloadTime) / float64(cleanResult.DownloadPhase.WallTime) * 100
		fmt.Printf("║    Improvement: %-46.2f%% ║\n", improvement)
	}

	fmt.Printf("║                                                                ║\n")
	fmt.Printf("║  Upload Time:                                                   ║\n")
	fmt.Printf("║    Clean:  %-52v ║\n", cleanResult.UploadPhase.WallTime)
	fmt.Printf("║    Cached: %-52v ║\n", avgCachedUploadTime)
	if avgCachedUploadTime > 0 {
		improvement := float64(cleanResult.UploadPhase.WallTime-avgCachedUploadTime) / float64(cleanResult.UploadPhase.WallTime) * 100
		fmt.Printf("║    Improvement: %-46.2f%% ║\n", improvement)
	}

	fmt.Printf("║                                                                ║\n")
	fmt.Printf("║  Cache Hits:                                                    ║\n")
	fmt.Printf("║    Clean:  %-52d ║\n", cleanResult.DownloadPhase.CacheHits)
	fmt.Printf("║    Cached: %-52d ║\n", avgCachedCacheHits)

	fmt.Printf("║                                                                ║\n")
	fmt.Printf("║  Bytes Uploaded:                                                ║\n")
	fmt.Printf("║    Clean:  %-52d (%.2f MB) ║\n", cleanResult.UploadPhase.BytesUploaded, float64(cleanResult.UploadPhase.BytesUploaded)/(1024*1024))
	fmt.Printf("║    Cached: %-52d (%.2f MB) ║\n", avgCachedBytes, float64(avgCachedBytes)/(1024*1024))
	fmt.Printf("╚═══════════════════════════════════════════════════════════════╝\n")
}

func (tr *TestRunner) compareV1VsV2(v1Results, v2Results []TestResult) {
	if len(v1Results) == 0 || len(v2Results) == 0 {
		return
	}

	fmt.Printf("\n╔═══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                    COMPREHENSIVE V1 vs V2 COMPARISON                          ║\n")
	fmt.Printf("╠═══════════════════════════════════════════════════════════════════════════════╣\n")

	// Compare clean runs (first iteration)
	v1Clean := v1Results[0]
	v2Clean := v2Results[0]

	// === TIMING COMPARISON ===
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  ═══ TIMING METRICS ═══════════════════════════════════════════════════════   ║\n")
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  Download Time:                                                               ║\n")
	fmt.Printf("║    V1: %-71v ║\n", v1Clean.DownloadPhase.WallTime)
	fmt.Printf("║    V2: %-71v ║\n", v2Clean.DownloadPhase.WallTime)
	if v1Clean.DownloadPhase.WallTime > 0 {
		diff := float64(v1Clean.DownloadPhase.WallTime-v2Clean.DownloadPhase.WallTime) / float64(v1Clean.DownloadPhase.WallTime) * 100
		status := "faster"
		if diff < 0 {
			status = "slower"
			diff = -diff
		}
		fmt.Printf("║    V2 is %.2f%% %s                                                          ║\n", diff, status)
	}

	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  Upload Time:                                                                 ║\n")
	fmt.Printf("║    V1: %-71v ║\n", v1Clean.UploadPhase.WallTime)
	fmt.Printf("║    V2: %-71v ║\n", v2Clean.UploadPhase.WallTime)
	if v1Clean.UploadPhase.WallTime > 0 {
		diff := float64(v1Clean.UploadPhase.WallTime-v2Clean.UploadPhase.WallTime) / float64(v1Clean.UploadPhase.WallTime) * 100
		status := "faster"
		if diff < 0 {
			status = "slower"
			diff = -diff
		}
		fmt.Printf("║    V2 is %.2f%% %s                                                          ║\n", diff, status)
	}

	totalV1 := v1Clean.DownloadPhase.WallTime + v1Clean.UploadPhase.WallTime
	totalV2 := v2Clean.DownloadPhase.WallTime + v2Clean.UploadPhase.WallTime
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  Total Time:                                                                  ║\n")
	fmt.Printf("║    V1: %-71v ║\n", totalV1)
	fmt.Printf("║    V2: %-71v ║\n", totalV2)

	// === DOWNLOAD SPEED COMPARISON ===
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  ═══ DOWNLOAD SPEED ═══════════════════════════════════════════════════════   ║\n")
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  Average Download Speed:                                                      ║\n")
	fmt.Printf("║    V1: %.2f MB/s                                                              ║\n", v1Clean.DownloadPhase.DownloadMetrics.AverageSpeedMBs)
	fmt.Printf("║    V2: %.2f MB/s                                                              ║\n", v2Clean.DownloadPhase.DownloadMetrics.AverageSpeedMBs)
	fmt.Printf("║  Peak Download Speed:                                                         ║\n")
	fmt.Printf("║    V1: %.2f MB/s                                                              ║\n", v1Clean.DownloadPhase.DownloadMetrics.PeakSpeedMBs)
	fmt.Printf("║    V2: %.2f MB/s                                                              ║\n", v2Clean.DownloadPhase.DownloadMetrics.PeakSpeedMBs)

	// === RESOURCE USAGE COMPARISON ===
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  ═══ RESOURCE USAGE ═══════════════════════════════════════════════════════   ║\n")
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  CPU Usage (Average / Peak):                                                  ║\n")
	fmt.Printf("║    V1: %.2f%% / %.2f%%                                                         ║\n",
		v1Clean.ResourceMetrics.CPUAvgPercent, v1Clean.ResourceMetrics.CPUPeakPercent)
	fmt.Printf("║    V2: %.2f%% / %.2f%%                                                         ║\n",
		v2Clean.ResourceMetrics.CPUAvgPercent, v2Clean.ResourceMetrics.CPUPeakPercent)
	fmt.Printf("║  Memory Usage (Average / Peak):                                               ║\n")
	fmt.Printf("║    V1: %.2f MB / %.2f MB                                                      ║\n",
		v1Clean.ResourceMetrics.MemoryAvgMB, v1Clean.ResourceMetrics.MemoryPeakMB)
	fmt.Printf("║    V2: %.2f MB / %.2f MB                                                      ║\n",
		v2Clean.ResourceMetrics.MemoryAvgMB, v2Clean.ResourceMetrics.MemoryPeakMB)

	// === NETWORK COMPARISON ===
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  ═══ NETWORK BANDWIDTH ════════════════════════════════════════════════════   ║\n")
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  Average Bandwidth:                                                           ║\n")
	fmt.Printf("║    V1: %.2f Mbps                                                              ║\n", v1Clean.NetworkMetrics.AverageBandwidthMbps)
	fmt.Printf("║    V2: %.2f Mbps                                                              ║\n", v2Clean.NetworkMetrics.AverageBandwidthMbps)
	fmt.Printf("║  Peak Bandwidth:                                                              ║\n")
	fmt.Printf("║    V1: %.2f Mbps                                                              ║\n", v1Clean.NetworkMetrics.PeakBandwidthMbps)
	fmt.Printf("║    V2: %.2f Mbps                                                              ║\n", v2Clean.NetworkMetrics.PeakBandwidthMbps)

	// === MIRROR CONTENT (from oc-mirror describe) ===
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  ═══ MIRROR CONTENT (oc-mirror describe) ══════════════════════════════════   ║\n")
	fmt.Printf("║                                                                               ║\n")
	if v1Clean.DescribeMetrics != nil && v2Clean.DescribeMetrics != nil {
		fmt.Printf("║  Total Images:                                                                ║\n")
		fmt.Printf("║    V1: %d                                                                     ║\n", v1Clean.DescribeMetrics.TotalImages)
		fmt.Printf("║    V2: %d                                                                     ║\n", v2Clean.DescribeMetrics.TotalImages)
		fmt.Printf("║  Total Layers:                                                                ║\n")
		fmt.Printf("║    V1: %d                                                                     ║\n", v1Clean.DescribeMetrics.TotalLayers)
		fmt.Printf("║    V2: %d                                                                     ║\n", v2Clean.DescribeMetrics.TotalLayers)
		fmt.Printf("║  Total Manifests:                                                             ║\n")
		fmt.Printf("║    V1: %d                                                                     ║\n", v1Clean.DescribeMetrics.TotalManifests)
		fmt.Printf("║    V2: %d                                                                     ║\n", v2Clean.DescribeMetrics.TotalManifests)
		fmt.Printf("║  Operator Packages:                                                           ║\n")
		fmt.Printf("║    V1: %d                                                                     ║\n", v1Clean.DescribeMetrics.OperatorPackages)
		fmt.Printf("║    V2: %d                                                                     ║\n", v2Clean.DescribeMetrics.OperatorPackages)
		fmt.Printf("║  Total Associations:                                                          ║\n")
		fmt.Printf("║    V1: %d                                                                     ║\n", v1Clean.DescribeMetrics.TotalAssociations)
		fmt.Printf("║    V2: %d                                                                     ║\n", v2Clean.DescribeMetrics.TotalAssociations)
	} else {
		fmt.Printf("║  (oc-mirror describe metrics not available for comparison)                   ║\n")
	}

	// === ERROR/RETRY METRICS ===
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  ═══ ERROR/RETRY METRICS ══════════════════════════════════════════════════   ║\n")
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  Errors:                                                                      ║\n")
	fmt.Printf("║    V1: %d                                                                     ║\n", v1Clean.DownloadPhase.ExtendedMetrics.ErrorCount+v1Clean.UploadPhase.ExtendedMetrics.ErrorCount)
	fmt.Printf("║    V2: %d                                                                     ║\n", v2Clean.DownloadPhase.ExtendedMetrics.ErrorCount+v2Clean.UploadPhase.ExtendedMetrics.ErrorCount)
	fmt.Printf("║  Retries:                                                                     ║\n")
	fmt.Printf("║    V1: %d                                                                     ║\n", v1Clean.DownloadPhase.ExtendedMetrics.RetryCount+v1Clean.UploadPhase.ExtendedMetrics.RetryCount)
	fmt.Printf("║    V2: %d                                                                     ║\n", v2Clean.DownloadPhase.ExtendedMetrics.RetryCount+v2Clean.UploadPhase.ExtendedMetrics.RetryCount)
	fmt.Printf("║  Warnings:                                                                    ║\n")
	fmt.Printf("║    V1: %d                                                                     ║\n", v1Clean.DownloadPhase.ExtendedMetrics.WarningCount+v1Clean.UploadPhase.ExtendedMetrics.WarningCount)
	fmt.Printf("║    V2: %d                                                                     ║\n", v2Clean.DownloadPhase.ExtendedMetrics.WarningCount+v2Clean.UploadPhase.ExtendedMetrics.WarningCount)

	// === OUTPUT SIZE COMPARISON ===
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  ═══ OUTPUT SIZE ══════════════════════════════════════════════════════════   ║\n")
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  Total Downloaded:                                                            ║\n")
	fmt.Printf("║    V1: %s                                                                     ║\n", monitor.FormatBytesHuman(v1Clean.OutputMetrics.TotalSize))
	fmt.Printf("║    V2: %s                                                                     ║\n", monitor.FormatBytesHuman(v2Clean.OutputMetrics.TotalSize))
	fmt.Printf("║  Total Files:                                                                 ║\n")
	fmt.Printf("║    V1: %d                                                                     ║\n", v1Clean.OutputMetrics.TotalFiles)
	fmt.Printf("║    V2: %d                                                                     ║\n", v2Clean.OutputMetrics.TotalFiles)

	// === OUTPUT VERIFICATION ===
	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("║  ═══ OUTPUT VERIFICATION ══════════════════════════════════════════════════   ║\n")
	fmt.Printf("║                                                                               ║\n")
	comparison, err := monitor.CompareOutputs("mirror/operators-v1", "mirror/operators-v2")
	if err != nil {
		fmt.Printf("║  Could not compare outputs: %v                                               ║\n", err)
	} else {
		if comparison.Match {
			fmt.Printf("║  ✓ V1 and V2 outputs are IDENTICAL                                           ║\n")
		} else {
			fmt.Printf("║  ✗ V1 and V2 outputs DIFFER                                                  ║\n")
			fmt.Printf("║    Size difference: %s                                                       ║\n", monitor.FormatBytesHuman(comparison.SizeDifference))
			fmt.Printf("║    File count difference: %d                                                 ║\n", comparison.FileCountDiff)
			if len(comparison.MissingInFirst) > 0 {
				fmt.Printf("║    Missing in V1: %d files                                                   ║\n", len(comparison.MissingInFirst))
			}
			if len(comparison.MissingInSecond) > 0 {
				fmt.Printf("║    Missing in V2: %d files                                                   ║\n", len(comparison.MissingInSecond))
			}
			if len(comparison.DifferentContent) > 0 {
				fmt.Printf("║    Different content: %d files                                               ║\n", len(comparison.DifferentContent))
			}
		}
	}

	// === CACHE EFFECTIVENESS (if we have cached runs) ===
	if len(v1Results) > 1 && len(v2Results) > 1 {
		fmt.Printf("║                                                                               ║\n")
		fmt.Printf("║  ═══ CACHING EFFECTIVENESS ════════════════════════════════════════════════   ║\n")
		fmt.Printf("║                                                                               ║\n")
		v1Cached := v1Results[1]
		v2Cached := v2Results[1]

		v1CacheImprovement := float64(v1Clean.DownloadPhase.WallTime-v1Cached.DownloadPhase.WallTime) / float64(v1Clean.DownloadPhase.WallTime) * 100
		v2CacheImprovement := float64(v2Clean.DownloadPhase.WallTime-v2Cached.DownloadPhase.WallTime) / float64(v2Clean.DownloadPhase.WallTime) * 100

		fmt.Printf("║  Download Time Improvement (Clean vs Cached):                                 ║\n")
		fmt.Printf("║    V1: %.2f%%                                                                 ║\n", v1CacheImprovement)
		fmt.Printf("║    V2: %.2f%%                                                                 ║\n", v2CacheImprovement)
		fmt.Printf("║  Cache Hits (Cached Run):                                                     ║\n")
		fmt.Printf("║    V1: %d                                                                     ║\n", v1Cached.DownloadPhase.CacheHits)
		fmt.Printf("║    V2: %d                                                                     ║\n", v2Cached.DownloadPhase.CacheHits)
	}

	fmt.Printf("║                                                                               ║\n")
	fmt.Printf("╚═══════════════════════════════════════════════════════════════════════════════╝\n")
}

func (tr *TestRunner) generateSummary(result TestResult) string {
	return fmt.Sprintf("Iteration %d (%s, %s): Download=%v, Upload=%v, Bytes=%d, CacheHits=%d",
		result.Iteration,
		map[bool]string{true: "CLEAN", false: "CACHED"}[result.IsCleanRun],
		result.Version,
		result.DownloadPhase.WallTime,
		result.UploadPhase.WallTime,
		result.UploadPhase.BytesUploaded,
		result.DownloadPhase.CacheHits,
	)
}

func (tr *TestRunner) saveResults() error {
	resultsPath := filepath.Join("results", fmt.Sprintf("results_%s.json", time.Now().Format("20060102_150405")))

	data, err := json.MarshalIndent(tr.results, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(resultsPath, data, 0644)
}
