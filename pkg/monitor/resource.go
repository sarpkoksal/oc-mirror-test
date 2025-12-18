package monitor

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ResourceMonitor monitors CPU and memory usage during operations
type ResourceMonitor struct {
	startTime    time.Time
	stopTime     time.Time
	monitoring   bool
	samples      []ResourceSample
	mu           sync.RWMutex
	pollInterval time.Duration
	pid          int
}

// ResourceSample represents a single resource measurement
type ResourceSample struct {
	Timestamp     time.Time
	CPUPercent    float64 // CPU usage percentage
	MemoryRSS     int64   // Resident Set Size in bytes
	MemoryVMS     int64   // Virtual Memory Size in bytes
	MemoryPercent float64 // Memory usage percentage
	NumGoroutines int     // Number of goroutines (Go-specific)
	NumThreads    int     // Number of OS threads
}

// ResourceMetrics represents aggregated resource metrics
type ResourceMetrics struct {
	Duration          time.Duration
	CPUAvgPercent     float64
	CPUPeakPercent    float64
	MemoryAvgMB       float64
	MemoryPeakMB      float64
	MemoryPeakRSS     int64
	AvgGoroutines     float64
	PeakGoroutines    int
	AvgThreads        float64
	PeakThreads       int
	Samples           []ResourceSample
	SampleCount       int
}

// NewResourceMonitor creates a new resource monitor for the current process
func NewResourceMonitor() *ResourceMonitor {
	return &ResourceMonitor{
		samples:      make([]ResourceSample, 0),
		pollInterval: 1 * time.Second,
		pid:          os.Getpid(),
	}
}

// NewResourceMonitorForPID creates a new resource monitor for a specific process
func NewResourceMonitorForPID(pid int) *ResourceMonitor {
	return &ResourceMonitor{
		samples:      make([]ResourceSample, 0),
		pollInterval: 1 * time.Second,
		pid:          pid,
	}
}

// SetTargetPID changes the target PID to monitor
func (rm *ResourceMonitor) SetTargetPID(pid int) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.pid = pid
}

// GetTargetPID returns the current target PID being monitored
func (rm *ResourceMonitor) GetTargetPID() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.pid
}

// SetPollInterval sets the polling interval for monitoring
func (rm *ResourceMonitor) SetPollInterval(interval time.Duration) {
	rm.pollInterval = interval
}

// Start begins resource monitoring
func (rm *ResourceMonitor) Start() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.monitoring {
		return nil
	}

	rm.startTime = time.Now()
	rm.monitoring = true
	rm.samples = make([]ResourceSample, 0)

	go rm.monitorLoop()

	return nil
}

// Stop stops monitoring and returns the collected metrics
func (rm *ResourceMonitor) Stop() ResourceMetrics {
	rm.mu.Lock()
	rm.monitoring = false
	rm.stopTime = time.Now()
	rm.mu.Unlock()

	time.Sleep(500 * time.Millisecond)

	return rm.calculateMetrics()
}

func (rm *ResourceMonitor) monitorLoop() {
	ticker := time.NewTicker(rm.pollInterval)
	defer ticker.Stop()

	// Get initial CPU times for delta calculation
	lastCPUTime := rm.getCPUTime()
	lastSampleTime := time.Now()

	for {
		rm.mu.RLock()
		monitoring := rm.monitoring
		rm.mu.RUnlock()

		if !monitoring {
			break
		}

		select {
		case <-ticker.C:
			currentTime := time.Now()
			currentCPUTime := rm.getCPUTime()

			// Calculate CPU percentage
			cpuDelta := currentCPUTime - lastCPUTime
			timeDelta := currentTime.Sub(lastSampleTime).Seconds()
			cpuPercent := 0.0
			if timeDelta > 0 {
				// CPU time is in clock ticks, convert to percentage
				// Assume 100 clock ticks per second (standard on Linux)
				cpuPercent = (cpuDelta / timeDelta) * 100.0 / float64(runtime.NumCPU())
			}

			memRSS, memVMS := rm.getMemoryUsage()
			memPercent := rm.getMemoryPercent(memRSS)

			sample := ResourceSample{
				Timestamp:     currentTime,
				CPUPercent:    cpuPercent,
				MemoryRSS:     memRSS,
				MemoryVMS:     memVMS,
				MemoryPercent: memPercent,
				NumGoroutines: runtime.NumGoroutine(),
				NumThreads:    rm.getThreadCount(),
			}

			rm.mu.Lock()
			rm.samples = append(rm.samples, sample)
			rm.mu.Unlock()

			lastCPUTime = currentCPUTime
			lastSampleTime = currentTime
		}
	}
}

