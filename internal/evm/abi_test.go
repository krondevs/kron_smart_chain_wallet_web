package evm

import (
	"encoding/json"
	"strings"
	"testing"
)

const testABI = `[
  {
    "type": "function",
    "name": "balanceOf",
    "stateMutability": "view",
    "inputs": [{"name":"owner","type":"address"}],
    "outputs": [{"name":"","type":"uint256"}]
  },
  {
    "type": "function",
    "name": "totalSupply",
    "stateMutability": "view",
    "inputs": [],
    "outputs": [{"name":"","type":"uint256"}]
  },
  {
    "type": "function",
    "name": "transfer",
    "stateMutability": "nonpayable",
    "inputs": [{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],
    "outputs": [{"name":"","type":"bool"}]
  },
  {
    "type": "constructor",
    "inputs": [{"name":"supply","type":"uint256"}]
  }
]`

func TestInspect(t *testing.T) {
	info, err := Inspect(testABI)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Methods) != 3 {
		t.Fatalf("methods = %d", len(info.Methods))
	}
	if len(info.Constructor) != 1 {
		t.Fatalf("constructor args = %d", len(info.Constructor))
	}
	for _, method := range info.Methods {
		if method.Inputs == nil || method.Outputs == nil {
			t.Fatalf("method %s has nil inputs/outputs", method.Name)
		}
	}
	if info.Events == nil || info.Constructor == nil {
		t.Fatal("events/constructor must be non-nil arrays")
	}
}

func TestEncodeCall(t *testing.T) {
	args := []json.RawMessage{
		json.RawMessage(`"0x0000000000000000000000000000000000000001"`),
		json.RawMessage(`"100"`),
	}
	data, err := EncodeCall(testABI, "transfer", args)
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := "a9059cbb"
	if !strings.HasPrefix(data, wantPrefix) {
		t.Fatalf("selector: %s", data)
	}
	if !strings.Contains(data, "0000000000000000000000000000000000000000000000000000000000000064") {
		t.Fatalf("amount missing from calldata: %s", data)
	}
}

func TestDecodeOutput(t *testing.T) {
	output := "00000000000000000000000000000000000000000000000000000000000000ff"
	values, err := DecodeOutput(testABI, "balanceOf", output)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0] != "255" {
		t.Fatalf("decoded = %#v", values)
	}
}

func TestEncodeConstructor(t *testing.T) {
	data, err := EncodeConstructor(testABI, []json.RawMessage{json.RawMessage(`"1000000"`)})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 64 {
		t.Fatalf("constructor data length = %d", len(data))
	}
}
