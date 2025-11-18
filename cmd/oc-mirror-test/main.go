package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/telco-core/ngc-495/pkg/runner"
)

func main() {
	var registryURL string
	var iterations int
	var compareV1V2 bool

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
			}

			testRunner := runner.NewTestRunner(config)
			if err := testRunner.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	rootCmd.Flags().StringVarP(&registryURL, "registry", "r", "", "Registry URL (e.g., docker://infra.5g-deployment.lab:8443/ocp/)")
	rootCmd.Flags().IntVarP(&iterations, "iterations", "i", 2, "Number of iterations to run (minimum 2 for clean vs cached comparison)")
	rootCmd.Flags().BoolVar(&compareV1V2, "compare-v1-v2", false, "Compare v1 and v2 runs of the same imageset configuration")

	rootCmd.MarkFlagRequired("registry")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
