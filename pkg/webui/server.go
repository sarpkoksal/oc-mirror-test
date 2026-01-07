package webui

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/telco-core/ngc-495/pkg/runner"
)

// Server represents the web UI server
type Server struct {
	port           int
	resultsDir     string
	cache          *resultCache
	registryMonitor *runner.RegistryMonitorInterface // Registry monitor for live metrics
}

// resultCache caches parsed results to avoid repeated file I/O
type resultCache struct {
	mu       sync.RWMutex
	entries  map[string]*cacheEntry
	maxAge   time.Duration
}

type cacheEntry struct {
	data      []runner.TestResult
	timestamp time.Time
}

func newResultCache(maxAge time.Duration) *resultCache {
	return &resultCache{
		entries: make(map[string]*cacheEntry),
		maxAge:  maxAge,
	}
}

func (c *resultCache) get(key string) ([]runner.TestResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	
	if time.Since(entry.timestamp) > c.maxAge {
		return nil, false
	}
	
	return entry.data, true
}

func (c *resultCache) set(key string, data []runner.TestResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.entries[key] = &cacheEntry{
		data:      data,
		timestamp: time.Now(),
	}
}

func (c *resultCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}

// NewServer creates a new web UI server
func NewServer(port int, resultsDir string) *Server {
	return &Server{
		port:       port,
		resultsDir: resultsDir,
		cache:      newResultCache(30 * time.Second), // Cache for 30 seconds
	}
}

// Start starts the web server
func (s *Server) Start() error {
	// Ensure results directory exists
	if err := os.MkdirAll(s.resultsDir, 0755); err != nil {
		return fmt.Errorf("failed to create results directory: %w", err)
	}

	// Register handlers
	http.HandleFunc("/", s.handleIndex)
	http.HandleFunc("/api/results", s.handleResultsList)
	http.HandleFunc("/api/results/", s.handleResultDetail)
	http.HandleFunc("/api/latest", s.handleLatestResult)
	http.HandleFunc("/api/live", s.handleLiveMetrics)
	http.HandleFunc("/api/registry", s.handleRegistryMetrics) // New endpoint for registry metrics
	http.HandleFunc("/static/", s.handleStatic)

	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("Starting web UI server on http://localhost%s", addr)
	log.Printf("Results directory: %s", s.resultsDir)
	return http.ListenAndServe(addr, nil)
}

// handleIndex serves the main HTML page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

// handleResultsList returns a list of all result files
func (s *Server) handleResultsList(w http.ResponseWriter, r *http.Request) {
	files, err := s.getResultFiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// handleResultDetail returns detailed metrics for a specific result file
func (s *Server) handleResultDetail(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimPrefix(r.URL.Path, "/api/results/")
	if filename == "" {
		http.Error(w, "filename required", http.StatusBadRequest)
		return
	}

	// Check cache first
	if results, ok := s.cache.get(filename); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		json.NewEncoder(w).Encode(results)
		return
	}

	filepath := filepath.Join(s.resultsDir, filename)
	data, err := os.ReadFile(filepath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var results []runner.TestResult
	if err := json.Unmarshal(data, &results); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Cache the results
	s.cache.set(filename, results)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	json.NewEncoder(w).Encode(results)
}

// handleLiveMetrics returns the most recent result with live updates
func (s *Server) handleLiveMetrics(w http.ResponseWriter, r *http.Request) {
	// Enable CORS for live updates
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	
	// Get latest result
	files, err := s.getResultFiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(files) == 0 {
		// Return empty result if no files yet
		json.NewEncoder(w).Encode([]runner.TestResult{})
		return
	}

	// Get the latest file
	latestFile := files[len(files)-1].Filename
	
	// Check cache first
	if results, ok := s.cache.get("latest"); ok {
		// Verify it's still the latest
		if len(files) > 0 && files[len(files)-1].Filename == latestFile {
			w.Header().Set("X-Cache", "HIT")
			json.NewEncoder(w).Encode(results)
			return
		}
	}

	filepath := filepath.Join(s.resultsDir, latestFile)
	data, err := os.ReadFile(filepath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var results []runner.TestResult
	if err := json.Unmarshal(data, &results); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Cache the results
	s.cache.set("latest", results)
	s.cache.set(latestFile, results)

	w.Header().Set("X-Cache", "MISS")
	json.NewEncoder(w).Encode(results)
}

// handleRegistryMetrics returns current registry upload metrics from the daemon
func (s *Server) handleRegistryMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	
	if s.registryMonitor == nil || *s.registryMonitor == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"monitoring": false,
			"message": "Registry monitor not available",
		})
		return
	}
	
	monitor := *s.registryMonitor
	if !monitor.IsMonitoring() {
		// Return empty metrics if not monitoring
		json.NewEncoder(w).Encode(map[string]interface{}{
			"monitoring": false,
			"message": "Registry monitor not active",
		})
		return
	}
	
	metrics := monitor.GetCurrentMetrics()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"monitoring": true,
		"metrics": metrics,
	})
}

