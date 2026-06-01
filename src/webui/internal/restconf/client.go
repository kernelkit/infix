// SPDX-License-Identifier: MIT

package restconf

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Credentials holds username/password for Basic Auth.
// Stored in request contexts by the auth middleware.
type Credentials struct {
	Username string
	Password string
}

type ctxKey struct{}

// ContextWithCredentials returns a child context carrying creds.
func ContextWithCredentials(ctx context.Context, c Credentials) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

// CredentialsFromContext extracts credentials set by the auth middleware.
func CredentialsFromContext(ctx context.Context) Credentials {
	c, _ := ctx.Value(ctxKey{}).(Credentials)
	return c
}

// Client talks to the rousette RESTCONF server.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a RESTCONF client pointing at baseURL
// (e.g. "https://192.168.1.1/restconf" or "https://127.0.0.1/restconf").
// When insecureTLS is true, TLS certificate verification is disabled.
func NewClient(baseURL string, insecureTLS bool) *Client {
	var tlsConfig *tls.Config
	if insecureTLS {
		tlsConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &Client{
		baseURL: strings.TrimRight(escapeZoneID(baseURL), "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
	}
}

// doRequest builds and executes an HTTP request against the RESTCONF server,
// setting Accept and Basic Auth from the context. The caller must close Body.
func (c *Client) doRequest(ctx context.Context, method, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/yang-data+json")
	creds := CredentialsFromContext(ctx)
	req.SetBasicAuth(creds.Username, creds.Password)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("restconf request failed: %w", err)
	}
	return resp, nil
}

// Get fetches a RESTCONF resource, decoding the JSON response into target.
// User credentials are taken from the request context (set by auth middleware).
func (c *Client) Get(ctx context.Context, path string, target any) error {
	resp, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return parseError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

// Post sends a POST request to a RESTCONF RPC endpoint.
// Used for operations like system-restart that return no body.
func (c *Client) Post(ctx context.Context, path string) error {
	resp, err := c.doRequest(ctx, http.MethodPost, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return parseError(resp)
	}
	return nil
}

// PostJSON sends a POST request with a JSON body to a RESTCONF RPC endpoint.
// Used for RPCs that require input parameters (e.g. install-bundle).
func (c *Client) PostJSON(ctx context.Context, path string, body any) error {
	return c.writeJSON(ctx, http.MethodPost, path, body)
}

// Put replaces a RESTCONF config resource with the given value.
func (c *Client) Put(ctx context.Context, path string, body any) error {
	return c.writeJSON(ctx, http.MethodPut, path, body)
}

// Patch merges the given value into a RESTCONF config resource.
func (c *Client) Patch(ctx context.Context, path string, body any) error {
	return c.writeJSON(ctx, http.MethodPatch, path, body)
}

// Delete removes a RESTCONF config resource.
func (c *Client) Delete(ctx context.Context, path string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return parseError(resp)
	}
	return nil
}

// GetDatastore fetches the full contents of the named NMDA datastore as raw JSON.
// datastore is one of "running", "candidate", "operational", "startup", "factory-default".
func (c *Client) GetDatastore(ctx context.Context, datastore string) (json.RawMessage, error) {
	return c.GetRaw(ctx, "/ds/ietf-datastores:"+datastore)
}

// PutDatastore replaces the named NMDA datastore with the given raw JSON.
func (c *Client) PutDatastore(ctx context.Context, datastore string, body json.RawMessage) error {
	return c.writeJSON(ctx, http.MethodPut, "/ds/ietf-datastores:"+datastore, body)
}

// CopyDatastore copies the full contents of src into dst.
// Mirrors the Infamy test framework's copy() operation.
func (c *Client) CopyDatastore(ctx context.Context, src, dst string) error {
	data, err := c.GetDatastore(ctx, src)
	if err != nil {
		return fmt.Errorf("copy datastore: read %s: %w", src, err)
	}
	if err := c.PutDatastore(ctx, dst, data); err != nil {
		return fmt.Errorf("copy datastore: write %s: %w", dst, err)
	}
	return nil
}

// writeJSON encodes body as JSON and sends it with the given HTTP method.
func (c *Client) writeJSON(ctx context.Context, method, path string, body any) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return fmt.Errorf("encoding request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/yang-data+json")
	req.Header.Set("Accept", "application/yang-data+json")
	creds := CredentialsFromContext(ctx)
	req.SetBasicAuth(creds.Username, creds.Password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("restconf request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusCreated &&
		resp.StatusCode != http.StatusNoContent {
		return parseError(resp)
	}
	return nil
}

// GetYANG downloads a YANG module file. YANG files are served outside the
// RESTCONF tree at /yang/{name}@{revision}.yang on the device host — mirroring
// the approach used by the Infamy test framework (yang_url = base_url + "/yang").
func (c *Client) GetYANG(ctx context.Context, name, revision string) ([]byte, error) {
	// Strip the /restconf suffix to get the host-level base URL.
	yangBase := c.baseURL
	if i := strings.LastIndex(yangBase, "/restconf"); i >= 0 {
		yangBase = yangBase[:i]
	}

	path := "/yang/" + name
	if revision != "" {
		path += "@" + revision
	}
	path += ".yang"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, yangBase+path, nil)
	if err != nil {
		return nil, err
	}
	creds := CredentialsFromContext(ctx)
	req.SetBasicAuth(creds.Username, creds.Password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("restconf request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp)
	}
	return io.ReadAll(resp.Body)
}

// GetRaw fetches a RESTCONF resource and returns the raw JSON bytes.
func (c *Client) GetRaw(ctx context.Context, path string) ([]byte, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp)
	}
	return io.ReadAll(resp.Body)
}

// CheckAuth verifies that the given credentials are accepted by rousette.
// It does a simple GET against /data/ietf-system:system with Basic Auth.
func (c *Client) CheckAuth(username, password string) error {
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		c.baseURL+"/data/ietf-system:system",
		nil,
	)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/yang-data+json")
	req.SetBasicAuth(username, password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("restconf request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &AuthError{Code: resp.StatusCode}
	}
	if resp.StatusCode != http.StatusOK {
		return parseError(resp)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "yang-data+json") {
		return fmt.Errorf("unexpected content-type from RESTCONF server: %q", ct)
	}

	return nil
}

// escapeZoneID replaces bare "%" in IPv6 zone IDs with "%25" so that
// Go's url.Parse doesn't reject them as invalid percent-encoding.
// e.g. "https://[ff02::1%qtap1]/restconf" → "https://[ff02::1%25qtap1]/restconf"
func escapeZoneID(rawURL string) string {
	open := strings.Index(rawURL, "[")
	close := strings.Index(rawURL, "]")
	if open < 0 || close < 0 || close < open {
		return rawURL
	}

	host := rawURL[open:close]
	if pct := strings.Index(host, "%"); pct >= 0 && !strings.HasPrefix(host[pct:], "%25") {
		return rawURL[:open+pct] + "%25" + rawURL[open+pct+1:]
	}
	return rawURL
}
