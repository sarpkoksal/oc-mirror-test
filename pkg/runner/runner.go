package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
		"operators",      // v2 cache directory
		"operators-v1",   // v1 cache directory
		"operators-v2",   // v2 cache directory
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

	// Run download phase
	fmt.Printf("\n  ┌─ Download Phase (%s) ───────────────────────────────────────┐\n", version)
	downloadMetrics, err := tr.runDownloadPhase(isCleanRun, version)
	if err != nil {
		networkMonitor.Stop()
		return result, fmt.Errorf("download phase failed: %w", err)
	}
	result.DownloadPhase = downloadMetrics
	fmt.Printf("  └─────────────────────────────────────────────────────────────┘\n")

	// Copy mirror to platform directory for upload
	if err := tr.prepareUploadMirror(version); err != nil {
		networkMonitor.Stop()
		return result, fmt.Errorf("failed to prepare upload mirror: %w", err)
	}

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
	if version == "v1" {
		mirrorDir = "file://mirror/operators-v1"
	} else {
		mirrorDir = "file://mirror/operators-v2"
	}

	cmd := command.NewOCMirrorCommand()
	cmd.SetV2(version == "v2")
	cmd.SetRemoveSignatures(false)
	
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
	output, err := cmd.Execute()
	metrics.WallTime = time.Since(startTime)

	if err != nil {
		return metrics, fmt.Errorf("oc-mirror download failed: %w", err)
	}

	// Parse logs for cache hits and skipped images
	metrics.Logs = output.Logs
	metrics.ImagesSkipped = output.CountSkippedImages()
	metrics.CacheHits = output.CountCacheHits()

	fmt.Printf("  │ Download completed in %v\n", metrics.WallTime)
	fmt.Printf("  │ Images skipped: %d\n", metrics.ImagesSkipped)
	fmt.Printf("  │ Cache hits: %d\n", metrics.CacheHits)

	return metrics, nil
}

func (tr *TestRunner) runUploadPhase(version string) (PhaseMetrics, error) {
	metrics := PhaseMetrics{}

	// Create platform config with version-specific API version
	var platformConfigPath string
	var apiVersion string
	if version == "v1" {
		platformConfigPath = "platform/platform_config-v1.yaml"
		apiVersion = "v1alpha2"
	} else {
		platformConfigPath = "platform/platform_config-v2.yaml"
		apiVersion = "v2alpha1"
	}
	if err := config.CreatePlatformConfigWithVersion(platformConfigPath, apiVersion); err != nil {
		return metrics, fmt.Errorf("failed to create platform config: %w", err)
	}

	// Ensure registry URL has a scheme prefix
	registryURL := tr.config.RegistryURL
	if !strings.Contains(registryURL, "://") {
		// Default to docker:// if no scheme is provided
		registryURL = "docker://" + registryURL
	}

	cmd := command.NewOCMirrorCommand()
	cmd.SetV2(version == "v2")
	cmd.SetConfig(platformConfigPath)
	cmd.SetFrom("file://platform/mirror")
	cmd.SetOutput(registryURL)

	startTime := time.Now()
	output, err := cmd.Execute()
	metrics.WallTime = time.Since(startTime)

	if err != nil {
		return metrics, fmt.Errorf("oc-mirror upload failed: %w", err)
	}

	// Parse logs for bytes uploaded
	metrics.Logs = output.Logs
	metrics.BytesUploaded = output.ExtractBytesUploaded()
	metrics.ImagesSkipped = output.CountSkippedImages()
	metrics.CacheHits = output.CountCacheHits()

	fmt.Printf("  │ Upload completed in %v\n", metrics.WallTime)
	fmt.Printf("  │ Bytes uploaded: %d (%.2f MB)\n", metrics.BytesUploaded, float64(metrics.BytesUploaded)/(1024*1024))
	fmt.Printf("  │ Images skipped: %d\n", metrics.ImagesSkipped)

	return metrics, nil
}

func (tr *TestRunner) prepareUploadMirror(version string) error {
	var sourceDir string
	if version == "v1" {
		sourceDir = "mirror/operators-v1"
	} else {
		sourceDir = "mirror/operators-v2"
	}
	targetDir := "platform/mirror"

	// Use cp command to copy directory
	cmd := exec.Command("cp", "-r", sourceDir, targetDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy mirror directory: %w", err)
	}

	return nil
}