// getCPUTime reads CPU time from /proc/[pid]/stat
func (rm *ResourceMonitor) getCPUTime() float64 {
	statPath := fmt.Sprintf("/proc/%d/stat", rm.pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0
	}

	fields := strings.Fields(string(data))
	if len(fields) < 15 {
		return 0
	}

	// Fields 14 and 15 are utime and stime (user and system CPU time)
	utime, _ := strconv.ParseFloat(fields[13], 64)
	stime, _ := strconv.ParseFloat(fields[14], 64)

	// Convert from clock ticks to seconds (assuming 100 Hz)
	return (utime + stime) / 100.0
}

// getMemoryUsage reads memory usage from /proc/[pid]/status
func (rm *ResourceMonitor) getMemoryUsage() (rss int64, vms int64) {
	statusPath := fmt.Sprintf("/proc/%d/status", rm.pid)
	file, err := os.Open(statusPath)
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseInt(fields[1], 10, 64)
				rss = val * 1024 // Convert from KB to bytes
			}
		} else if strings.HasPrefix(line, "VmSize:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseInt(fields[1], 10, 64)
				vms = val * 1024 // Convert from KB to bytes
			}
		}
	}

	return rss, vms
}

// getMemoryPercent calculates memory usage as percentage of total system memory
func (rm *ResourceMonitor) getMemoryPercent(rss int64) float64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				total, _ := strconv.ParseInt(fields[1], 10, 64)
				totalBytes := total * 1024
				if totalBytes > 0 {
					return float64(rss) / float64(totalBytes) * 100.0
				}
			}
			break
		}
	}

	return 0
}

// getThreadCount reads thread count from /proc/[pid]/status
func (rm *ResourceMonitor) getThreadCount() int {
	statusPath := fmt.Sprintf("/proc/%d/status", rm.pid)
	file, err := os.Open(statusPath)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Threads:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				count, _ := strconv.Atoi(fields[1])
				return count
			}
		}
	}

	return 0
}

func (rm *ResourceMonitor) calculateMetrics() ResourceMetrics {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	metrics := ResourceMetrics{
		Duration:    rm.stopTime.Sub(rm.startTime),
		Samples:     make([]ResourceSample, len(rm.samples)),
		SampleCount: len(rm.samples),
	}

	copy(metrics.Samples, rm.samples)

	if len(rm.samples) == 0 {
		return metrics
	}

	var totalCPU, totalMem float64
	var totalGoroutines, totalThreads int

	for _, sample := range rm.samples {
		totalCPU += sample.CPUPercent
		totalMem += float64(sample.MemoryRSS)
		totalGoroutines += sample.NumGoroutines
		totalThreads += sample.NumThreads

		if sample.CPUPercent > metrics.CPUPeakPercent {
			metrics.CPUPeakPercent = sample.CPUPercent
		}
		if sample.MemoryRSS > metrics.MemoryPeakRSS {
			metrics.MemoryPeakRSS = sample.MemoryRSS
		}
		if sample.NumGoroutines > metrics.PeakGoroutines {
			metrics.PeakGoroutines = sample.NumGoroutines
		}
		if sample.NumThreads > metrics.PeakThreads {
			metrics.PeakThreads = sample.NumThreads
		}
	}

	count := float64(len(rm.samples))
	metrics.CPUAvgPercent = totalCPU / count
	metrics.MemoryAvgMB = totalMem / count / (1024 * 1024)
	metrics.MemoryPeakMB = float64(metrics.MemoryPeakRSS) / (1024 * 1024)
	metrics.AvgGoroutines = float64(totalGoroutines) / count
	metrics.AvgThreads = float64(totalThreads) / count

	return metrics
}

// PrintSummary prints a formatted summary of the resource metrics
func (m *ResourceMetrics) PrintSummary() {
	fmt.Printf("  │ ─── Resource Usage ───────────────────────────────────────────\n")
	fmt.Printf("  │   CPU Avg: %.2f%% | Peak: %.2f%%\n", m.CPUAvgPercent, m.CPUPeakPercent)
	fmt.Printf("  │   Memory Avg: %.2f MB | Peak: %.2f MB\n", m.MemoryAvgMB, m.MemoryPeakMB)
	fmt.Printf("  │   Goroutines Avg: %.0f | Peak: %d\n", m.AvgGoroutines, m.PeakGoroutines)
	fmt.Printf("  │   Threads Avg: %.0f | Peak: %d\n", m.AvgThreads, m.PeakThreads)
}