// SetRegistryMonitor sets the registry monitor for live metrics
func (s *Server) SetRegistryMonitor(monitor runner.RegistryMonitorInterface) {
	s.registryMonitor = &monitor
}

// handleLatestResult returns the most recent result
func (s *Server) handleLatestResult(w http.ResponseWriter, r *http.Request) {
	files, err := s.getResultFiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(files) == 0 {
		http.Error(w, "no results found", http.StatusNotFound)
		return
	}

	// Get the latest file
	latestFile := files[len(files)-1].Filename
	
	// Check cache first
	if results, ok := s.cache.get("latest"); ok {
		// Verify it's still the latest
		if len(files) > 0 && files[len(files)-1].Filename == latestFile {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			json.NewEncoder(w).Encode(results)
			return
		}
	}

	filepath := filepath.Join(s.resultsDir, latestFile)
	data, err := os.ReadFile(filepath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var results []runner.TestResult
	if err := json.Unmarshal(data, &results); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Cache the results
	s.cache.set("latest", results)
	s.cache.set(latestFile, results)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	json.NewEncoder(w).Encode(results)
}

// ResultFileInfo represents information about a result file
type ResultFileInfo struct {
	Filename    string    `json:"filename"`
	ModTime     time.Time `json:"mod_time"`
	ModTimeStr  string    `json:"mod_time_str"`
	ResultCount int       `json:"result_count"`
}

// getResultFiles returns a list of all result JSON files
func (s *Server) getResultFiles() ([]ResultFileInfo, error) {
	var files []ResultFileInfo

	entries, err := os.ReadDir(s.resultsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if !strings.HasPrefix(entry.Name(), "results_") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Count results in file
		filepath := filepath.Join(s.resultsDir, entry.Name())
		data, err := os.ReadFile(filepath)
		if err != nil {
			continue
		}

		var results []runner.TestResult
		if err := json.Unmarshal(data, &results); err != nil {
			continue
		}

		files = append(files, ResultFileInfo{
			Filename:    entry.Name(),
			ModTime:     info.ModTime(),
			ModTimeStr:  info.ModTime().Format("2006-01-02 15:04:05"),
			ResultCount: len(results),
		})
	}

	// Sort by modification time (oldest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.Before(files[j].ModTime)
	})

	return files, nil
}

