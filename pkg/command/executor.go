package command

// CommandExecutor defines an interface for executing commands
// This enables dependency injection and improves testability
type CommandExecutor interface {
	// Execute runs the command and returns output
	Execute() (*CommandOutput, error)
	
	// ExecuteWithCallback runs the command with a callback for process start
	ExecuteWithCallback(onStart func(pid int)) (*CommandOutput, error)
}

// Ensure OCMirrorCommand implements CommandExecutor
var _ CommandExecutor = (*OCMirrorCommand)(nil)

// MockCommandExecutor is a mock implementation for testing
type MockCommandExecutor struct {
	Output *CommandOutput
	Error  error
}

// Execute implements CommandExecutor interface
func (m *MockCommandExecutor) Execute() (*CommandOutput, error) {
	if m.Error != nil {
		return m.Output, m.Error
	}
	return m.Output, nil
}

// ExecuteWithCallback implements CommandExecutor interface
func (m *MockCommandExecutor) ExecuteWithCallback(onStart func(pid int)) (*CommandOutput, error) {
	if onStart != nil {
		onStart(12345) // Mock PID
	}
	return m.Execute()
}




