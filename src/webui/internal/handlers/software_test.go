package handlers

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestOtherSlots(t *testing.T) {
	tests := []struct {
		name  string
		slots []slotEntry
		want  []string
	}{
		{"same version", []slotEntry{
			{Name: "primary", Version: "v1", Booted: true},
			{Name: "secondary", Version: "v1"},
		}, nil},
		{"other is older", []slotEntry{
			{Name: "primary", Version: "v2", Booted: true},
			{Name: "secondary", Version: "v1"},
		}, []string{"secondary"}},
		{"other is empty", []slotEntry{
			{Name: "primary", Version: "v2", Booted: true},
			{Name: "secondary"},
		}, []string{"secondary"}},
		{"none booted", []slotEntry{
			{Name: "primary", Version: "v2"},
			{Name: "secondary", Version: "v1"},
		}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := softwareData{Slots: tt.slots}.OtherSlots()
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i].Name != tt.want[i] {
					t.Errorf("got %s, want %s", got[i].Name, tt.want[i])
				}
			}
		})
	}
}

type memPart struct{ *bytes.Reader }

func (memPart) Close() error { return nil }

func TestKeepUpload(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	spilled, err := os.CreateTemp("", "multipart-")
	if err != nil {
		t.Fatal(err)
	}
	spilled.WriteString("bundle")
	spilled.Seek(0, io.SeekStart)
	path, err := keepUpload(spilled)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(spilled.Name()); !os.IsNotExist(err) {
		t.Errorf("spilled part still present, was copied rather than renamed")
	}
	if b, _ := os.ReadFile(path); string(b) != "bundle" {
		t.Errorf("kept bundle = %q", b)
	}

	path, err = keepUpload(memPart{bytes.NewReader([]byte("small"))})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "small" {
		t.Errorf("kept small bundle = %q", b)
	}
}
