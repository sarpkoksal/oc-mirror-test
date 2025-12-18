package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// MirrorMetadata represents the JSON output from oc-mirror describe
type MirrorMetadata struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	UID        string `json:"uid"`
	SingleUse  bool   `json:"singleUse"`
	PastMirror struct {
		Timestamp    int64 `json:"timestamp"`
		Sequence     int   `json:"sequence"`
		Mirror       MirrorConfig `json:"mirror"`
		Operators    []OperatorInfo `json:"operators"`
		Associations []Association `json:"associations"`
	} `json:"pastMirror"`
}

// MirrorConfig contains mirror configuration
type MirrorConfig struct {
	Platform  interface{} `json:"platform"`
	Operators []struct {
		Packages []struct {
			Name     string `json:"name"`
			Channels []struct {
				Name       string `json:"name"`
				MinVersion string `json:"minVersion"`
				MaxVersion string `json:"maxVersion"`
			} `json:"channels"`
		} `json:"packages"`
		Catalog string `json:"catalog"`
	} `json:"operators"`
	Helm interface{} `json:"helm"`
}

// OperatorInfo contains operator package information
type OperatorInfo struct {
	Catalog  string `json:"catalog"`
	ImagePin string `json:"imagePin"`
	Packages []struct {
		Name     string `json:"name"`
		Channels []struct {
			Name       string `json:"name"`
			MinVersion string `json:"minVersion"`
			MaxVersion string `json:"maxVersion"`
		} `json:"channels"`
	} `json:"packages"`
}

// Association represents an image association in the mirror
type Association struct {
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	ID              string   `json:"id"`
	TagSymlink      string   `json:"tagSymlink"`
	Type            string   `json:"type"`
	LayerDigests    []string `json:"layerDigests,omitempty"`
	ManifestDigests []string `json:"manifestDigests,omitempty"`
}

// DescribeMetrics contains metrics extracted from oc-mirror describe
type DescribeMetrics struct {
	TotalImages       int      // Images with registry.redhat.io prefix (actual images)
	TotalManifests    int      // Total manifest entries
	TotalLayers       int      // Total unique layers
	TotalAssociations int      // Total associations
	OperatorPackages  int      // Number of operator packages
	Catalogs          []string // List of catalogs
	UniqueImages      []string // List of unique image names
	LayerDigests      []string // All layer digests
}

// DescribeMirror runs oc-mirror describe and parses the output
func DescribeMirror(mirrorPath string) (*DescribeMetrics, error) {
	// Run oc-mirror describe
	cmd := exec.Command("oc-mirror", "describe", mirrorPath)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("oc-mirror describe failed: %w\nStderr: %s", err, stderr.String())
	}

	// Parse JSON output (skip any warning lines before JSON)
	output := stdout.String()
	jsonStart := strings.Index(output, "{")
	if jsonStart == -1 {
		return nil, fmt.Errorf("no JSON found in oc-mirror describe output")
	}
	jsonOutput := output[jsonStart:]

	var metadata MirrorMetadata
	if err := json.Unmarshal([]byte(jsonOutput), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return extractMetrics(&metadata), nil
}

func extractMetrics(metadata *MirrorMetadata) *DescribeMetrics {
	metrics := &DescribeMetrics{
		Catalogs:     make([]string, 0),
		UniqueImages: make([]string, 0),
		LayerDigests: make([]string, 0),
	}

	// Track unique items
	uniqueImages := make(map[string]bool)
	uniqueLayers := make(map[string]bool)
	uniqueCatalogs := make(map[string]bool)

	metrics.TotalAssociations = len(metadata.PastMirror.Associations)

	for _, assoc := range metadata.PastMirror.Associations {
		// Count images (those with registry prefix in name)
		if strings.Contains(assoc.Name, "registry.redhat.io/") ||
			strings.Contains(assoc.Name, "registry.access.redhat.com/") ||
			strings.Contains(assoc.Name, "quay.io/") {
			if !uniqueImages[assoc.Name] {
				uniqueImages[assoc.Name] = true
				metrics.UniqueImages = append(metrics.UniqueImages, assoc.Name)
				metrics.TotalImages++
			}
		}

		// Count manifests
		metrics.TotalManifests += len(assoc.ManifestDigests)

		// Count unique layers
		for _, layer := range assoc.LayerDigests {
			if !uniqueLayers[layer] {
				uniqueLayers[layer] = true
				metrics.LayerDigests = append(metrics.LayerDigests, layer)
			}
		}
	}

	metrics.TotalLayers = len(uniqueLayers)

	// Count operator packages
	for _, op := range metadata.PastMirror.Operators {
		metrics.OperatorPackages += len(op.Packages)
		if !uniqueCatalogs[op.Catalog] {
			uniqueCatalogs[op.Catalog] = true
			metrics.Catalogs = append(metrics.Catalogs, op.Catalog)
		}
	}

	return metrics
}

// PrintSummary prints a summary of the describe metrics
func (m *DescribeMetrics) PrintSummary() {
	fmt.Printf("  │ ─── Mirror Content (from oc-mirror describe) ─────────────────\n")
	fmt.Printf("  │   Total Images: %d\n", m.TotalImages)
	fmt.Printf("  │   Total Layers: %d\n", m.TotalLayers)
	fmt.Printf("  │   Total Manifests: %d\n", m.TotalManifests)
	fmt.Printf("  │   Total Associations: %d\n", m.TotalAssociations)
	fmt.Printf("  │   Operator Packages: %d\n", m.OperatorPackages)
	if len(m.Catalogs) > 0 {
		fmt.Printf("  │   Catalogs: %d\n", len(m.Catalogs))
	}
}

