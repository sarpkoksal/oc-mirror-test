package command

// OCMirrorCommandBuilder provides a fluent interface for building oc-mirror commands
// This implements the Builder pattern for better OOP design
type OCMirrorCommandBuilder struct {
	cmd *OCMirrorCommand
}

// NewOCMirrorCommandBuilder creates a new builder instance
func NewOCMirrorCommandBuilder() *OCMirrorCommandBuilder {
	return &OCMirrorCommandBuilder{
		cmd: NewOCMirrorCommand(),
	}
}

// WithV2 sets the v2 flag and returns the builder for method chaining
func (b *OCMirrorCommandBuilder) WithV2(v2 bool) *OCMirrorCommandBuilder {
	b.cmd.SetV2(v2)
	return b
}

// WithConfig sets the config file and returns the builder
func (b *OCMirrorCommandBuilder) WithConfig(config string) *OCMirrorCommandBuilder {
	b.cmd.SetConfig(config)
	return b
}

// WithOutput sets the output destination and returns the builder
func (b *OCMirrorCommandBuilder) WithOutput(output string) *OCMirrorCommandBuilder {
	b.cmd.SetOutput(output)
	return b
}

// WithFrom sets the --from flag and returns the builder
func (b *OCMirrorCommandBuilder) WithFrom(from string) *OCMirrorCommandBuilder {
	b.cmd.SetFrom(from)
	return b
}

// WithCacheDir sets the cache directory and returns the builder
func (b *OCMirrorCommandBuilder) WithCacheDir(cacheDir string) *OCMirrorCommandBuilder {
	b.cmd.SetCacheDir(cacheDir)
	return b
}

// WithSkipMissing sets skip-missing flag and returns the builder
func (b *OCMirrorCommandBuilder) WithSkipMissing(skip bool) *OCMirrorCommandBuilder {
	b.cmd.SetSkipMissing(skip)
	return b
}

// WithContinueOnError sets continue-on-error flag and returns the builder
func (b *OCMirrorCommandBuilder) WithContinueOnError(continueOn bool) *OCMirrorCommandBuilder {
	b.cmd.SetContinueOnError(continueOn)
	return b
}

// WithSkipTLS sets skip TLS verification and returns the builder
func (b *OCMirrorCommandBuilder) WithSkipTLS(skip bool) *OCMirrorCommandBuilder {
	b.cmd.SetSkipTLS(skip)
	return b
}

// WithWorkspace sets the workspace directory and returns the builder
func (b *OCMirrorCommandBuilder) WithWorkspace(workspace string) *OCMirrorCommandBuilder {
	b.cmd.SetWorkspace(workspace)
	return b
}

// Build returns the configured OCMirrorCommand
func (b *OCMirrorCommandBuilder) Build() *OCMirrorCommand {
	return b.cmd
}

// BuildForV1Download creates a pre-configured builder for v1 download operations
func BuildForV1Download(configFile, outputDir string) *OCMirrorCommandBuilder {
	return NewOCMirrorCommandBuilder().
		WithV2(false).
		WithConfig(configFile).
		WithOutput(outputDir).
		WithSkipMissing(true).
		WithContinueOnError(true)
}

// BuildForV1Upload creates a pre-configured builder for v1 upload operations
func BuildForV1Upload(configFile, fromDir, registryURL string, skipTLS bool) *OCMirrorCommandBuilder {
	return NewOCMirrorCommandBuilder().
		WithV2(false).
		WithConfig(configFile).
		WithFrom(fromDir).
		WithOutput(registryURL).
		WithSkipTLS(skipTLS)
}

// BuildForV2Download creates a pre-configured builder for v2 download operations
func BuildForV2Download(configFile, outputDir, cacheDir string, skipTLS bool) *OCMirrorCommandBuilder {
	return NewOCMirrorCommandBuilder().
		WithV2(true).
		WithConfig(configFile).
		WithOutput(outputDir).
		WithCacheDir(cacheDir).
		WithSkipTLS(skipTLS)
}

// BuildForV2Upload creates a pre-configured builder for v2 upload operations
func BuildForV2Upload(configFile, registryURL, cacheDir string, skipTLS bool) *OCMirrorCommandBuilder {
	return NewOCMirrorCommandBuilder().
		WithV2(true).
		WithConfig(configFile).
		WithOutput(registryURL).
		WithCacheDir(cacheDir).
		WithWorkspace("file://./mirror/operators-v2/").
		WithSkipTLS(skipTLS)
}