// handleStatic serves static files (CSS, JS)
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	
	switch path {
	case "app.js":
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, appJS)
	case "styles.css":
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, stylesCSS)
	default:
		http.NotFound(w, r)
	}
}

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>OC Mirror Test Metrics Dashboard</title>
    <link rel="stylesheet" href="/static/styles.css">
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
</head>
<body>
    <div class="container">
        <header>
            <h1>OC Mirror Test Metrics Dashboard</h1>
            <div class="controls">
                <select id="resultSelect">
                    <option value="">Loading results...</option>
                </select>
                <button id="refreshBtn">Refresh</button>
                <button id="autoRefreshBtn">Auto-refresh: OFF</button>
            </div>
        </header>

        <div id="status" class="status-info" style="display: none;">
            <span id="statusText">Monitoring test execution...</span>
        </div>

        <div id="loading" class="loading">Loading metrics...</div>
        <div id="error" class="error" style="display: none;"></div>
        <div id="content" style="display: none;">
            <div class="metrics-grid">
                <div class="metric-card">
                    <h3>Timing Metrics</h3>
                    <div class="metric-item">
                        <span class="label">Download Time:</span>
                        <span class="value" id="downloadTime">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Upload Time:</span>
                        <span class="value" id="uploadTime">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Total Time:</span>
                        <span class="value" id="totalTime">-</span>
                    </div>
                </div>

                <div class="metric-card">
                    <h3>Data Transfer</h3>
                    <div class="metric-item">
                        <span class="label">Downloaded:</span>
                        <span class="value" id="downloaded">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Uploaded:</span>
                        <span class="value" id="uploaded">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Avg Speed:</span>
                        <span class="value" id="avgSpeed">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Peak Speed:</span>
                        <span class="value" id="peakSpeed">-</span>
                    </div>
                </div>

                <div class="metric-card">
                    <h3>Resource Usage</h3>
                    <div class="metric-item">
                        <span class="label">CPU Avg:</span>
                        <span class="value" id="cpuAvg">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">CPU Peak:</span>
                        <span class="value" id="cpuPeak">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Memory Avg:</span>
                        <span class="value" id="memAvg">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Memory Peak:</span>
                        <span class="value" id="memPeak">-</span>
                    </div>
                </div>

                <div class="metric-card">
                    <h3>Network</h3>
                    <div class="metric-item">
                        <span class="label">Avg Bandwidth:</span>
                        <span class="value" id="netAvg">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Peak Bandwidth:</span>
                        <span class="value" id="netPeak">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Total Transferred:</span>
                        <span class="value" id="netTotal">-</span>
                    </div>
                </div>

                <div class="metric-card">
                    <h3>Registry Upload (Live)</h3>
                    <div class="metric-item">
                        <span class="label">Total Uploaded:</span>
                        <span class="value" id="registryTotal">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Avg Upload Rate:</span>
                        <span class="value" id="registryAvg">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Peak Upload Rate:</span>
                        <span class="value" id="registryPeak">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Active Connections:</span>
                        <span class="value" id="registryConnections">-</span>
                    </div>
                </div>

                <div class="metric-card">
                    <h3>Mirror Content</h3>
                    <div class="metric-item">
                        <span class="label">Images:</span>
                        <span class="value" id="images">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Layers:</span>
                        <span class="value" id="layers">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Manifests:</span>
                        <span class="value" id="manifests">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Files:</span>
                        <span class="value" id="files">-</span>
                    </div>
                </div>

                <div class="metric-card">
                    <h3>Cache & Performance</h3>
                    <div class="metric-item">
                        <span class="label">Cache Hits:</span>
                        <span class="value" id="cacheHits">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Images Skipped:</span>
                        <span class="value" id="imagesSkipped">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Errors:</span>
                        <span class="value" id="errors">-</span>
                    </div>
                    <div class="metric-item">
                        <span class="label">Retries:</span>
                        <span class="value" id="retries">-</span>
                    </div>
                </div>
            </div>

            <div class="charts-section">
                <div class="chart-container">
                    <canvas id="speedChart"></canvas>
                </div>
                <div class="chart-container">
                    <canvas id="resourceChart"></canvas>
                </div>
                <div class="chart-container">
                    <canvas id="networkChart"></canvas>
                </div>
            </div>

            <div id="iterations" class="iterations-section"></div>
        </div>
    </div>
    <script src="/static/app.js"></script>
</body>
</html>`

const stylesCSS = `* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
    background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
    min-height: 100vh;
    padding: 20px;
    color: #333;
}

.container {
    max-width: 1400px;
    margin: 0 auto;
}

header {
    background: white;
    padding: 20px 30px;
    border-radius: 10px;
    box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
    margin-bottom: 20px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    flex-wrap: wrap;
    gap: 15px;
}

