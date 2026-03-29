// SPDX-License-Identifier: MIT

package handlers

import (
	"fmt"
	"os/exec"
	"strings"
)

// HashPassword returns a yescrypt crypt hash of password using mkpasswd(1).
// mkpasswd is available on Infix target systems and uses the system's libcrypt,
// so the output matches whatever the device expects (default: yescrypt $y$).
func HashPassword(password string) (string, error) {
	path, err := exec.LookPath("mkpasswd")
	if err != nil {
		return "", fmt.Errorf("mkpasswd not found: %w", err)
	}
	cmd := exec.Command(path, "--method=yescrypt", "--password-fd=0")
	cmd.Stdin = strings.NewReader(password)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("mkpasswd: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
