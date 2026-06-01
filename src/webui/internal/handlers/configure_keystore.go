// SPDX-License-Identifier: MIT

package handlers

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/kernelkit/webui/internal/restconf"
	"github.com/kernelkit/webui/internal/schema"
)

const keystorePath = candidatePath + "/ietf-keystore:keystore"

// ─── Template data ────────────────────────────────────────────────────────────

type cfgKeystorePageData struct {
	PageData
	Loading        bool
	SymmetricKeys  []cfgSymKeyEntry
	AsymmetricKeys []cfgAsymKeyEntry
	SymKeyFormats  []schema.IdentityOption
	Error          string
}

type cfgSymKeyEntry struct {
	Name   string
	Format string
	Value  string
}

type cfgCertEntry struct {
	Name string
	PEM  string // DER re-encoded as PEM for display
}

type cfgAsymKeyEntry struct {
	Name          string
	Algorithm     string
	PublicKeyPEM  string
	PrivateKeyPEM string
	Certificates  []cfgCertEntry
}

// ─── Handler ──────────────────────────────────────────────────────────────────

type ConfigureKeystoreHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
	Schema   *schema.Cache
}

// Overview renders the Configure > Keystore page reading from the candidate.
// GET /configure/keystore
func (h *ConfigureKeystoreHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := cfgKeystorePageData{
		PageData: newPageData(r, "configure-keystore", "Configure: Keystore"),
	}

	var ks keystoreWrapper
	if err := h.RC.Get(r.Context(), keystorePath, &ks); err != nil {
		var rcErr *restconf.Error
		if errors.As(err, &rcErr) && rcErr.StatusCode == http.StatusNotFound {
			if fallErr := h.RC.Get(r.Context(), "/data/ietf-keystore:keystore", &ks); fallErr != nil {
				var rcFall *restconf.Error
				if !errors.As(fallErr, &rcFall) || rcFall.StatusCode != http.StatusNotFound {
					log.Printf("configure keystore (running fallback): %v", fallErr)
					data.Error = "Could not read keystore"
				}
			}
		} else {
			log.Printf("configure keystore: %v", err)
			data.Error = "Could not read keystore"
		}
	}

	for _, k := range ks.Keystore.SymmetricKeys.SymmetricKey {
		data.SymmetricKeys = append(data.SymmetricKeys, cfgSymKeyEntry{
			Name:   k.Name,
			Format: shortFormat(k.KeyFormat),
			Value:  decodeSymmetricValue(k),
		})
	}
	for _, k := range ks.Keystore.AsymmetricKeys.AsymmetricKey {
		entry := cfgAsymKeyEntry{
			Name:          k.Name,
			Algorithm:     asymAlgorithm(k),
			PublicKeyPEM:  derBase64ToPEM(k.PublicKey, pemBlockType(k.PublicKeyFormat)),
			PrivateKeyPEM: derBase64ToPEM(k.CleartextPrivateKey, pemBlockType(k.PrivateKeyFormat)),
		}
		for _, c := range k.Certificates.Certificate {
			entry.Certificates = append(entry.Certificates, cfgCertEntry{
				Name: c.Name,
				PEM:  derBase64ToPEM(c.CertData, "CERTIFICATE"),
			})
		}
		data.AsymmetricKeys = append(data.AsymmetricKeys, entry)
	}

	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	if mgr != nil {
		data.SymKeyFormats = schema.OptionsFor(mgr, "/ietf-keystore:keystore/symmetric-keys/symmetric-key/key-format")
	}

	tmplName := "configure-keystore.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// AddSymKey adds a symmetric key to the candidate.
// POST /configure/keystore/symmetric
func (h *ConfigureKeystoreHandler) AddSymKey(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	value := r.FormValue("value")
	format := r.FormValue("format")
	if format == "" {
		format = "infix-crypto-types:passphrase-key-format"
	}
	if name == "" {
		renderSaveError(w, fmt.Errorf("name is required"))
		return
	}

	// Passphrase values are base64-encoded plaintext in the YANG model.
	keyB64 := base64.StdEncoding.EncodeToString([]byte(value))

	body := map[string]any{
		"ietf-keystore:symmetric-key": []map[string]any{{
			"name":                    name,
			"key-format":              format,
			"cleartext-symmetric-key": keyB64,
		}},
	}
	path := keystorePath + "/symmetric-keys/symmetric-key=" + url.PathEscape(name)
	if err := h.RC.Put(r.Context(), path, body); err != nil {
		log.Printf("configure keystore add sym %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Symmetric key added", "/configure/keystore")
}

// DeleteSymKey removes a symmetric key from the candidate.
// DELETE /configure/keystore/symmetric/{name}
func (h *ConfigureKeystoreHandler) DeleteSymKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	path := keystorePath + "/symmetric-keys/symmetric-key=" + url.PathEscape(name)
	if err := h.RC.Delete(r.Context(), path); err != nil {
		log.Printf("configure keystore delete sym %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Symmetric key deleted", "/configure/keystore")
}

// AddAsymKey adds an asymmetric key from a PEM-encoded private key.
// POST /configure/keystore/asymmetric
func (h *ConfigureKeystoreHandler) AddAsymKey(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	privPEM := r.FormValue("private_key")
	pubPEM := strings.TrimSpace(r.FormValue("public_key"))
	if name == "" {
		renderSaveError(w, fmt.Errorf("name is required"))
		return
	}
	if privPEM == "" {
		renderSaveError(w, fmt.Errorf("private key is required"))
		return
	}

	block, _ := pem.Decode([]byte(privPEM))
	if block == nil {
		renderSaveError(w, fmt.Errorf("invalid private key PEM: no PEM block found"))
		return
	}
	privB64 := base64.StdEncoding.EncodeToString(block.Bytes)

	keyBody := map[string]any{
		"name":                  name,
		"private-key-format":    pemTypeToKeyFormat(block.Type),
		"cleartext-private-key": privB64,
	}

	// Use explicitly provided public key, or derive from private key (avoids re-parsing PEM).
	if pubPEM == "" {
		pubPEM = derivePublicKeyFromDER(block.Bytes, block.Type)
	}
	if pubPEM != "" {
		if err := applyPublicKey(keyBody, pubPEM); err != nil {
			renderSaveError(w, err)
			return
		}
	}

	body := map[string]any{"ietf-keystore:asymmetric-key": []map[string]any{keyBody}}
	path := keystorePath + "/asymmetric-keys/asymmetric-key=" + url.PathEscape(name)
	if err := h.RC.Put(r.Context(), path, body); err != nil {
		log.Printf("configure keystore add asym %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Asymmetric key added", "/configure/keystore")
}

// derivePublicKeyFromDER extracts the public key from DER-encoded private key bytes
// and returns it as a PKIX PEM block. Returns empty string if derivation fails.
func derivePublicKeyFromDER(der []byte, pemType string) string {
	var pubKey any
	switch strings.ToUpper(pemType) {
	case "EC PRIVATE KEY":
		k, err := x509.ParseECPrivateKey(der)
		if err != nil {
			return ""
		}
		pubKey = &k.PublicKey
	case "RSA PRIVATE KEY":
		k, err := x509.ParsePKCS1PrivateKey(der)
		if err != nil {
			return ""
		}
		pubKey = &k.PublicKey
	default: // PKCS#8 / "PRIVATE KEY"
		k, err := x509.ParsePKCS8PrivateKey(der)
		if err != nil {
			return ""
		}
		switch v := k.(type) {
		case *ecdsa.PrivateKey:
			pubKey = &v.PublicKey
		case *rsa.PrivateKey:
			pubKey = &v.PublicKey
		default:
			return ""
		}
	}

	pubDER, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return ""
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
}

// applyPublicKey parses pubPEM and sets public-key and public-key-format in keyBody.
func applyPublicKey(keyBody map[string]any, pubPEM string) error {
	b64, pemType, err := parsePEMBody(pubPEM)
	if err != nil {
		return fmt.Errorf("invalid public key PEM: %w", err)
	}
	keyBody["public-key-format"] = pemTypeToKeyFormat(pemType)
	keyBody["public-key"] = b64
	return nil
}

// DeleteAsymKey removes an asymmetric key from the candidate.
// DELETE /configure/keystore/asymmetric/{name}
func (h *ConfigureKeystoreHandler) DeleteAsymKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	path := keystorePath + "/asymmetric-keys/asymmetric-key=" + url.PathEscape(name)
	if err := h.RC.Delete(r.Context(), path); err != nil {
		log.Printf("configure keystore delete asym %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Asymmetric key deleted", "/configure/keystore")
}

// AddCert adds a certificate to an asymmetric key entry.
// POST /configure/keystore/asymmetric/{name}/certs
func (h *ConfigureKeystoreHandler) AddCert(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	keyName := r.PathValue("name")
	certName := strings.TrimSpace(r.FormValue("cert_name"))
	pemData := r.FormValue("cert_data")
	if certName == "" || pemData == "" {
		renderSaveError(w, fmt.Errorf("certificate name and data are required"))
		return
	}

	certB64, _, err := parsePEMBody(pemData)
	if err != nil {
		renderSaveError(w, fmt.Errorf("invalid PEM: %w", err))
		return
	}

	body := map[string]any{
		"ietf-keystore:certificate": []map[string]any{{
			"name":      certName,
			"cert-data": certB64,
		}},
	}
	path := keystorePath + "/asymmetric-keys/asymmetric-key=" + url.PathEscape(keyName) +
		"/certificates/certificate=" + url.PathEscape(certName)
	if err := h.RC.Put(r.Context(), path, body); err != nil {
		log.Printf("configure keystore add cert %q/%q: %v", keyName, certName, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Certificate added", "/configure/keystore")
}

// UpdateCert replaces the PEM data of an existing certificate.
// POST /configure/keystore/asymmetric/{name}/certs/{certname}
func (h *ConfigureKeystoreHandler) UpdateCert(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	keyName := r.PathValue("name")
	certName := r.PathValue("certname")
	pemData := r.FormValue("cert_data")
	if pemData == "" {
		renderSaveError(w, fmt.Errorf("certificate PEM data is required"))
		return
	}

	certB64, _, err := parsePEMBody(pemData)
	if err != nil {
		renderSaveError(w, fmt.Errorf("invalid PEM: %w", err))
		return
	}

	body := map[string]any{
		"ietf-keystore:certificate": []map[string]any{{
			"name":      certName,
			"cert-data": certB64,
		}},
	}
	path := keystorePath + "/asymmetric-keys/asymmetric-key=" + url.PathEscape(keyName) +
		"/certificates/certificate=" + url.PathEscape(certName)
	if err := h.RC.Patch(r.Context(), path, body); err != nil {
		log.Printf("configure keystore update cert %q/%q: %v", keyName, certName, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Certificate updated")
}

// DeleteCert removes a certificate from an asymmetric key entry.
// DELETE /configure/keystore/asymmetric/{name}/certs/{certname}
func (h *ConfigureKeystoreHandler) DeleteCert(w http.ResponseWriter, r *http.Request) {
	keyName := r.PathValue("name")
	certName := r.PathValue("certname")
	path := keystorePath + "/asymmetric-keys/asymmetric-key=" + url.PathEscape(keyName) +
		"/certificates/certificate=" + url.PathEscape(certName)
	if err := h.RC.Delete(r.Context(), path); err != nil {
		log.Printf("configure keystore delete cert %q/%q: %v", keyName, certName, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Certificate deleted", "/configure/keystore")
}

// derBase64ToPEM converts a base64-encoded DER blob (as returned by RESTCONF
// for YANG binary fields) back into a PEM-encoded string for display.
func derBase64ToPEM(b64, blockType string) string {
	b64 = strings.ReplaceAll(b64, "\n", "")
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(der) == 0 {
		return ""
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}))
}

// UpdateSymKey changes the value of an existing symmetric key in the candidate.
// POST /configure/keystore/symmetric/{name}
func (h *ConfigureKeystoreHandler) UpdateSymKey(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	value := r.FormValue("value")
	format := r.FormValue("format")
	if format == "" {
		format = "infix-crypto-types:passphrase-key-format"
	}
	keyB64 := base64.StdEncoding.EncodeToString([]byte(value))
	body := map[string]any{
		"ietf-keystore:symmetric-key": []map[string]any{{
			"name":                    name,
			"key-format":              format,
			"cleartext-symmetric-key": keyB64,
		}},
	}
	path := keystorePath + "/symmetric-keys/symmetric-key=" + url.PathEscape(name)
	if err := h.RC.Patch(r.Context(), path, body); err != nil {
		log.Printf("configure keystore update sym %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Key updated")
}

// UpdateAsymKey updates the private and/or public key of an asymmetric key entry.
// POST /configure/keystore/asymmetric/{name}
func (h *ConfigureKeystoreHandler) UpdateAsymKey(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	privPEM := strings.TrimSpace(r.FormValue("private_key"))
	pubPEM := strings.TrimSpace(r.FormValue("public_key"))
	if privPEM == "" && pubPEM == "" {
		renderSaveError(w, fmt.Errorf("at least one key field is required"))
		return
	}

	keyBody := map[string]any{"name": name}
	if privPEM != "" {
		block, _ := pem.Decode([]byte(privPEM))
		if block == nil {
			renderSaveError(w, fmt.Errorf("invalid private key PEM: no PEM block found"))
			return
		}
		keyBody["private-key-format"] = pemTypeToKeyFormat(block.Type)
		keyBody["cleartext-private-key"] = base64.StdEncoding.EncodeToString(block.Bytes)
		if pubPEM == "" {
			pubPEM = derivePublicKeyFromDER(block.Bytes, block.Type)
		}
	}
	if pubPEM != "" {
		if err := applyPublicKey(keyBody, pubPEM); err != nil {
			renderSaveError(w, err)
			return
		}
	}

	body := map[string]any{"ietf-keystore:asymmetric-key": []map[string]any{keyBody}}
	path := keystorePath + "/asymmetric-keys/asymmetric-key=" + url.PathEscape(name)
	if err := h.RC.Patch(r.Context(), path, body); err != nil {
		log.Printf("configure keystore update asym %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Key updated")
}

// pemBlockType maps a YANG key-format identity string to the corresponding PEM block type.
func pemBlockType(format string) string {
	switch shortFormat(format) {
	case "ec":
		return "EC PRIVATE KEY"
	case "rsa":
		return "RSA PRIVATE KEY"
	case "subject-public-key-info":
		return "PUBLIC KEY"
	default: // one-asymmetric (PKCS#8) and anything else
		return "PRIVATE KEY"
	}
}

// parsePEMBody decodes a PEM block and returns the DER content re-encoded as
// standard base64 (no line breaks), plus the PEM block type string.
func parsePEMBody(s string) (b64 string, pemType string, err error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return "", "", fmt.Errorf("no PEM block found")
	}
	return base64.StdEncoding.EncodeToString(block.Bytes), block.Type, nil
}

// pemTypeToKeyFormat maps a PEM block type to the appropriate ietf-crypto-types identity.
func pemTypeToKeyFormat(pemType string) string {
	switch strings.ToUpper(pemType) {
	case "EC PRIVATE KEY":
		return "ietf-crypto-types:ec-private-key-format"
	case "RSA PRIVATE KEY":
		return "ietf-crypto-types:rsa-private-key-format"
	case "PUBLIC KEY":
		return "ietf-crypto-types:subject-public-key-info-format"
	default: // "PRIVATE KEY" (PKCS#8 / one-asymmetric-key) and anything else
		return "ietf-crypto-types:one-asymmetric-key-format"
	}
}
