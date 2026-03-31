package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kernelkit/infix/src/yangerd/internal/testutil"
)

const (
	testOSRelease = `NAME="Infix"
VERSION_ID="25.01.0"
BUILD_ID="v25.01.0"
ARCHITECTURE="x86_64"
HOME_URL="https://kernelkit.github.io"
`

	testRaucStatus = `{
  "compatible": "Infix x86_64",
  "variant": "",
  "booted": "rootfs.0",
  "slots": [
    {
      "rootfs.0": {
        "bootname": "A",
        "class": "rootfs",
        "state": "booted",
        "slot_status": {
          "bundle": {
            "compatible": "Infix x86_64",
            "version": "25.01.0"
          },
          "checksum": {
            "sha256": "abc123",
            "size": 134217728
          },
          "installed": {
            "timestamp": "2025-01-15T10:30:00Z",
            "count": 3
          },
          "activated": {
            "timestamp": "2025-01-15T10:31:00Z",
            "count": 3
          }
        }
      }
    },
    {
      "rootfs.1": {
        "bootname": "B",
        "class": "rootfs",
        "state": "inactive",
        "slot_status": {
          "bundle": {
            "compatible": "Infix x86_64",
            "version": "24.10.0"
          },
          "checksum": {
            "sha256": "def456",
            "size": 130000000
          },
          "installed": {
            "timestamp": "2024-10-01T08:00:00Z",
            "count": 1
          },
          "activated": {
            "timestamp": "2024-10-01T08:01:00Z",
            "count": 1
          }
        }
      }
    }
  ]
}`

	testRaucInstallStatus = `{
  "operation": "idle",
  "progress": {
    "percentage": 100,
    "message": "Installation complete"
  }
}`

	testBootOrder = "BOOT_ORDER=A B\n"
)

func TestBootPlatform(t *testing.T) {
	fs := &testutil.MockFileReader{
		Files: map[string][]byte{
			"/etc/os-release": []byte(testOSRelease),
		},
	}

	raw := BootPlatform(fs)
	if raw == nil {
		t.Fatal("BootPlatform returned nil")
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	platform, ok := result["platform"].(map[string]interface{})
	if !ok {
		t.Fatal("missing platform key")
	}

	checks := map[string]string{
		"os-name":    "Infix",
		"os-version": "25.01.0",
		"os-release": "v25.01.0",
		"machine":    "x86_64",
	}
	for key, expected := range checks {
		got, ok := platform[key].(string)
		if !ok || got != expected {
			t.Fatalf("platform[%q]: expected %q, got %v", key, expected, platform[key])
		}
	}
}

func TestBootPlatformMissingFile(t *testing.T) {
	fs := &testutil.MockFileReader{Files: map[string][]byte{}}
	raw := BootPlatform(fs)
	if raw != nil {
		t.Fatalf("expected nil for missing os-release, got %s", raw)
	}
}

func TestBootSoftware(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"rauc status --detailed --output-format=json": []byte(testRaucStatus),
			"rauc-installation-status":                    []byte(testRaucInstallStatus),
			"fw_printenv BOOT_ORDER":                      []byte(testBootOrder),
		},
		Errors: map[string]error{},
	}

	raw := BootSoftware(context.Background(), runner)
	if raw == nil {
		t.Fatal("BootSoftware returned nil")
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	sw, ok := result["infix-system:software"].(map[string]interface{})
	if !ok {
		t.Fatal("missing infix-system:software key")
	}

	if sw["compatible"] != "Infix x86_64" {
		t.Fatalf("compatible: expected 'Infix x86_64', got %v", sw["compatible"])
	}
	if sw["booted"] != "rootfs.0" {
		t.Fatalf("booted: expected 'rootfs.0', got %v", sw["booted"])
	}

	bootOrder, ok := sw["boot-order"].([]interface{})
	if !ok || len(bootOrder) != 2 {
		t.Fatalf("expected boot-order [A B], got %v", sw["boot-order"])
	}
	if bootOrder[0] != "A" || bootOrder[1] != "B" {
		t.Fatalf("boot-order: expected [A B], got %v", bootOrder)
	}

	slots, ok := sw["slot"].([]interface{})
	if !ok || len(slots) != 2 {
		t.Fatalf("expected 2 slots, got %v", sw["slot"])
	}

	installer, ok := sw["installer"].(map[string]interface{})
	if !ok {
		t.Fatal("missing installer")
	}
	if installer["operation"] != "idle" {
		t.Fatalf("installer operation: expected 'idle', got %v", installer["operation"])
	}
}

func TestBootSoftwareAllCommandsFail(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{},
		Errors: map[string]error{
			"rauc status --detailed --output-format=json": fmt.Errorf("not found"),
			"rauc-installation-status":                    fmt.Errorf("not found"),
			"fw_printenv BOOT_ORDER":                      fmt.Errorf("not found"),
			"grub-editenv /mnt/aux/grub/grubenv list":     fmt.Errorf("not found"),
		},
	}

	raw := BootSoftware(context.Background(), runner)
	if raw == nil {
		t.Fatal("BootSoftware should return non-nil even when all commands fail")
	}

	var result map[string]interface{}
	json.Unmarshal(raw, &result)
	sw := result["infix-system:software"].(map[string]interface{})
	if _, ok := sw["boot-order"]; ok {
		t.Fatal("boot-order should not be present when commands fail")
	}
	if _, ok := sw["installer"]; !ok {
		t.Fatal("installer key should always be present")
	}
}

func TestReadBootOrderFwPrintenv(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"fw_printenv BOOT_ORDER": []byte("BOOT_ORDER=A B\n"),
		},
		Errors: map[string]error{},
	}

	order := ReadBootOrder(context.Background(), runner)
	if len(order) != 2 || order[0] != "A" || order[1] != "B" {
		t.Fatalf("expected [A B], got %v", order)
	}
}

func TestReadBootOrderGrubFallback(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"grub-editenv /mnt/aux/grub/grubenv list": []byte("ORDER=B A\n"),
		},
		Errors: map[string]error{
			"fw_printenv BOOT_ORDER": fmt.Errorf("command not found"),
		},
	}

	order := ReadBootOrder(context.Background(), runner)
	if len(order) != 2 || order[0] != "B" || order[1] != "A" {
		t.Fatalf("expected [B A], got %v", order)
	}
}

func TestReadBootOrderBothFail(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{},
		Errors: map[string]error{
			"fw_printenv BOOT_ORDER":                  fmt.Errorf("not found"),
			"grub-editenv /mnt/aux/grub/grubenv list": fmt.Errorf("not found"),
		},
	}

	order := ReadBootOrder(context.Background(), runner)
	if order != nil {
		t.Fatalf("expected nil, got %v", order)
	}
}
