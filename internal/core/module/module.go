package module

// Module defines the interface that all ChatBridge modules must implement.
// This allows for a unified way to start, stop, and manage different parts of the application.
type Module interface {
	// Start initializes and runs the module. It should be non-blocking or spawn its own goroutines.
	// It returns an error if the module fails to start.
	Start() error

	// Stop cleanly shuts down the module, releasing any resources it holds.
	// It returns an error if the shutdown process fails.
	Stop() error

	// Name returns the identifier for the module (e.g., "ChatFlow", "AudioBridge").
	Name() string
}
