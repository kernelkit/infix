package collector

import (
	"context"
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
type DBusInstaller struct{}

func (DBusInstaller) GetInstallStatus() (string, string, int, string, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return "", "", 0, "", err
	}
	defer conn.Close()

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