header h1 {
    color: #667eea;
    font-size: 28px;
}

.controls {
    display: flex;
    gap: 10px;
    align-items: center;
}

.controls select {
    padding: 8px 12px;
    border: 2px solid #ddd;
    border-radius: 5px;
    font-size: 14px;
    min-width: 200px;
}

.controls button {
    padding: 8px 16px;
    background: #667eea;
    color: white;
    border: none;
    border-radius: 5px;
    cursor: pointer;
    font-size: 14px;
    transition: background 0.3s;
}

.controls button:hover {
    background: #5568d3;
}

.controls button.active {
    background: #48bb78;
}

.loading, .error {
    background: white;
    padding: 30px;
    border-radius: 10px;
    text-align: center;
    box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
    margin-bottom: 20px;
}

.error {
    background: #fed7d7;
    color: #c53030;
}

.status-info {
    background: #e6f3ff;
    border-left: 4px solid #667eea;
    padding: 12px 20px;
    margin-bottom: 20px;
    border-radius: 5px;
    color: #2c5282;
    font-weight: 500;
}

.metrics-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
    gap: 20px;
    margin-bottom: 30px;
}

.metric-card {
    background: white;
    padding: 20px;
    border-radius: 10px;
    box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
}

.metric-card h3 {
    color: #667eea;
    margin-bottom: 15px;
    font-size: 18px;
    border-bottom: 2px solid #e2e8f0;
    padding-bottom: 10px;
}

.metric-item {
    display: flex;
    justify-content: space-between;
    padding: 8px 0;
    border-bottom: 1px solid #f0f0f0;
}

.metric-item:last-child {
    border-bottom: none;
}

.metric-item .label {
    color: #666;
    font-weight: 500;
}

.metric-item .value {
    color: #333;
    font-weight: 600;
}

.charts-section {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
    gap: 20px;
    margin-bottom: 30px;
}

.chart-container {
    background: white;
    padding: 20px;
    border-radius: 10px;
    box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
    height: 300px;
}

.iterations-section {
    background: white;
    padding: 20px;
    border-radius: 10px;
    box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
}

.iteration-card {
    border: 2px solid #e2e8f0;
    border-radius: 8px;
    padding: 15px;
    margin-bottom: 15px;
}

.iteration-card h4 {
    color: #667eea;
    margin-bottom: 10px;
    display: flex;
    align-items: center;
    gap: 10px;
}

.badge {
    display: inline-block;
    padding: 4px 8px;
    border-radius: 4px;
    font-size: 12px;
    font-weight: 600;
}

.badge.clean {
    background: #c6f6d5;
    color: #22543d;
}

.badge.cached {
    background: #fed7aa;
    color: #7c2d12;
}

.badge.v1 {
    background: #bee3f8;
    color: #2c5282;
}

.badge.v2 {
    background: #fbb6ce;
    color: #702459;
}

@media (max-width: 768px) {
    header {
        flex-direction: column;
        align-items: flex-start;
    }
    
    .metrics-grid {
        grid-template-columns: 1fr;
    }
    
    .charts-section {
        grid-template-columns: 1fr;
    }
}`

const appJS = `
let autoRefreshInterval = null;
let speedChart = null;
let resourceChart = null;
let networkChart = null;

// Format duration
function formatDuration(seconds) {
    if (!seconds) return '-';
    const s = Math.floor(seconds);
    const hours = Math.floor(s / 3600);
    const minutes = Math.floor((s % 3600) / 60);
    const secs = s % 60;
    if (hours > 0) {
        return hours + 'h ' + minutes + 'm ' + secs + 's';
    } else if (minutes > 0) {
        return minutes + 'm ' + secs + 's';
    }
    return secs + 's';
}

// Format bytes
function formatBytes(bytes) {
    if (!bytes) return '-';
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    if (bytes === 0) return '0 B';
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return Math.round(bytes / Math.pow(1024, i) * 100) / 100 + ' ' + sizes[i];
}

