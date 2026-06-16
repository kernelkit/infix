// SPDX-License-Identifier: MIT

package handlers

import (
	"fmt"
	"os/exec"
	"strings"
)

// CryptMethod is a password-hashing algorithm offered in the UI.
type CryptMethod struct {
	Value   string // form value and mkpasswd --method argument
	Label   string
	Default bool
}

// CryptMethods is the single source of truth for the hashes the UI offers,
// ordered strongest first.  Only the four the infix-system:crypt-hash YANG type
// accepts ($y$/$6$/$5$/$1$) are listed — mkpasswd supports more (bcrypt,
// scrypt, …), but the password leaf's pattern would reject those on save.
var CryptMethods = []CryptMethod{
	{"yescrypt", "yescrypt (recommended)", true},
	{"sha512crypt", "SHA-512", false},
	{"sha256crypt", "SHA-256", false},
	{"md5crypt", "MD5", false},
}

// normalizeCryptMethod returns method if it's one we offer, otherwise the
// default (the entry flagged Default).  This guards an empty or tampered form
// value from reaching mkpasswd --method.
func normalizeCryptMethod(method string) string {
	def := CryptMethods[0].Value
	for _, m := range CryptMethods {
		if m.Default {
			def = m.Value
		}
		if m.Value == method {
			return method
		}
	}
	return def
}

// HashPassword returns a crypt hash of password using mkpasswd(1) with the
// given method.  mkpasswd is available on Infix target systems and uses the
// system's libcrypt.  An empty or unrecognised method (e.g. one the YANG type
// would reject) falls back to the default, so callers can pass the raw form
// value safely.
func HashPassword(password, method string) (string, error) {
	method = normalizeCryptMethod(method)
	path, err := exec.LookPath("mkpasswd")
	if err != nil {
		return "", fmt.Errorf("mkpasswd not found: %w", err)
	}
	cmd := exec.Command(path, "--method="+method, "--password-fd=0")
	cmd.Stdin = strings.NewReader(password)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("mkpasswd: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
