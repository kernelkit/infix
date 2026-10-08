package wpactrl

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wpactrl")
	if err != nil {
		panic(err)
	}
	localDir = dir
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
