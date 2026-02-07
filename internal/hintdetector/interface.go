package hintdetector

// Detector defines the interface for analyzing code changes
type Detector interface {
	// Detect analyzes a line of code and the file path to determine if it represents a significant signal.
	// Returns the signal type, a hint Details, and a boolean indicating if a signal was found.
	Detect(line string, path string) (string, string, bool)
}
