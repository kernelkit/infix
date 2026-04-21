// SPDX-License-Identifier: MIT

package testutil

import (
	"context"
	"encoding/json"
	"sync"
)

type mockEntry struct {
	body any
	err  error
}

type MockFetcher struct {
	mu        sync.Mutex
	responses map[string]mockEntry
	errors    map[string]error
}

func NewMockFetcher() *MockFetcher {
	return &MockFetcher{
		responses: make(map[string]mockEntry),
		errors:    make(map[string]error),
	}
}

func (m *MockFetcher) SetResponse(path string, body any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[path] = mockEntry{body: body}
}

func (m *MockFetcher) SetError(path string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[path] = err
}

func (m *MockFetcher) Get(_ context.Context, path string, target any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err, ok := m.errors[path]; ok {
		return err
	}

	entry, ok := m.responses[path]
	if !ok {
		return nil
	}

	raw, err := json.Marshal(entry.body)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func (m *MockFetcher) GetRaw(_ context.Context, path string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err, ok := m.errors[path]; ok {
		return nil, err
	}

	entry, ok := m.responses[path]
	if !ok {
		return []byte("{}"), nil
	}

	return json.Marshal(entry.body)
}

func (m *MockFetcher) Post(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err, ok := m.errors[path]; ok {
		return err
	}
	return nil
}

func (m *MockFetcher) PostJSON(_ context.Context, path string, _ any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err, ok := m.errors[path]; ok {
		return err
	}
	return nil
}

func (m *MockFetcher) GetYANG(_ context.Context, _, _ string) ([]byte, error) { return nil, nil }
func (m *MockFetcher) Put(_ context.Context, _ string, _ any) error          { return nil }
func (m *MockFetcher) Patch(_ context.Context, _ string, _ any) error        { return nil }
func (m *MockFetcher) Delete(_ context.Context, _ string) error              { return nil }

func (m *MockFetcher) GetDatastore(_ context.Context, _ string) (json.RawMessage, error) {
	return json.RawMessage("{}"), nil
}

func (m *MockFetcher) PutDatastore(_ context.Context, _ string, _ json.RawMessage) error {
	return nil
}

func (m *MockFetcher) CopyDatastore(_ context.Context, _, _ string) error { return nil }