// Load results list
async function loadResultsList() {
    try {
        const response = await fetch('/api/results');
        const files = await response.json();
        const select = document.getElementById('resultSelect');
        select.innerHTML = '';
        
        if (files.length === 0) {
            select.innerHTML = '<option value="">No results found</option>';
            return;
        }
        
        // Add latest option
        const latestOption = document.createElement('option');
        latestOption.value = 'latest';
        latestOption.textContent = 'Latest Results';
        select.appendChild(latestOption);
        
        // Add individual files
        files.forEach(file => {
            const option = document.createElement('option');
            option.value = file.filename;
            option.textContent = file.mod_time_str + ' (' + file.result_count + ' results)';
            select.appendChild(option);
        });
        
        // Select latest by default
        select.value = 'latest';
        loadResultData('latest', true); // Use live endpoint for initial load
    } catch (error) {
        showError('Failed to load results list: ' + error.message);
    }
}

// Load registry metrics
async function loadRegistryMetrics() {
    try {
        const response = await fetch('/api/registry');
        if (!response.ok) {
            // Registry monitor not available or not monitoring
            document.getElementById('registryTotal').textContent = '-';
            document.getElementById('registryAvg').textContent = '-';
            document.getElementById('registryPeak').textContent = '-';
            document.getElementById('registryConnections').textContent = '-';
            return;
        }
        const data = await response.json();
        if (data.monitoring && data.metrics) {
            const metrics = data.metrics;
            document.getElementById('registryTotal').textContent = formatBytes(metrics.TotalBytesUploaded || 0);
            document.getElementById('registryAvg').textContent = (metrics.AverageUploadRateMB || 0).toFixed(2) + ' MB/s';
            document.getElementById('registryPeak').textContent = (metrics.PeakUploadRateMB || 0).toFixed(2) + ' MB/s';
            document.getElementById('registryConnections').textContent = metrics.ConnectionCount || 0;
        } else {
            document.getElementById('registryTotal').textContent = '-';
            document.getElementById('registryAvg').textContent = '-';
            document.getElementById('registryPeak').textContent = '-';
            document.getElementById('registryConnections').textContent = '-';
        }
    } catch (error) {
        // Silently fail - registry monitor may not be available
        console.log('Registry metrics not available:', error);
    }
}

// Load result data
async function loadResultData(filename, useLive = false) {
    const loading = document.getElementById('loading');
    const content = document.getElementById('content');
    const errorDiv = document.getElementById('error');
    const statusDiv = document.getElementById('status');
    const statusText = document.getElementById('statusText');
    
    // Use live endpoint for latest when auto-refresh is on or explicitly requested
    const useLiveEndpoint = useLive || (filename === 'latest' && autoRefreshInterval !== null);
    
    if (useLiveEndpoint && filename === 'latest') {
        statusDiv.style.display = 'block';
        statusText.textContent = '🔄 Live monitoring active - Refreshing every 2 seconds...';
        // Also load registry metrics when in live mode
        loadRegistryMetrics();
    } else {
        statusDiv.style.display = 'none';
    }
    
    loading.style.display = 'block';
    content.style.display = 'none';
    errorDiv.style.display = 'none';
    
    try {
        const url = useLiveEndpoint && filename === 'latest' ? '/api/live' : 
                   (filename === 'latest' ? '/api/latest' : '/api/results/' + filename);
        const response = await fetch(url);
        if (!response.ok) {
            if (response.status === 404 && filename === 'latest') {
                // No results yet, show waiting message
                loading.textContent = '⏳ Waiting for test results to be generated...';
                statusText.textContent = '⏳ Waiting for test execution to start...';
                return;
            }
            throw new Error('Failed to load result data');
        }
        const results = await response.json();
        if (results && results.length > 0) {
            displayResults(results);
            loading.style.display = 'none';
            content.style.display = 'block';
            if (useLiveEndpoint) {
                statusText.textContent = '✅ Live monitoring active - Latest results displayed';
            } else {
                statusDiv.style.display = 'none';
            }
        } else {
            // No results yet, keep loading state
            loading.textContent = '⏳ Waiting for test results...';
            statusText.textContent = '⏳ Waiting for test execution to complete...';
        }
    } catch (error) {
        loading.style.display = 'none';
        if (error.message.includes('Failed to load') || error.message.includes('404')) {
            // No results file yet, show waiting message
            showError('⏳ Waiting for test results to be generated...');
            statusText.textContent = '⏳ Waiting for test execution to start...';
        } else {
            showError('Failed to load result data: ' + error.message);
            statusDiv.style.display = 'none';
        }
    }
}

