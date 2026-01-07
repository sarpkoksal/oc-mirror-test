package monitor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// OutputVerifier verifies and compares mirror output directories
type OutputVerifier struct {
	directory string
}

// OutputMetrics contains metrics about the output directory
type OutputMetrics struct {
	TotalSize       int64            `json:"TotalSize"`
	TotalFiles      int               `json:"TotalFiles"`
	TotalDirs       int               `json:"TotalDirs"`
	DirectoryHash   string            `json:"DirectoryHash"`   // Combined hash of all file hashes
	FileHashes      map[string]string `json:"FileHashes"`      // Individual file hashes
	LargestFiles    []FileInfo        `json:"LargestFiles"`    // Top 10 largest files
	FileTypes       map[string]int    `json:"FileTypes"`       // Count by extension
	LayerCount      int               `json:"LayerCount"`      // Number of blob layers
	ManifestCount   int               `json:"ManifestCount"`    // Number of manifests
	SignatureCount  int               `json:"SignatureCount"`  // Number of signatures
}

// FileInfo contains information about a single file
type FileInfo struct {
	Path string `json:"Path"`
	Size int64  `json:"Size"`
	Hash string `json:"Hash"`
}

// ComparisonResult contains the comparison between two outputs
type OutputComparisonResult struct {
	Match            bool     `json:"Match"`
	SizeDifference   int64    `json:"SizeDifference"`
	FileCountDiff    int      `json:"FileCountDiff"`
	MissingInFirst   []string `json:"MissingInFirst"`
	MissingInSecond  []string `json:"MissingInSecond"`
	DifferentContent []string `json:"DifferentContent"`
	HashMatch        bool     `json:"HashMatch"`
}

// NewOutputVerifier creates a new output verifier for the given directory
func NewOutputVerifier(directory string) *OutputVerifier {
	return &OutputVerifier{
		directory: directory,
	}
}

// Analyze analyzes the output directory and returns metrics
func (ov *OutputVerifier) Analyze() (OutputMetrics, error) {
	metrics := OutputMetrics{
		FileHashes:   make(map[string]string),
		LargestFiles: make([]FileInfo, 0),
		FileTypes:    make(map[string]int),
	}

	// Pre-allocate slices with estimated capacity to reduce reallocations
	var allHashes []string
	var allFiles []FileInfo
	allFiles = make([]FileInfo, 0, 1000) // Pre-allocate for better performance

	err := filepath.Walk(ov.directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		relPath, _ := filepath.Rel(ov.directory, path)

		if info.IsDir() {
			metrics.TotalDirs++
			return nil
		}

		metrics.TotalFiles++
		metrics.TotalSize += info.Size()

		// Count file types (optimize string operations)
		ext := filepath.Ext(path)
		if ext == "" {
			ext = "(no extension)"
		} else {
			// Convert to lowercase only once
			ext = strings.ToLower(ext)
		}
		metrics.FileTypes[ext]++

		// Identify content types (optimize string checks)
		pathLower := strings.ToLower(path)
		if strings.Contains(pathLower, "/blobs/") {
			metrics.LayerCount++
		}
		if strings.Contains(pathLower, "manifest") || strings.HasSuffix(pathLower, ".json") {
			metrics.ManifestCount++
		}
		if strings.Contains(pathLower, "signature") || strings.HasSuffix(pathLower, ".sig") {
			metrics.SignatureCount++
		}

		// Calculate file hash (for smaller files, skip very large ones for performance)
		var hash string
		if info.Size() < 100*1024*1024 { // Only hash files < 100MB
			hash, _ = hashFile(path)
			if hash != "" {
				metrics.FileHashes[relPath] = hash
				allHashes = append(allHashes, hash)
			}
		} else {
			// For large files, use size + name as pseudo-hash
			hash = fmt.Sprintf("size:%d", info.Size())
			metrics.FileHashes[relPath] = hash
			allHashes = append(allHashes, hash)
		}

		allFiles = append(allFiles, FileInfo{
			Path: relPath,
			Size: info.Size(),
			Hash: hash,
		})

		return nil
	})

	if err != nil {
		return metrics, err
	}

	// Sort to get largest files
	sort.Slice(allFiles, func(i, j int) bool {
		return allFiles[i].Size > allFiles[j].Size
	})

	// Keep top 10 largest
	if len(allFiles) > 10 {
		metrics.LargestFiles = allFiles[:10]
	} else {
		metrics.LargestFiles = allFiles
	}

	// Calculate combined directory hash
	sort.Strings(allHashes)
	combinedHash := sha256.New()
	for _, h := range allHashes {
		combinedHash.Write([]byte(h))
	}
	metrics.DirectoryHash = hex.EncodeToString(combinedHash.Sum(nil))

	return metrics, nil
}

