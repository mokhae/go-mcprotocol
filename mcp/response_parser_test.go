package mcp

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParser_Do(t *testing.T) {
	mcResp, _ := hex.DecodeString("d00000ffff0300040000000000")

	p := NewParser()
	response, err := p.Do(mcResp)
	if err != nil {
		t.Fatalf("unexpected parser err: %v", err)
	}

	expected := &Response{
		SubHeader:      "D000",
		NetworkNum:     "00",
		PCNum:          "FF",
		UnitIONum:      "FF03",
		UnitStationNum: "00",
		DataLen:        "0400",
		EndCode:        0,
		Payload:        []uint8{0x00, 0x00},
		ErrInfo:        nil,
	}

	if diff := cmp.Diff(response, expected); diff != "" {
		t.Errorf("parse Resp differs: (-got +want)\n%s", diff)
	}
}

func TestParser_DoEndCodeIsLittleEndian(t *testing.T) {
	mcResp, _ := hex.DecodeString("d00000ffff030004003412aabb")

	response, err := NewParser().Do(mcResp)
	if err != nil {
		t.Fatalf("unexpected parser error: %v", err)
	}
	if response.EndCode != 0x1234 {
		t.Fatalf("EndCode = 0x%04X, want 0x1234", response.EndCode)
	}
	if got := hex.EncodeToString(response.ErrInfo); got != "aabb" {
		t.Fatalf("ErrInfo = %s, want aabb", got)
	}
	if response.Payload != nil {
		t.Fatalf("Payload = %X, want nil for an error response", response.Payload)
	}
}

func TestParser_DoRejectsMalformedFrames(t *testing.T) {
	tests := []struct {
		name string
		hex  string
	}{
		{name: "wrong subheader", hex: "500000ffff030002000000"},
		{name: "declared length too short", hex: "d00000ffff0300010000"},
		{name: "truncated body", hex: "d00000ffff030004000000"},
		{name: "trailing bytes", hex: "d00000ffff030002000000ff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, _ := hex.DecodeString(tt.hex)
			_, err := NewParser().Do(frame)
			var protocolErr *ProtocolError
			if !errors.As(err, &protocolErr) {
				t.Fatalf("error = %v, want *ProtocolError", err)
			}
		})
	}
}
