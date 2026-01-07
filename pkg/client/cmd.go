package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// NewDownloadCommand creates a cobra command for downloading client tools
func NewDownloadCommand() *cobra.Command {
	var ocpVersion string
	var binDir string
	var tools []string

	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download OpenShift client tools (oc, opm, oc-mirror)",
		Long:  "Downloads and installs OpenShift client tools from the official mirror. Supports concurrent downloads and automatic system detection.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if ocpVersion == "" {
				ocpVersion = "4.20"
			}
			if binDir == "" {
				binDir = "./bin"
			}
			if len(tools) == 0 {
				tools = []string{"oc", "opm", "oc-mirror"}
			}

			downloader, err := NewDownloader(ocpVersion, binDir)
			if err != nil {
				return fmt.Errorf("failed to create downloader: %w", err)
			}
			defer downloader.Cleanup()

			// Set progress callback
			downloader.SetProgressFunc(func(tool string, downloaded, total int64) {
				if total > 0 {
					percent := float64(downloaded) / float64(total) * 100
					fmt.Printf("\r  │ Downloading %s: %.1f%% (%d/%d bytes)", tool, percent, downloaded, total)
				}
			})

			fmt.Printf("╔════════════════════════════════════════════════════════════════╗\n")
			fmt.Printf("║       OpenShift Client Tools Downloader                       ║\n")
			fmt.Printf("╚════════════════════════════════════════════════════════════════╝\n")
			fmt.Printf("\n")
			fmt.Printf("  System Information:\n")
			fmt.Printf("    OS: %s\n", downloader.OS)
			fmt.Printf("    Architecture: %s\n", downloader.Arch)
			fmt.Printf("    RHEL Version: %s\n", downloader.RHELVersion)
			fmt.Printf("    OpenShift Version: %s\n", downloader.OCPVersion)
			fmt.Printf("    Target Directory: %s\n", downloader.BinDir)
			fmt.Printf("\n")

			ctx := context.Background()
			results, err := downloader.DownloadAll(ctx, tools)
			if err != nil {
				return fmt.Errorf("download failed: %w", err)
			}

			// Print results
			fmt.Printf("\n")
			fmt.Printf("╔════════════════════════════════════════════════════════════════╗\n")
			fmt.Printf("║  Installation Summary                                        ║\n")
			fmt.Printf("╠════════════════════════════════════════════════════════════════╣\n")

			allSuccess := true
			for _, result := range results {
				if result.Success {
					fmt.Printf("║  ✅ %s: SUCCESS\n", result.Tool)
					fmt.Printf("║     Version: %s\n", result.Version)
					fmt.Printf("║     Location: %s\n", result.Path)
				} else {
					fmt.Printf("║  ❌ %s: FAILED\n", result.Tool)
					if result.Error != nil {
						fmt.Printf("║     Error: %v\n", result.Error)
					}
					allSuccess = false
				}
			}

			fmt.Printf("╚════════════════════════════════════════════════════════════════╝\n")
			fmt.Printf("\n")

			if allSuccess {
				fmt.Printf("📁 All binaries installed in: %s\n", downloader.BinDir)
				fmt.Printf("\n")
				absPath, _ := filepath.Abs(downloader.BinDir)
				fmt.Printf("💡 To use these binaries, add them to your PATH:\n")
				fmt.Printf("   export PATH=\"%s:$PATH\"\n", absPath)
				fmt.Printf("\n")
			} else {
				return fmt.Errorf("some downloads failed")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&ocpVersion, "version", "v", "4.20", "OpenShift version to download")
	cmd.Flags().StringVarP(&binDir, "bin-dir", "b", "./bin", "Directory to install binaries")
	cmd.Flags().StringSliceVarP(&tools, "tools", "t", []string{"oc", "opm", "oc-mirror"}, "Tools to download (oc, opm, oc-mirror)")

	return cmd
}

// EnsureTools ensures required tools are available, downloading if necessary
func EnsureTools(ctx context.Context, binDir string, tools []string) error {
	// First check if tools are in PATH
	var toolsToDownload []string
	for _, tool := range tools {
		if path, err := CheckToolInPath(tool); err == nil {
			// Tool found in PATH, verify it works
			downloader, _ := NewDownloader("4.20", binDir)
			if downloader != nil {
				if _, err := downloader.verifyTool(path, tool); err == nil {
					continue // Tool is available and working
				}
			}
		}
		toolsToDownload = append(toolsToDownload, tool)
	}

	if len(toolsToDownload) == 0 {
		return nil // All tools already available in PATH
	}

	downloader, err := NewDownloader("4.20", binDir)
	if err != nil {
		return err
	}
	defer downloader.Cleanup()

	// Check which tools need downloading from binDir
	var toolsNeedingDownload []string
	for _, tool := range toolsToDownload {
		toolPath := filepath.Join(binDir, tool)
		if _, err := os.Stat(toolPath); os.IsNotExist(err) {
			toolsNeedingDownload = append(toolsNeedingDownload, tool)
		} else if _, err := downloader.verifyTool(toolPath, tool); err != nil {
			// Tool exists but verification failed, re-download
			toolsNeedingDownload = append(toolsNeedingDownload, tool)
		}
	}

	if len(toolsNeedingDownload) == 0 {
		return nil // All tools already available in binDir
	}

	// Download missing tools
	results, err := downloader.DownloadAll(ctx, toolsNeedingDownload)
	if err != nil {
		return err
	}

	// Check results
	for _, result := range results {
		if !result.Success {
			return fmt.Errorf("failed to download %s: %w", result.Tool, result.Error)
		}
	}

	return nil
}

