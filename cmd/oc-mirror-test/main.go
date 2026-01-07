package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/telco-core/ngc-495/pkg/client"
	"github.com/telco-core/ngc-495/pkg/runner"
	"github.com/telco-core/ngc-495/pkg/webui"
)

func main() {
	var registryURL string
	var iterations int
	var compareV1V2 bool
	var skipTLS bool

	var rootCmd = &cobra.Command{
		Use:   "oc-mirror-test",
		Short: "OC Mirror test automation with metrics collection",
		Long:  "Runs oc-mirror tests with metrics collection including time, bytes, logs, and network utilization. Supports v1 and v2 comparison.",
		Run: func(cmd *cobra.Command, args []string) {
			if registryURL == "" {
				fmt.Fprintf(os.Stderr, "Error: registry URL is required\n")
				os.Exit(1)
			}

			config := &runner.Config{
				RegistryURL: registryURL,
				Iterations:  iterations,
				CompareV1V2: compareV1V2,
				SkipTLS:     skipTLS,
			}

			testRunner := runner.NewTestRunner(config)
			if err := testRunner.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	var webUICmd = &cobra.Command{
		Use:   "webui",
		Short: "Start the web UI server to view mirroring metrics",
		Long:  "Starts a web server that displays mirroring metrics from test results in a browser-based dashboard. Can run tests concurrently for live metrics viewing.",
		Run: func(cmd *cobra.Command, args []string) {
			port, _ := cmd.Flags().GetInt("port")
			resultsDir, _ := cmd.Flags().GetString("results-dir")
			
			// Check if test flags are provided
			testRegistry, _ := cmd.Flags().GetString("registry")
			testIterations, _ := cmd.Flags().GetInt("iterations")
			testCompareV1V2, _ := cmd.Flags().GetBool("compare-v1-v2")
			testSkipTLS, _ := cmd.Flags().GetBool("skip-tls")

			server := webui.NewServer(port, resultsDir)
			
			// If test flags are provided, run tests in background
			if testRegistry != "" {
				// Ensure registry URL has proper format
				if !strings.Contains(testRegistry, "://") {
					testRegistry = "docker://" + testRegistry
				}
				
				config := &runner.Config{
					RegistryURL: testRegistry,
					Iterations:  testIterations,
					CompareV1V2: testCompareV1V2,
					SkipTLS:     testSkipTLS,
				}
				testRunner := runner.NewTestRunner(config)
				
				// Set registry monitor in server for live metrics API
				if registryMonitor := testRunner.GetRegistryMonitor(); registryMonitor != nil {
					server.SetRegistryMonitor(registryMonitor)
				}
				
				fmt.Printf("\n")
				fmt.Printf("╔═══════════════════════════════════════════════════════════════╗\n")
				fmt.Printf("║  Starting tests in background with live metrics viewing     ║\n")
				fmt.Printf("╚═══════════════════════════════════════════════════════════════╝\n")
				fmt.Printf("Registry: %s\n", testRegistry)
				fmt.Printf("Iterations: %d\n", testIterations)
				if testCompareV1V2 {
					fmt.Printf("Mode: V1 vs V2 Comparison\n")
				}
				fmt.Printf("\n")
				
				// Run tests in background goroutine
				go func() {
					if err := testRunner.Run(); err != nil {
						fmt.Fprintf(os.Stderr, "Test execution error: %v\n", err)
					} else {
						fmt.Printf("✅ Test execution completed successfully.\n")
					}
				}()
			}

			if err := server.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	rootCmd.Flags().StringVarP(&registryURL, "registry", "r", "", "Registry URL (e.g., docker://infra.5g-deployment.lab:8443/ocp/)")
	rootCmd.Flags().IntVarP(&iterations, "iterations", "i", 2, "Number of iterations to run (minimum 2 for clean vs cached comparison)")
	rootCmd.Flags().BoolVar(&compareV1V2, "compare-v1-v2", false, "Compare v1 and v2 runs of the same imageset configuration")
	rootCmd.Flags().BoolVar(&skipTLS, "skip-tls", false, "Skip TLS verification for destination registry (--dest-tls-verify=false)")

	webUICmd.Flags().IntP("port", "p", 8080, "Port to run the web server on")
	webUICmd.Flags().String("results-dir", "results", "Directory containing test results JSON files")
	// Add test flags to webui command (these run tests in background when provided)
	webUICmd.Flags().StringP("registry", "r", "", "Registry URL for test execution (runs tests in background)")
	webUICmd.Flags().IntP("iterations", "i", 2, "Number of test iterations to run")
	webUICmd.Flags().Bool("compare-v1-v2", false, "Compare v1 and v2 runs")
	webUICmd.Flags().Bool("skip-tls", false, "Skip TLS verification for destination registry")

	// Add download command
	downloadCmd := client.NewDownloadCommand()

	rootCmd.MarkFlagRequired("registry")
	rootCmd.AddCommand(webUICmd)
	rootCmd.AddCommand(downloadCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
