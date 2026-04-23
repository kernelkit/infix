// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/base64"
	"strings"
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
