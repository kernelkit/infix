package handlers

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestUnwrapForPutContainer(t *testing.T) {
	raw := []byte(`{"infix-services:web":{"certificate":"gencert","enabled":true}}`)
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	obj, wrap, err := unwrapForPut(doc, "/infix-services:web")
	if err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	if wrap != "infix-services:web" {
		t.Errorf("wrap = %q, want %q", wrap, "infix-services:web")
	}
	if obj["certificate"] != "gencert" {
		t.Errorf("certificate = %v, want gencert", obj["certificate"])
	}
}

func TestUnwrapForPutListInstance(t *testing.T) {
	raw := []byte(`{"ietf-interfaces:interfaces":{"interface":[{"name":"lan3","type":"ethernet","infix-interfaces:bridge-port":{"bridge":"br0"}}]}}`)
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	obj, wrap, err := unwrapForPut(doc, "/ietf-interfaces:interfaces/interface=lan3")
	if err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	if wrap != "interface" {
		t.Errorf("wrap = %q, want %q", wrap, "interface")
	}
	if obj["name"] != "lan3" {
		t.Errorf("name = %v, want lan3", obj["name"])
	}
	if _, ok := obj["infix-interfaces:bridge-port"]; !ok {
		t.Errorf("expected bridge-port key in unwrapped entry, got: %v", obj)
	}
}

func TestUnwrapForPutEmpty(t *testing.T) {
	doc := map[string]any{}
	if _, _, err := unwrapForPut(doc, "/x"); err == nil {
		t.Errorf("expected error for empty doc")
	}
}

func TestExtractFieldValuesWithSibling(t *testing.T) {
	data := []byte(`{
      "ietf-interfaces:interfaces": {
        "interface": [
          {"name": "lan1", "type": "ethernet"},
          {"name": "br0", "type": "bridge", "infix-interfaces:bridge": {"vlans": {}}},
          {"name": "br1", "type": "bridge", "infix-interfaces:bridge": {}},
          {"name": "lan3", "type": "ethernet", "infix-interfaces:bridge-port": {"bridge":"br0"}}
        ]
      }
    }`)
	got := extractFieldValuesWithSibling(data, "name", "bridge")
	want := []string{"br0", "br1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestExtractFieldValuesNestedRecord guards against the bug where a
// leafref dropdown for /ietf-keystore:keystore/asymmetric-keys/
// asymmetric-key/name picked up "self-signed" — which lives in the
// nested certificates/certificate[]/name list inside each asymmetric
// key entry. The walker has to treat the asymmetric-key map as a
// record boundary and not descend into siblings once it has found
// the field there.
func TestExtractFieldValuesNestedRecord(t *testing.T) {
	data := []byte(`{
      "ietf-keystore:keystore": {
        "asymmetric-keys": {
          "asymmetric-key": [
            {
              "name": "gencert",
              "certificates": {
                "certificate": [
                  {"name": "self-signed"}
                ]
              }
            },
            {
              "name": "genkey",
              "certificates": {}
            }
          ]
        }
      }
    }`)
	got := extractFieldValues(data, "name")
	want := []string{"gencert", "genkey"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
