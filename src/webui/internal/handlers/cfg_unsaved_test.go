package handlers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestCfgUnsavedFromRequest(t *testing.T) {
	orig := webuiStartTime
	defer func() { webuiStartTime = orig }()
	webuiStartTime = time.Now()

	mkReq := func(value string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		if value != "" {
			r.AddCookie(&http.Cookie{Name: cfgUnsavedCookie, Value: value})
		}
		return r
	}

	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"no cookie", "", false},
		{"empty value", "", false},
		{"legacy literal 1", "1", false},
		{"junk", "abc", false},
		{"before boot", strconv.FormatInt(webuiStartTime.Add(-time.Hour).Unix(), 10), false},
		{"after boot", strconv.FormatInt(webuiStartTime.Add(time.Hour).Unix(), 10), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cfgUnsavedFromRequest(mkReq(tc.value)); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
