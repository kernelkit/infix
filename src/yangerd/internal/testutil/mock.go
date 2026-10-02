package testutil

import (
	"context"
	"fmt"
)

// MockRunner records command invocations and returns pre-configured output.
type MockRunner struct {
	Results map[string][]byte
	Errors  map[string]error
}

// Run returns the pre-configured result for the command name.
func (m *MockRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name
	for _, a := range args {
		key += " " + a
	}
	if err, ok := m.Errors[key]; ok {
		return nil, err
	}
	if data, ok := m.Results[key]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("mock: no result for %q", key)
}

// MockFileReader returns pre-configured file contents.
type MockFileReader struct {
	Files map[string][]byte
	Globs map[string][]string
}

// ReadFile returns pre-configured data for the path.
func (m *MockFileReader) ReadFile(path string) ([]byte, error) {
	if data, ok := m.Files[path]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("mock: file not found: %s", path)
}

// Glob returns pre-configured matches for the pattern.
func (m *MockFileReader) Glob(pattern string) ([]string, error) {
	if matches, ok := m.Globs[pattern]; ok {
		return matches, nil
	}
	return nil, nil
}