// Display results
function displayResults(results) {
    if (!results || results.length === 0) {
        showError('No results found');
        return;
    }
    
    // Aggregate metrics from all iterations
    let totalDownloadTime = 0;
    let totalUploadTime = 0;
    let totalDownloaded = 0;
    let totalUploaded = 0;
    let totalCacheHits = 0;
    let totalImagesSkipped = 0;
    let totalErrors = 0;
    let totalRetries = 0;
    
    let cpuAvgSum = 0;
    let cpuPeakMax = 0;
    let memAvgSum = 0;
    let memPeakMax = 0;
    let netAvgSum = 0;
    let netPeakMax = 0;
    let netTotalSum = 0;
    
    let avgSpeedSum = 0;
    let peakSpeedMax = 0;
    
    let totalImages = 0;
    let totalLayers = 0;
    let totalManifests = 0;
    let totalFiles = 0;
    
    let speedData = [];
    let resourceData = [];
    let networkData = [];
    
    results.forEach((result, index) => {
        // Timing
        const downloadTime = result.download_phase.wall_time_seconds || 0;
        const uploadTime = result.upload_phase.wall_time_seconds || 0;
        totalDownloadTime += downloadTime;
        totalUploadTime += uploadTime;
        
        // Data transfer
        const downloaded = result.download_phase.download_metrics?.TotalBytesDownloaded || 0;
        const uploaded = result.upload_phase.bytes_uploaded || 0;
        totalDownloaded += downloaded;
        totalUploaded += uploaded;
        
        // Speed
        const avgSpeed = result.download_phase.download_metrics?.AverageSpeedMBs || 0;
        const peakSpeed = result.download_phase.download_metrics?.PeakSpeedMBs || 0;
        avgSpeedSum += avgSpeed;
        if (peakSpeed > peakSpeedMax) peakSpeedMax = peakSpeed;
        
        // Resources
        const cpuAvg = result.resource_metrics?.CPUAvgPercent || 0;
        const cpuPeak = result.resource_metrics?.CPUPeakPercent || 0;
        const memAvg = result.resource_metrics?.MemoryAvgMB || 0;
        const memPeak = result.resource_metrics?.MemoryPeakMB || 0;
        cpuAvgSum += cpuAvg;
        if (cpuPeak > cpuPeakMax) cpuPeakMax = cpuPeak;
        memAvgSum += memAvg;
        if (memPeak > memPeakMax) memPeakMax = memPeak;
        
        // Network
        const netAvg = result.network_metrics?.AverageBandwidthMbps || 0;
        const netPeak = result.network_metrics?.PeakBandwidthMbps || 0;
        const netTotal = result.network_metrics?.TotalBytesTransferred || 0;
        netAvgSum += netAvg;
        if (netPeak > netPeakMax) netPeakMax = netPeak;
        netTotalSum += netTotal;
        
        // Cache & performance
        totalCacheHits += result.download_phase.cache_hits || 0;
        totalImagesSkipped += result.download_phase.images_skipped || 0;
        totalErrors += (result.download_phase.extended_metrics?.ErrorCount || 0) + 
                      (result.upload_phase.extended_metrics?.ErrorCount || 0);
        totalRetries += (result.download_phase.extended_metrics?.RetryCount || 0) + 
                       (result.upload_phase.extended_metrics?.RetryCount || 0);
        
        // Mirror content (use first result with describe metrics)
        if (result.describe_metrics && totalImages === 0) {
            totalImages = result.describe_metrics.TotalImages || 0;
            totalLayers = result.describe_metrics.TotalLayers || 0;
            totalManifests = result.describe_metrics.TotalManifests || 0;
        }
        
        if (result.output_metrics && totalFiles === 0) {
            totalFiles = result.output_metrics.TotalFiles || 0;
        }
        
        // Chart data
        speedData.push({
            x: 'Iteration ' + result.iteration,
            avg: avgSpeed,
            peak: peakSpeed
        });
        
        resourceData.push({
            x: 'Iteration ' + result.iteration,
            cpu: cpuAvg,
            mem: memAvg
        });
        
        networkData.push({
            x: 'Iteration ' + result.iteration,
            avg: netAvg,
            peak: netPeak
        });
    });
    
    const count = results.length;
    
    // Update metrics display
    document.getElementById('downloadTime').textContent = formatDuration(totalDownloadTime / count);
    document.getElementById('uploadTime').textContent = formatDuration(totalUploadTime / count);
    document.getElementById('totalTime').textContent = formatDuration((totalDownloadTime + totalUploadTime) / count);
    
    document.getElementById('downloaded').textContent = formatBytes(totalDownloaded);
    document.getElementById('uploaded').textContent = formatBytes(totalUploaded);
    document.getElementById('avgSpeed').textContent = (avgSpeedSum / count).toFixed(2) + ' MB/s';
    document.getElementById('peakSpeed').textContent = peakSpeedMax.toFixed(2) + ' MB/s';
    
    document.getElementById('cpuAvg').textContent = (cpuAvgSum / count).toFixed(2) + '%';
    document.getElementById('cpuPeak').textContent = cpuPeakMax.toFixed(2) + '%';
    document.getElementById('memAvg').textContent = (memAvgSum / count).toFixed(2) + ' MB';
    document.getElementById('memPeak').textContent = memPeakMax.toFixed(2) + ' MB';
    
    document.getElementById('netAvg').textContent = (netAvgSum / count).toFixed(2) + ' Mbps';
    document.getElementById('netPeak').textContent = netPeakMax.toFixed(2) + ' Mbps';
    document.getElementById('netTotal').textContent = formatBytes(netTotalSum);
    
    document.getElementById('images').textContent = totalImages;
    document.getElementById('layers').textContent = totalLayers;
    document.getElementById('manifests').textContent = totalManifests;
    document.getElementById('files').textContent = totalFiles;
    
    document.getElementById('cacheHits').textContent = totalCacheHits;
    document.getElementById('imagesSkipped').textContent = totalImagesSkipped;
    document.getElementById('errors').textContent = totalErrors;
    document.getElementById('retries').textContent = totalRetries;
    
    // Update charts
    updateCharts(speedData, resourceData, networkData);
    
    // Display iterations
    displayIterations(results);
}

