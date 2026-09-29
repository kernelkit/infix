package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/godbus/dbus/v5"
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

// InstallerStatus queries RAUC installation progress.
type InstallerStatus interface {
	GetInstallStatus() (operation string, lastError string, percentage int, message string, err error)
}

// runJSON runs a command and decodes its JSON output into dst.
func runJSON(ctx context.Context, cmd CommandRunner, dst interface{}, name string, args ...string) error {
	out, err := cmd.Run(ctx, name, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := json.Unmarshal(out, dst); err != nil {
		return fmt.Errorf("%s: parse output: %w", name, err)
	}
	return nil
}

// ExecRunner is the production CommandRunner using os/exec.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// OSFileReader is the production FileReader using the os package.
type OSFileReader struct{}

func (OSFileReader) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (OSFileReader) Glob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

// DBusInstaller reads RAUC installation status from D-Bus properties.
// It runs on every system-state GET, so it uses the process-wide shared
// bus connection, which godbus re-establishes if it drops, instead of
// paying a connect and auth handshake per query.
type DBusInstaller struct{}

func (DBusInstaller) GetInstallStatus() (string, string, int, string, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return "", "", 0, "", err
	}

	obj := conn.Object("de.pengutronix.rauc", "/")

	operation, _ := obj.GetProperty("de.pengutronix.rauc.Installer.Operation")
	lastError, _ := obj.GetProperty("de.pengutronix.rauc.Installer.LastError")

	var pct int
	var msg string
	progress, err := obj.GetProperty("de.pengutronix.rauc.Installer.Progress")
	if err == nil {
		if vals, ok := progress.Value().([]interface{}); ok && len(vals) >= 2 {
			if p, ok := vals[0].(int32); ok {
				pct = int(p)
			}
			if s, ok := vals[1].(string); ok {
				msg = s
			}
		}
	}

	return variantString(operation), variantString(lastError), pct, msg, nil
}

func variantString(v dbus.Variant) string {
	if s, ok := v.Value().(string); ok {
		return s
	}
	return ""
}
