// SPDX-License-Identifier: MIT

package handlers

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
)

func TestParseWGPrivateKey(t *testing.T) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw := base64.StdEncoding.EncodeToString(priv.Bytes())
	der, _ := x509.MarshalPKCS8PrivateKey(priv)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	for _, in := range []string{raw, pemKey} {
		got, err := parseWGPrivateKey(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if !got.Equal(priv) {
			t.Fatalf("%q: key mismatch", in)
		}
	}

	rsaDER, _ := x509.MarshalPKCS8PrivateKey(mustRSA(t))
	rsaPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: rsaDER}))
	for _, in := range []string{"not a key", base64.StdEncoding.EncodeToString([]byte("short")), rsaPEM} {
		if _, err := parseWGPrivateKey(in); err == nil {
			t.Fatalf("%q: expected error", in)
		}
	}
}

func mustRSA(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	return k
}