// Update charts
function updateCharts(speedData, resourceData, networkData) {
    // Speed chart
    const speedCtx = document.getElementById('speedChart').getContext('2d');
    if (speedChart) speedChart.destroy();
    speedChart = new Chart(speedCtx, {
        type: 'bar',
        data: {
            labels: speedData.map(d => d.x),
            datasets: [{
                label: 'Avg Speed (MB/s)',
                data: speedData.map(d => d.avg),
                backgroundColor: 'rgba(102, 126, 234, 0.6)'
            }, {
                label: 'Peak Speed (MB/s)',
                data: speedData.map(d => d.peak),
                backgroundColor: 'rgba(118, 75, 162, 0.6)'
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                y: { beginAtZero: true }
            }
        }
    });
    
    // Resource chart
    const resourceCtx = document.getElementById('resourceChart').getContext('2d');
    if (resourceChart) resourceChart.destroy();
    resourceChart = new Chart(resourceCtx, {
        type: 'line',
        data: {
            labels: resourceData.map(d => d.x),
            datasets: [{
                label: 'CPU Avg (%)',
                data: resourceData.map(d => d.cpu),
                borderColor: 'rgb(102, 126, 234)',
                backgroundColor: 'rgba(102, 126, 234, 0.1)',
                tension: 0.4
            }, {
                label: 'Memory Avg (MB)',
                data: resourceData.map(d => d.mem),
                borderColor: 'rgb(118, 75, 162)',
                backgroundColor: 'rgba(118, 75, 162, 0.1)',
                tension: 0.4,
                yAxisID: 'y1'
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                y: { beginAtZero: true },
                y1: { beginAtZero: true, position: 'right' }
            }
        }
    });
    
    // Network chart
    const networkCtx = document.getElementById('networkChart').getContext('2d');
    if (networkChart) networkChart.destroy();
    networkChart = new Chart(networkCtx, {
        type: 'bar',
        data: {
            labels: networkData.map(d => d.x),
            datasets: [{
                label: 'Avg Bandwidth (Mbps)',
                data: networkData.map(d => d.avg),
                backgroundColor: 'rgba(72, 187, 120, 0.6)'
            }, {
                label: 'Peak Bandwidth (Mbps)',
                data: networkData.map(d => d.peak),
                backgroundColor: 'rgba(245, 101, 101, 0.6)'
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                y: { beginAtZero: true }
            }
        }
    });
}

// Display iterations
function displayIterations(results) {
    const container = document.getElementById('iterations');
    container.innerHTML = '<h2>Iterations</h2>';
    
    results.forEach(result => {
        const card = document.createElement('div');
        card.className = 'iteration-card';
        
        const badges = [];
        badges.push(result.is_clean_run ? '<span class="badge clean">CLEAN</span>' : '<span class="badge cached">CACHED</span>');
        badges.push('<span class="badge ' + result.version + '">' + result.version.toUpperCase() + '</span>');
        
        card.innerHTML = 
            '<h4>Iteration ' + result.iteration + ' ' + badges.join(' ') + '</h4>' +
            '<div class="metric-item"><span class="label">Download:</span><span class="value">' + formatDuration(result.download_phase.wall_time_seconds) + '</span></div>' +
            '<div class="metric-item"><span class="label">Upload:</span><span class="value">' + formatDuration(result.upload_phase.wall_time_seconds) + '</span></div>' +
            '<div class="metric-item"><span class="label">Downloaded:</span><span class="value">' + formatBytes(result.download_phase.download_metrics?.TotalBytesDownloaded) + '</span></div>' +
            '<div class="metric-item"><span class="label">Cache Hits:</span><span class="value">' + (result.download_phase.cache_hits || 0) + '</span></div>';
        
        container.appendChild(card);
    });
}

// Show error
function showError(message) {
    const errorDiv = document.getElementById('error');
    errorDiv.textContent = message;
    errorDiv.style.display = 'block';
}

// Toggle auto-refresh
function toggleAutoRefresh() {
    const btn = document.getElementById('autoRefreshBtn');
    if (autoRefreshInterval) {
        clearInterval(autoRefreshInterval);
        autoRefreshInterval = null;
        btn.textContent = 'Auto-refresh: OFF';
        btn.classList.remove('active');
    } else {
        // Use shorter interval for live updates (2 seconds)
        autoRefreshInterval = setInterval(() => {
            const select = document.getElementById('resultSelect');
            const filename = select.value || 'latest';
            loadResultData(filename, true); // Use live endpoint
            loadRegistryMetrics(); // Also refresh registry metrics
        }, 2000);
        btn.textContent = 'Auto-refresh: ON';
        btn.classList.add('active');
    }
}

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    loadResultsList();
    
    // Auto-enable auto-refresh on page load for live monitoring
    setTimeout(() => {
        if (autoRefreshInterval === null) {
            toggleAutoRefresh();
        }
    }, 1000);
    
    document.getElementById('refreshBtn').addEventListener('click', () => {
        const select = document.getElementById('resultSelect');
        loadResultData(select.value || 'latest', true);
    });
    
    document.getElementById('autoRefreshBtn').addEventListener('click', toggleAutoRefresh);
    
    document.getElementById('resultSelect').addEventListener('change', (e) => {
        loadResultData(e.target.value || 'latest');
    });
});
`