// Compare compares two output directories (optimized with concurrent processing)
func CompareOutputs(dir1, dir2 string) (OutputComparisonResult, error) {
	result := OutputComparisonResult{
		MissingInFirst:   make([]string, 0),
		MissingInSecond:  make([]string, 0),
		DifferentContent: make([]string, 0),
	}

	verifier1 := NewOutputVerifier(dir1)
	verifier2 := NewOutputVerifier(dir2)

	// Analyze both directories concurrently
	type analyzeResult struct {
		metrics OutputMetrics
		err     error
	}
	
	resultsChan := make(chan analyzeResult, 2)
	
	go func() {
		metrics, err := verifier1.Analyze()
		resultsChan <- analyzeResult{metrics, err}
	}()
	
	go func() {
		metrics, err := verifier2.Analyze()
		resultsChan <- analyzeResult{metrics, err}
	}()
	
	var metrics1, metrics2 OutputMetrics
	var err1, err2 error
	
	// Collect results
	for i := 0; i < 2; i++ {
		res := <-resultsChan
		if i == 0 {
			metrics1, err1 = res.metrics, res.err
		} else {
			metrics2, err2 = res.metrics, res.err
		}
	}
	
	if err1 != nil {
		return result, fmt.Errorf("failed to analyze %s: %w", dir1, err1)
	}
	if err2 != nil {
		return result, fmt.Errorf("failed to analyze %s: %w", dir2, err2)
	}

	result.SizeDifference = metrics1.TotalSize - metrics2.TotalSize
	result.FileCountDiff = metrics1.TotalFiles - metrics2.TotalFiles
	result.HashMatch = metrics1.DirectoryHash == metrics2.DirectoryHash

	// Pre-allocate slices with estimated capacity
	missingInSecond := make([]string, 0, len(metrics1.FileHashes)/10)
	missingInFirst := make([]string, 0, len(metrics2.FileHashes)/10)
	differentContent := make([]string, 0, len(metrics1.FileHashes)/10)

	// Find missing files and different content in a single pass
	for path, hash1 := range metrics1.FileHashes {
		if hash2, exists := metrics2.FileHashes[path]; exists {
			if hash1 != hash2 {
				differentContent = append(differentContent, path)
			}
		} else {
			missingInSecond = append(missingInSecond, path)
		}
	}

	for path := range metrics2.FileHashes {
		if _, exists := metrics1.FileHashes[path]; !exists {
			missingInFirst = append(missingInFirst, path)
		}
	}

	result.MissingInFirst = missingInFirst
	result.MissingInSecond = missingInSecond
	result.DifferentContent = differentContent

	result.Match = result.HashMatch &&
		len(result.MissingInFirst) == 0 &&
		len(result.MissingInSecond) == 0 &&
		len(result.DifferentContent) == 0

	return result, nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Use buffered I/O for better performance
	hash := sha256.New()
	buf := make([]byte, 32*1024) // 32KB buffer
	if _, err := io.CopyBuffer(hash, file, buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// PrintSummary prints a formatted summary of the output metrics
func (m *OutputMetrics) PrintSummary() {
	fmt.Printf("  │ ─── Output Analysis ──────────────────────────────────────────\n")
	fmt.Printf("  │   Total Size: %s\n", FormatBytesHuman(m.TotalSize))
	fmt.Printf("  │   Total Files: %d | Directories: %d\n", m.TotalFiles, m.TotalDirs)
	fmt.Printf("  │   Layers/Blobs: %d | Manifests: %d | Signatures: %d\n",
		m.LayerCount, m.ManifestCount, m.SignatureCount)
	fmt.Printf("  │   Directory Hash: %s...\n", m.DirectoryHash[:16])

	if len(m.LargestFiles) > 0 {
		fmt.Printf("  │   Largest Files:\n")
		for i, f := range m.LargestFiles {
			if i >= 5 {
				break
			}
			fmt.Printf("  │     %d. %s (%s)\n", i+1, truncatePath(f.Path, 40), FormatBytesHuman(f.Size))
		}
	}
}

// PrintComparisonSummary prints a comparison between two outputs
func (r *OutputComparisonResult) PrintSummary(name1, name2 string) {
	fmt.Printf("  │ ─── Output Comparison (%s vs %s) ─────────────────────\n", name1, name2)
	if r.Match {
		fmt.Printf("  │   ✓ Outputs MATCH\n")
	} else {
		fmt.Printf("  │   ✗ Outputs DIFFER\n")
	}
	fmt.Printf("  │   Size Difference: %s\n", FormatBytesHuman(abs(r.SizeDifference)))
	fmt.Printf("  │   File Count Difference: %d\n", abs64(int64(r.FileCountDiff)))
	fmt.Printf("  │   Hash Match: %v\n", r.HashMatch)

	if len(r.MissingInFirst) > 0 {
		fmt.Printf("  │   Missing in %s: %d files\n", name1, len(r.MissingInFirst))
	}
	if len(r.MissingInSecond) > 0 {
		fmt.Printf("  │   Missing in %s: %d files\n", name2, len(r.MissingInSecond))
	}
	if len(r.DifferentContent) > 0 {
		fmt.Printf("  │   Different Content: %d files\n", len(r.DifferentContent))
	}
}

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

