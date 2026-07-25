package mcp

import (
	"bytes"
	"errors"
	"testing"
)

func TestStation_BuildRRequest(t *testing.T) {
	station := NewLocalStation()
	request := station.BuildReadRequest("D", 300, 3)

	if request != "500000FFFF03000C001000010400002C0100A80300" {
		t.Fatalf("expected %v but actual is %v", "500000FFFF03000C001000010400002C0100A80300", request)
	}

	request2 := station.BuildReadRequest("D", 500, 50)
	if request2 != "500000FFFF03000C00100001040000F40100A83200" {
		t.Fatalf("expected %v but actual is %v", "500000FFFF03000C00100001040000F40100A83200", request2)
	}
}

func TestPackBits(t *testing.T) {
	// MELSECコミュニケーションプロトコル リファレンス: 1 point per 4 bits,
	// the 1st point in the upper 4 bits of the 1st byte.
	got := PackBits([]bool{true, false, true, false, false, false, true, true})
	if want := []byte{0x10, 0x10, 0x00, 0x11}; !bytes.Equal(got, want) {
		t.Fatalf("expected %X but actual is %X", want, got)
	}

	// odd number of points: the trailing lower 4 bits are padded with 0
	got = PackBits([]bool{true, true, true})
	if want := []byte{0x11, 0x10}; !bytes.Equal(got, want) {
		t.Fatalf("expected %X but actual is %X", want, got)
	}

	if got := PackBits(nil); len(got) != 0 {
		t.Fatalf("expected empty but actual is %X", got)
	}
}

func TestStation_BuildBitWriteRequest(t *testing.T) {
	station := NewLocalStation()

	// M100..M107 = 1,0,1,0,0,0,1,1
	request := station.BuildBitWriteRequest("M", 100, 8, PackBits([]bool{true, false, true, false, false, false, true, true}))
	//              sub    net    pc     io       stn    len     timer   cmd     sub     offset    dev    pts     data
	want := "5000" + "00" + "FF" + "FF03" + "00" + "1000" + "1000" + "0114" + "0100" + "640000" + "90" + "0800" + "10100011"
	if request != want {
		t.Fatalf("expected %v but actual is %v", want, request)
	}

	// single point: M100 ON
	request = station.BuildBitWriteRequest("M", 100, 1, PackBits([]bool{true}))
	want = "5000" + "00" + "FF" + "FF03" + "00" + "0D00" + "1000" + "0114" + "0100" + "640000" + "90" + "0100" + "10"
	if request != want {
		t.Fatalf("expected %v but actual is %v", want, request)
	}
}

func TestValidateBitWriteRequest(t *testing.T) {
	tests := []struct {
		name       string
		deviceName string
		offset     int64
		numPoints  int64
		writeData  []byte
		wantErr    bool
	}{
		{name: "ok even points", deviceName: "M", offset: 100, numPoints: 8, writeData: []byte{0x10, 0x10, 0x00, 0x11}},
		{name: "ok odd points", deviceName: "B", offset: 0, numPoints: 3, writeData: []byte{0x11, 0x10}},

		{name: "word device D is rejected", deviceName: "D", offset: 100, numPoints: 1, writeData: []byte{0x10}, wantErr: true},
		{name: "word device ZR is rejected", deviceName: "ZR", offset: 100, numPoints: 1, writeData: []byte{0x10}, wantErr: true},
		{name: "word device W is rejected", deviceName: "W", offset: 100, numPoints: 1, writeData: []byte{0x10}, wantErr: true},
		{name: "unknown device", deviceName: "QQ", offset: 100, numPoints: 1, writeData: []byte{0x10}, wantErr: true},
		{name: "zero points", deviceName: "M", offset: 100, numPoints: 0, writeData: []byte{}, wantErr: true},
		{name: "too many points", deviceName: "M", offset: 100, numPoints: maxBitPoints + 1, writeData: make([]byte, (maxBitPoints+2)/2), wantErr: true},
		{name: "offset out of 3byte range", deviceName: "M", offset: maxDeviceAddress + 1, numPoints: 1, writeData: []byte{0x10}, wantErr: true},
		{name: "writeData too short", deviceName: "M", offset: 100, numPoints: 8, writeData: []byte{0x10, 0x10}, wantErr: true},
		{name: "writeData too long", deviceName: "M", offset: 100, numPoints: 8, writeData: []byte{0x10, 0x10, 0x00, 0x11, 0x00}, wantErr: true},
		{name: "upper 4 bits not 0 or 1", deviceName: "M", offset: 100, numPoints: 2, writeData: []byte{0xF0}, wantErr: true},
		{name: "lower 4 bits not 0 or 1", deviceName: "M", offset: 100, numPoints: 2, writeData: []byte{0x0F}, wantErr: true},
		{name: "odd points with dirty padding", deviceName: "M", offset: 100, numPoints: 1, writeData: []byte{0x11}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBitWriteRequest(tt.deviceName, tt.offset, tt.numPoints, tt.writeData)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %v, want *ValidationError", err)
			}
		})
	}
}
