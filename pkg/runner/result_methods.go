package runner

import (
	"fmt"
	"time"

	"github.com/telco-core/ngc-495/pkg/monitor"
)

// TestResult methods

// GetTotalTime returns the total time for download and upload phases
func (tr *TestResult) GetTotalTime() time.Duration {
	return tr.DownloadPhase.WallTime + tr.UploadPhase.WallTime
}

// GetTotalBytes returns the total bytes transferred (downloaded + uploaded)
func (tr *TestResult) GetTotalBytes() int64 {
	return tr.DownloadPhase.DownloadMetrics.TotalBytesDownloaded + tr.UploadPhase.BytesUploaded
}

// GetAverageSpeedMBs calculates average speed across both phases
func (tr *TestResult) GetAverageSpeedMBs() float64 {
	totalTime := tr.GetTotalTime().Seconds()
	if totalTime > 0 {
		return float64(tr.GetTotalBytes()) / totalTime / (1024 * 1024)
	}
	return 0
}

// GetCacheEfficiency returns cache hit ratio
func (tr *TestResult) GetCacheEfficiency() float64 {
	totalOps := tr.DownloadPhase.CacheHits + tr.DownloadPhase.ImagesSkipped
	if totalOps > 0 {
		return float64(tr.DownloadPhase.CacheHits) / float64(totalOps)
	}
	return 0
}

// GetSuccessRate returns success rate based on errors
func (tr *TestResult) GetSuccessRate() float64 {
	totalErrors := tr.DownloadPhase.ExtendedMetrics.ErrorCount + tr.UploadPhase.ExtendedMetrics.ErrorCount
	if totalErrors == 0 {
		return 1.0
	}
	// Calculate based on operations attempted
	totalOps := tr.DownloadPhase.ExtendedMetrics.ImagesProcessed + tr.UploadPhase.ExtendedMetrics.ImagesProcessed
	if totalOps > 0 {
		return 1.0 - (float64(totalErrors) / float64(totalOps))
	}
	return 0
}

// Format returns a human-readable summary
func (tr *TestResult) Format() string {
	return fmt.Sprintf("Iteration %d (%s, %s): Total=%v, Downloaded=%s, Uploaded=%s, CacheHits=%d",
		tr.Iteration,
		map[bool]string{true: "CLEAN", false: "CACHED"}[tr.IsCleanRun],
		tr.Version,
		tr.GetTotalTime(),
		monitor.FormatBytesHuman(tr.DownloadPhase.DownloadMetrics.TotalBytesDownloaded),
		monitor.FormatBytesHuman(tr.UploadPhase.BytesUploaded),
		tr.DownloadPhase.CacheHits)
}

// GetPerformanceScore returns a normalized performance score (0-100)
func (tr *TestResult) GetPerformanceScore() float64 {
	score := 0.0
	
	// Speed component (0-40 points)
	avgSpeed := tr.GetAverageSpeedMBs()
	if avgSpeed > 100 {
		score += 40
	} else if avgSpeed > 50 {
		score += 30
	} else if avgSpeed > 25 {
		score += 20
	} else if avgSpeed > 10 {
		score += 10
	}
	
	// Cache efficiency (0-30 points)
	cacheEff := tr.GetCacheEfficiency()
	score += cacheEff * 30
	
	// Success rate (0-30 points)
	successRate := tr.GetSuccessRate()
	score += successRate * 30
	
	return score
}

// PhaseMetrics methods

// GetTotalBytes returns total bytes for the phase
func (pm *PhaseMetrics) GetTotalBytes() int64 {
	if pm.DownloadMetrics.TotalBytesDownloaded > 0 {
		return pm.DownloadMetrics.TotalBytesDownloaded
	}
	return pm.BytesUploaded
}

// GetAverageSpeedMBs returns average speed for the phase
func (pm *PhaseMetrics) GetAverageSpeedMBs() float64 {
	if pm.DownloadMetrics.AverageSpeedMBs > 0 {
		return pm.DownloadMetrics.AverageSpeedMBs
	}
	// Calculate from bytes and time
	if pm.WallTime.Seconds() > 0 {
		return float64(pm.BytesUploaded) / pm.WallTime.Seconds() / (1024 * 1024)
	}
	return 0
}

// GetEfficiency returns efficiency metrics
func (pm *PhaseMetrics) GetEfficiency() float64 {
	if pm.DownloadMetrics.PeakSpeedMBs > 0 {
		return pm.DownloadMetrics.AverageSpeedMBs / pm.DownloadMetrics.PeakSpeedMBs
	}
	return 0
}

// Format returns a human-readable summary
func (pm *PhaseMetrics) Format() string {
	return fmt.Sprintf("Time: %v | Bytes: %s | Speed: %.2f MB/s | Cache Hits: %d",
		pm.WallTime,
		monitor.FormatBytesHuman(pm.GetTotalBytes()),
		pm.GetAverageSpeedMBs(),
		pm.CacheHits)
}

// ComparisonResult methods

// GetTotalImprovement returns overall improvement percentage
func (cr *ComparisonResult) GetTotalImprovement() float64 {
	return (cr.DownloadTimeDiffPct + cr.UploadTimeDiffPct) / 2.0
}

// Format returns a human-readable comparison
func (cr *ComparisonResult) Format() string {
	return fmt.Sprintf("Type: %s | Download: %.2f%% | Upload: %.2f%% | Overall: %.2f%%",
		cr.Type,
		cr.DownloadTimeDiffPct,
		cr.UploadTimeDiffPct,
		cr.GetTotalImprovement())
}

// IsImprovement returns true if the comparison shows improvement
func (cr *ComparisonResult) IsImprovement() bool {
	return cr.DownloadTimeDiffPct > 0 || cr.UploadTimeDiffPct > 0
}




