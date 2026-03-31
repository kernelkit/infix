package collector

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
)

// CommandRunner executes external commands and returns their stdout.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// FileReader reads files and globs paths on the filesystem.
type FileReader interface {
	ReadFile(path string) ([]byte, error)
	Glob(pattern string) ([]string, error)
}

// ExecRunner is the production CommandRunner using os/exec.
type ExecRunner struct{}

// Run executes name with args and returns combined stdout.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// OSFileReader is the production FileReader using the os package.
type OSFileReader struct{}

// ReadFile reads the named file.
func (OSFileReader) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Glob returns filenames matching the pattern.
func (OSFileReader) Glob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}
