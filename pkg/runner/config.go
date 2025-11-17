package runner

// Config holds the test runner configuration
type Config struct {
	RegistryURL string
	Iterations  int
	CompareV1V2 bool
}
