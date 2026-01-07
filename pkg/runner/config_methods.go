package runner

import "fmt"

// Config methods

// Validate validates the configuration and returns an error if invalid
func (c *Config) Validate() error {
	if c.RegistryURL == "" {
		return fmt.Errorf("registry URL is required")
	}
	if c.Iterations < 1 {
		return fmt.Errorf("iterations must be at least 1")
	}
	if c.Iterations < 2 && !c.CompareV1V2 {
		return fmt.Errorf("iterations must be at least 2 for clean vs cached comparison")
	}
	return nil
}

// GetEffectiveIterations returns the effective number of iterations
// For v1/v2 comparison, this accounts for both versions
func (c *Config) GetEffectiveIterations() int {
	if c.CompareV1V2 {
		return c.Iterations * 2 // Both v1 and v2
	}
	return c.Iterations
}

// String returns a string representation of the configuration
func (c *Config) String() string {
	mode := "Standard"
	if c.CompareV1V2 {
		mode = "V1/V2 Comparison"
	}
	return fmt.Sprintf("Config{Registry: %s, Iterations: %d, Mode: %s, SkipTLS: %v}",
		c.RegistryURL, c.Iterations, mode, c.SkipTLS)
}




