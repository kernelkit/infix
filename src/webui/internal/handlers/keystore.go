// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/base64"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/kernelkit/webui/internal/restconf"
)

// RESTCONF JSON structures for ietf-keystore:keystore.

type keystoreWrapper struct {
	Keystore keystoreJSON `json:"ietf-keystore:keystore"`
}

type keystoreJSON struct {
	SymmetricKeys  symmetricKeysJSON  `json:"symmetric-keys"`
	AsymmetricKeys asymmetricKeysJSON `json:"asymmetric-keys"`
}

type symmetricKeysJSON struct {
	SymmetricKey []symmetricKeyJSON `json:"symmetric-key"`
}

type symmetricKeyJSON struct {
	Name                  string `json:"name"`
	KeyFormat             string `json:"key-format"`
	CleartextSymmetricKey string `json:"cleartext-symmetric-key"`
}

type asymmetricKeysJSON struct {
	AsymmetricKey []asymmetricKeyJSON `json:"asymmetric-key"`
}

type asymmetricKeyJSON struct {
	Name                string           `json:"name"`
	PrivateKeyFormat    string           `json:"private-key-format"`
	PublicKeyFormat     string           `json:"public-key-format"`
	PublicKey           string           `json:"public-key"`
	CleartextPrivateKey string           `json:"cleartext-private-key"`
	Certificates        certificatesJSON `json:"certificates"`
}

type certificatesJSON struct {
	Certificate []certificateJSON `json:"certificate"`
}

type certificateJSON struct {
	Name     string `json:"name"`
	CertData string `json:"cert-data"`
}

// Template data structures.

type keystoreData struct {
	CsrfToken      string
	Username       string
	ActivePage     string
	PageTitle      string
	Capabilities   *Capabilities
	SymmetricKeys  []symKeyEntry
	AsymmetricKeys []asymKeyEntry
	Empty          bool
	Error          string
}

type symKeyEntry struct {
	Name   string
	Format string
	Value  string
}

type asymKeyEntry struct {
	Name           string
	Algorithm      string
	PublicKey      string
	PublicKeyFull  string
	PrivateKey     string
	PrivateKeyFull string
	Certificates   []string
}

// KeystoreHandler serves the keystore overview page.
type KeystoreHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Overview renders the keystore overview (GET /keystore).
func (h *KeystoreHandler) Overview(w http.ResponseWriter, r *http.Request) {
	creds := restconf.CredentialsFromContext(r.Context())
	data := keystoreData{
		Username:     creds.Username,
		CsrfToken:    csrfToken(r.Context()),
		ActivePage:   "keystore",
		PageTitle:    "Keystore",
		Capabilities: CapabilitiesFromContext(r.Context()),
	}

	var ks keystoreWrapper
	if err := h.RC.Get(r.Context(), "/data/ietf-keystore:keystore", &ks); err != nil {
		log.Printf("restconf keystore: %v", err)
		data.Error = "Could not fetch keystore"
	} else {
		for _, k := range ks.Keystore.SymmetricKeys.SymmetricKey {
			data.SymmetricKeys = append(data.SymmetricKeys, symKeyEntry{
				Name:   k.Name,
				Format: shortFormat(k.KeyFormat),
				Value:  decodeSymmetricValue(k),
			})
		}

		for _, k := range ks.Keystore.AsymmetricKeys.AsymmetricKey {
			entry := asymKeyEntry{
				Name:           k.Name,
				Algorithm:      asymAlgorithm(k),
				PublicKeyFull:  k.PublicKey,
				PublicKey:      truncate(k.PublicKey, 40),
				PrivateKeyFull: k.CleartextPrivateKey,
				PrivateKey:     truncate(k.CleartextPrivateKey, 40),
			}
			for _, c := range k.Certificates.Certificate {
				entry.Certificates = append(entry.Certificates, c.Name)
			}
			data.AsymmetricKeys = append(data.AsymmetricKeys, entry)
		}

		data.Empty = len(data.SymmetricKeys) == 0 && len(data.AsymmetricKeys) == 0
	}

	tmplName := "keystore.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// shortFormat strips the YANG module prefix and "-key-format" suffix.
// e.g. "ietf-crypto-types:octet-string-key-format" → "octet-string"
func shortFormat(full string) string {
	if i := strings.LastIndex(full, ":"); i >= 0 {
		full = full[i+1:]
	}
	full = strings.TrimSuffix(full, "-key-format")
	full = strings.TrimSuffix(full, "-private-key-format")
	full = strings.TrimSuffix(full, "-public-key-format")
	return full
}

// asymAlgorithm derives the key algorithm from the format fields.
func asymAlgorithm(k asymmetricKeyJSON) string {
	for _, fmt := range []string{k.PrivateKeyFormat, k.PublicKeyFormat} {
		name := shortFormat(fmt)
		if name != "" {
			return name
		}
	}
	return ""
}

// decodeSymmetricValue returns the displayable value for a symmetric key.
// Passphrases are base64-decoded to plaintext; others shown as-is.
func decodeSymmetricValue(k symmetricKeyJSON) string {
	val := k.CleartextSymmetricKey
	if val == "" {
		return "-"
	}
	if shortFormat(k.KeyFormat) == "passphrase" {
		if decoded, err := base64.StdEncoding.DecodeString(val); err == nil {
			return string(decoded)
		}
	}
	return val
}

// truncate shortens s to max characters, adding "..." if truncated.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
