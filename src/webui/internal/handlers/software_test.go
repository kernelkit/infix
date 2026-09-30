package handlers

import "testing"

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