func (tr *TestRunner) printIterationSummary(result TestResult) {
	fmt.Printf("\n╔═══════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  Iteration %d Summary (%s)                                    ║\n", result.Iteration, result.Version)
	fmt.Printf("╠═══════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  Type: %-55s ║\n", map[bool]string{true: "CLEAN", false: "CACHED"}[result.IsCleanRun])
	fmt.Printf("║  Download Time: %-45v ║\n", result.DownloadPhase.WallTime)
	fmt.Printf("║  Upload Time: %-47v ║\n", result.UploadPhase.WallTime)
	fmt.Printf("║  Total Time: %-49v ║\n", result.DownloadPhase.WallTime+result.UploadPhase.WallTime)
	fmt.Printf("║  Bytes Uploaded: %-42d (%.2f MB) ║\n", result.UploadPhase.BytesUploaded, float64(result.UploadPhase.BytesUploaded)/(1024*1024))
	fmt.Printf("║  Download Cache Hits: %-38d ║\n", result.DownloadPhase.CacheHits)
	fmt.Printf("║  Upload Cache Hits: %-40d ║\n", result.UploadPhase.CacheHits)
	fmt.Printf("║  Average Bandwidth: %-40.2f Mbps ║\n", result.NetworkMetrics.AverageBandwidthMbps)
	fmt.Printf("║  Peak Bandwidth: %-43.2f Mbps ║\n", result.NetworkMetrics.PeakBandwidthMbps)
	fmt.Printf("╚═══════════════════════════════════════════════════════════════╝\n")
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

	fmt.Printf("\n╔═══════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  Comparison: V1 vs V2                                          ║\n")
	fmt.Printf("╠═══════════════════════════════════════════════════════════════╣\n")

	// Compare clean runs (first iteration)
	v1Clean := v1Results[0]
	v2Clean := v2Results[0]

	fmt.Printf("║  Clean Run Comparison:                                         ║\n")
	fmt.Printf("║                                                                ║\n")
	fmt.Printf("║  Download Time:                                                ║\n")
	fmt.Printf("║    V1: %-54v ║\n", v1Clean.DownloadPhase.WallTime)
	fmt.Printf("║    V2: %-54v ║\n", v2Clean.DownloadPhase.WallTime)
	if v1Clean.DownloadPhase.WallTime > 0 {
		diff := float64(v1Clean.DownloadPhase.WallTime-v2Clean.DownloadPhase.WallTime) / float64(v1Clean.DownloadPhase.WallTime) * 100
		fmt.Printf("║    V2 Improvement: %-46.2f%% ║\n", diff)
	}

	fmt.Printf("║                                                                ║\n")
	fmt.Printf("║  Upload Time:                                                  ║\n")
	fmt.Printf("║    V1: %-54v ║\n", v1Clean.UploadPhase.WallTime)
	fmt.Printf("║    V2: %-54v ║\n", v2Clean.UploadPhase.WallTime)
	if v1Clean.UploadPhase.WallTime > 0 {
		diff := float64(v1Clean.UploadPhase.WallTime-v2Clean.UploadPhase.WallTime) / float64(v1Clean.UploadPhase.WallTime) * 100
		fmt.Printf("║    V2 Improvement: %-46.2f%% ║\n", diff)
	}

	fmt.Printf("║                                                                ║\n")
	fmt.Printf("║  Cache Hits (Download):                                        ║\n")
	fmt.Printf("║    V1: %-54d ║\n", v1Clean.DownloadPhase.CacheHits)
	fmt.Printf("║    V2: %-54d ║\n", v2Clean.DownloadPhase.CacheHits)

	fmt.Printf("║                                                                ║\n")
	fmt.Printf("║  Bytes Uploaded:                                               ║\n")
	fmt.Printf("║    V1: %-54d (%.2f MB) ║\n", v1Clean.UploadPhase.BytesUploaded, float64(v1Clean.UploadPhase.BytesUploaded)/(1024*1024))
	fmt.Printf("║    V2: %-54d (%.2f MB) ║\n", v2Clean.UploadPhase.BytesUploaded, float64(v2Clean.UploadPhase.BytesUploaded)/(1024*1024))

	fmt.Printf("║                                                                ║\n")
	fmt.Printf("║  Network Bandwidth:                                            ║\n")
	fmt.Printf("║    V1 Avg: %-50.2f Mbps ║\n", v1Clean.NetworkMetrics.AverageBandwidthMbps)
	fmt.Printf("║    V2 Avg: %-50.2f Mbps ║\n", v2Clean.NetworkMetrics.AverageBandwidthMbps)
	fmt.Printf("║    V1 Peak: %-49.2f Mbps ║\n", v1Clean.NetworkMetrics.PeakBandwidthMbps)
	fmt.Printf("║    V2 Peak: %-49.2f Mbps ║\n", v2Clean.NetworkMetrics.PeakBandwidthMbps)

	fmt.Printf("╚═══════════════════════════════════════════════════════════════╝\n")
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
