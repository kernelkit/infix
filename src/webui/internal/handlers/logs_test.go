// SPDX-License-Identifier: MIT

package handlers

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogsTailOutlivesWriteTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "messages")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	saved := logSources
	logSources = []logSource{{"messages", "Messages", path}}
	defer func() { logSources = saved }()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /maintenance/logs/{name}/tail", (&LogsHandler{}).Tail)
	srv := httptest.NewUnstartedServer(mux)
	srv.Config.WriteTimeout = 200 * time.Millisecond
	srv.Start()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/maintenance/logs/messages/tail")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	time.Sleep(600 * time.Millisecond)
	if err := os.WriteFile(path, []byte("late line\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if strings.Contains(sc.Text(), "late line") {
			return
		}
	}
	t.Fatalf("stream ended before the line arrived: %v", sc.Err())
}
